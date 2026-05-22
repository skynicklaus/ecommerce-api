package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/imageutil"
	"github.com/skynicklaus/ecommerce-api/util"
)

const (
	maxFilesPerUpload     = 10
	multipartMemoryBudget = 10 << 20
)

type PendingUpload struct {
	Token           string    `json:"token"`
	OrganizationID  uuid.UUID `json:"organizationId"`
	TempKey         string    `json:"tempKey"`
	FinalKey        string    `json:"finalKey"`
	Type            string    `json:"type"`
	ContentType     string    `json:"contentType"`
	OriginalName    string    `json:"originalName"`
	DurationSeconds float64   `json:"durationSeconds"`
}

type UploadTokenResponse struct {
	Token        string `json:"token"`
	OriginalName string `json:"originalName"`
	ContentType  string `json:"contentType"`
}

func (h *V1Handler) PreUploadAssets(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	organization, ctxErr := organizationFromCtx(ctx)
	if ctxErr != nil {
		return ctxErr
	}

	maxUploadSize := h.maxVideoSize * maxFilesPerUpload
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	//nolint:gosec // G120: Request body is strictly bounded by MaxBytesReader above
	if err := r.ParseMultipartForm(multipartMemoryBudget); err != nil {
		return apierror.NewAPIError(
			http.StatusRequestEntityTooLarge,
			err,
		)
	}
	defer func() {
		if err := r.MultipartForm.RemoveAll(); err != nil {
			h.logger.WarnContext(
				ctx,
				"failed to clean up multipart temp files",
				slog.Any("err", err),
			)
		}
	}()

	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("no files provided"))
	}
	if len(headers) > maxFilesPerUpload {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("too many files"))
	}

	results := make([]*PendingUpload, len(headers))
	g, gCtx := errgroup.WithContext(ctx)

	for i, header := range headers {
		g.Go(func() error {
			record, err := h.processTempUpload(gCtx, header, organization.ID)
			if err != nil {
				return err
			}
			results[i] = record
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		var cleanupWg sync.WaitGroup
		for _, record := range results {
			if record == nil {
				continue
			}

			cleanupWg.Add(1)
			go func(key string) {
				defer cleanupWg.Done()
				h.deleteTempObject(ctx, key)
			}(record.TempKey)
		}
		cleanupWg.Wait()

		if apiErr, ok := errors.AsType[apierror.APIError](err); ok {
			return apiErr
		}
		return apierror.NewAPIError(http.StatusInternalServerError, err)
	}

	tokens := make([]UploadTokenResponse, len(results))
	for i, record := range results {
		tokens[i] = UploadTokenResponse{
			Token:        record.Token,
			OriginalName: record.OriginalName,
			ContentType:  record.ContentType,
		}
	}

	return WriteJSON(w, http.StatusCreated, map[string]any{
		"uploads": tokens,
	})
}

func (h *V1Handler) processTempUpload(
	ctx context.Context,
	header *multipart.FileHeader,
	organizationID uuid.UUID,
) (*PendingUpload, error) {
	file, err := header.Open()
	if err != nil {
		return nil, fmt.Errorf("cannot open %q", header.Filename)
	}
	defer file.Close()

	const sniffSize = 512
	sniff := make([]byte, sniffSize)
	n, err := io.ReadFull(file, sniff)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, fmt.Errorf("cannot read %q", header.Filename)
	}
	sniff = sniff[:n]

	contentType := http.DetectContentType(sniff)
	if !h.mime.Allowed(contentType) {
		return nil, apierror.NewAPIError(
			http.StatusBadRequest,
			fmt.Errorf("type %q is not allowed", contentType),
		)
	}

	full := io.MultiReader(bytes.NewReader(sniff), file)

	switch {
	case strings.HasPrefix(contentType, "image/"):
		return h.processImageUpload(ctx, organizationID, full, header, contentType)
	case strings.HasPrefix(contentType, "video/"):
		if header.Size > h.maxVideoSize {
			return nil, apierror.NewAPIError(
				http.StatusBadRequest,
				fmt.Errorf("%q exceeds size limit", header.Filename),
			)
		}

		return h.processVideoUpload(ctx, organizationID, full, header)
	default:
		return nil, apierror.NewAPIError(
			http.StatusBadRequest,
			fmt.Errorf("type %q not supported", contentType),
		)
	}
}

func (h *V1Handler) processImageUpload(
	ctx context.Context,
	organizationID uuid.UUID,
	reader io.Reader,
	header *multipart.FileHeader,
	contentType string,
) (*PendingUpload, error) {
	if header.Size > h.maxImageSize {
		return nil, apierror.NewAPIError(
			http.StatusBadRequest,
			fmt.Errorf("%q exceeds size limit", header.Filename),
		)
	}

	buf, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("unable to read from reader: %w", err)
	}

	newBuf, err := imageutil.ValidateAndStrip(buf)
	if err != nil {
		return nil, apierror.NewAPIError(http.StatusBadRequest, err)
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = h.mime.Extension(contentType)
	}

	tempKey := fmt.Sprintf("temp/%s%s", uuid.New().String(), ext)
	imageSize := int64(len(newBuf))
	//nolint:exhaustruct // too many fields
	_, err = h.storage.S3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        h.bucket,
		Key:           &tempKey,
		Body:          bytes.NewReader(newBuf),
		ContentLength: &imageSize,
		ContentType:   &contentType,
		Metadata:      map[string]string{"original-file-name": header.Filename},
	})
	if err != nil {
		return nil, err
	}

	digest := sha256.Sum256(newBuf)
	hashHex := hex.EncodeToString(digest[:])
	now := time.Now()
	year, month, _ := now.Date()
	token := uuid.New().String()
	finalKey := fmt.Sprintf(
		"assets/%s/%d/%02d/%s%s",
		organizationID.String(),
		year,
		int(month),
		hashHex,
		ext,
	)

	pendingUpload := &PendingUpload{
		Token:           token,
		OrganizationID:  organizationID,
		TempKey:         tempKey,
		FinalKey:        finalKey,
		Type:            string(util.ProductAssetImage),
		ContentType:     contentType,
		OriginalName:    header.Filename,
		DurationSeconds: 0,
	}

	cacheData, err := json.Marshal(pendingUpload)
	if err != nil {
		h.deleteTempObject(ctx, tempKey)
		return nil, err
	}

	if cacheErr := h.cache.CachePendingUpload(ctx, token, cacheData); cacheErr != nil {
		h.deleteTempObject(ctx, tempKey)

		return nil, fmt.Errorf("failed to cache pending upload: %w", cacheErr)
	}

	return pendingUpload, nil
}

//nolint:funlen // still readable
func (h *V1Handler) processVideoUpload(
	ctx context.Context,
	organizationID uuid.UUID,
	reader io.Reader,
	header *multipart.FileHeader,
) (*PendingUpload, error) {
	tempFile, err := os.CreateTemp("", "video-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	// safe to ignore
	_ = tempFile.Close()
	defer os.Remove(tempPath)

	cmd := exec.CommandContext(
		ctx,
		"ffmpeg",
		"-i", "pipe:0",
		"-c:v", "libx264",
		"-crf", "28",
		"-preset", "fast",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		"-f", "mp4",
		tempPath,
		"-y",
	)
	cmd.Stdin = reader

	if err = cmd.Run(); err != nil {
		return nil, apierror.NewAPIError(
			http.StatusBadRequest,
			fmt.Errorf("failed to transcode video: %w", err),
		)
	}

	duration, err := probeDuration(ctx, tempPath)
	if err != nil {
		return nil, apierror.NewAPIError(
			http.StatusBadRequest,
			fmt.Errorf("invalid or corrupted video file: %w", err),
		)
	}

	const maxDurationSeconds = 60
	if duration > maxDurationSeconds {
		return nil, apierror.NewAPIError(http.StatusBadRequest, fmt.Errorf(
			"video too long (%.0fs), maximum is %ds",
			duration,
			maxDurationSeconds,
		))
	}

	videoFile, err := os.Open(tempPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open transcoded video: %w", err)
	}
	defer videoFile.Close()

	digester := sha256.New()
	if _, copyErr := io.Copy(digester, videoFile); copyErr != nil {
		return nil, fmt.Errorf("failed to hash transcoded video: %w", copyErr)
	}

	if _, seekErr := videoFile.Seek(0, io.SeekStart); seekErr != nil {
		return nil, fmt.Errorf("failed to seek transcoded video: %w", seekErr)
	}

	videoInfo, err := videoFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat transcoded video: %w", err)
	}
	videoSize := videoInfo.Size()

	uploadID := uuid.New().String()
	tempKey := fmt.Sprintf("temp/%s.mp4", uploadID)
	videoType := "video/mp4"

	//nolint:exhaustruct // too many fields
	_, err = h.storage.S3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        h.bucket,
		Key:           &tempKey,
		Body:          videoFile,
		ContentLength: &videoSize,
		ContentType:   &videoType,
		Metadata:      map[string]string{"original-file-name": header.Filename},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload video: %w", err)
	}

	hashHex := hex.EncodeToString(digester.Sum(nil))
	now := time.Now()
	year, month, _ := now.Date()
	token := uuid.New().String()
	finalKey := fmt.Sprintf(
		"assets/%s/%d/%02d/%s.mp4",
		organizationID.String(),
		year,
		int(month),
		hashHex,
	)

	pendingUpload := &PendingUpload{
		Token:           token,
		OrganizationID:  organizationID,
		TempKey:         tempKey,
		FinalKey:        finalKey,
		Type:            string(util.ProductAssetVideo),
		ContentType:     videoType,
		OriginalName:    header.Filename,
		DurationSeconds: duration,
	}

	cacheData, err := json.Marshal(pendingUpload)
	if err != nil {
		h.deleteTempObject(ctx, tempKey)
		return nil, err
	}

	if cacheErr := h.cache.CachePendingUpload(ctx, token, cacheData); cacheErr != nil {
		h.deleteTempObject(ctx, tempKey)

		return nil, fmt.Errorf("failed to cache pending upload: %w", cacheErr)
	}

	return pendingUpload, nil
}

func (h *V1Handler) deleteTempObject(ctx context.Context, key string) {
	//nolint:mnd // Fixed timeout duration
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := h.storage.DeleteObject(cleanupCtx, *h.bucket, key); err != nil {
		h.logger.WarnContext(
			cleanupCtx,
			"failed to delete temp S3 object",
			slog.String("key", key),
			slog.Any("err", err),
		)
	}
}

func probeDuration(ctx context.Context, path string) (float64, error) {
	cmd := exec.CommandContext(
		ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)

	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to probe transcoded video: %w", err)
	}

	s := strings.TrimSpace(string(out))

	if s == "N/A" {
		return 0, nil
	}

	duration, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("could not parse duration: %w", err)
	}

	return duration, nil
}
