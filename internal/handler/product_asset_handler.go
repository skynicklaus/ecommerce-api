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

	"github.com/skynicklaus/ecommerce-api/internal/apierror"
	"github.com/skynicklaus/ecommerce-api/internal/imageutil"
	"github.com/skynicklaus/ecommerce-api/util"
)

const (
	maxFilesPerUpload = 10
)

type PendingUpload struct {
	Token           string  `json:"token"`
	TempKey         string  `json:"tempKey"`
	FinalKey        string  `json:"finalKey"`
	Type            string  `json:"type"`
	ContentType     string  `json:"contentType"`
	OriginalName    string  `json:"originalName"`
	DurationSeconds float64 `json:"durationSeconds"`
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

	maxMemorySize := h.maxImageSize*(maxFilesPerUpload-1) + h.maxVideoSize
	r.Body = http.MaxBytesReader(w, r.Body, maxMemorySize)
	if err := r.ParseMultipartForm(maxMemorySize); err != nil {
		return apierror.NewAPIError(
			http.StatusRequestEntityTooLarge,
			err,
		)
	}

	headers := r.MultipartForm.File["files"]
	if len(headers) == 0 {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("no files provided"))
	}
	if len(headers) > maxFilesPerUpload {
		return apierror.NewAPIError(http.StatusBadRequest, errors.New("too many files"))
	}

	type result struct {
		index  int
		record *PendingUpload
		err    error
	}

	results := make([]result, len(headers))
	var wg sync.WaitGroup

	for i, header := range headers {
		wg.Add(1)
		go func(i int, header *multipart.FileHeader) {
			defer wg.Done()
			record, err := h.processTempUpload(ctx, header, organization.ID)
			results[i] = result{index: i, record: record, err: err}
		}(i, header)
	}
	wg.Wait()

	var tokens []UploadTokenResponse
	for _, res := range results {
		if res.err != nil {
			for _, other := range results {
				if other.err != nil || other.record == nil {
					continue
				}

				if deleteErr := h.storage.DeleteObject(ctx, *h.bucket, other.record.TempKey); deleteErr != nil {
					h.logger.WarnContext(
						ctx,
						"failed to delete temp object",
						slog.Any("err", deleteErr),
					)
				}
			}

			return apierror.NewAPIError(http.StatusInternalServerError, res.err)
		}

		tokens = append(tokens, UploadTokenResponse{
			Token:        res.record.Token,
			OriginalName: res.record.OriginalName,
			ContentType:  res.record.ContentType,
		})
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
		return nil, fmt.Errorf("type %q is not allowed", contentType)
	}

	full := io.MultiReader(bytes.NewReader(sniff), file)

	switch {
	case strings.HasPrefix(contentType, "image/"):
		return h.processImageUpload(ctx, organizationID, full, header, contentType)
	case strings.HasPrefix(contentType, "video/"):
		if header.Size > h.maxVideoSize {
			return nil, fmt.Errorf("%q exceeds size limit", header.Filename)
		}

		return h.processVideoUpload(ctx, organizationID, full, header)
	default:
		return nil, fmt.Errorf("type %q not supported", contentType)
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
		return nil, fmt.Errorf("%q exceeds size limit", header.Filename)
	}

	buf, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("unable to read from reader: %w", err)
	}

	newBuf, err := imageutil.ValidateAndStrip(buf)
	if err != nil {
		return nil, err
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = h.mime.Extension(contentType)
	}

	tempKey := fmt.Sprintf("temp/%s%s", uuid.New().String(), ext)
	//nolint:exhaustruct // too many fields
	_, err = h.storage.S3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      h.bucket,
		Key:         &tempKey,
		Body:        bytes.NewReader(newBuf),
		ContentType: &contentType,
		Metadata:    map[string]string{"original-file-name": header.Filename},
	})
	if err != nil {
		return nil, err
	}

	digest := sha256.Sum256(buf)
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
		TempKey:         tempKey,
		FinalKey:        finalKey,
		Type:            string(util.ProductAssetImage),
		ContentType:     contentType,
		OriginalName:    header.Filename,
		DurationSeconds: 0,
	}

	cacheData, err := json.Marshal(pendingUpload)
	if err != nil {
		if deleteErr := h.storage.DeleteObject(ctx, *h.bucket, tempKey); deleteErr != nil {
			h.logger.WarnContext(
				ctx,
				"unable to delete temp object",
				slog.Any("err", deleteErr),
			)
		}
		return nil, err
	}

	if cacheErr := h.cache.CachePendingUpload(ctx, token, cacheData); cacheErr != nil {
		if deleteErr := h.storage.DeleteObject(ctx, *h.bucket, tempKey); deleteErr != nil {
			h.logger.WarnContext(
				ctx,
				"unable to delete temp object",
				slog.Any("err", deleteErr),
			)
		}

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
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	digester := sha256.New()

	//nolint:gosec // ffmpeg is trusted system binary
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
		tempFile.Name(),
		"-y",
	)
	cmd.Stdin = io.TeeReader(reader, digester)

	if err = cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to transcode video: %w", err)
	}

	duration, err := probeDuration(ctx, tempFile.Name())
	if err != nil {
		return nil, err
	}

	const maxDurationSeconds = 60
	if duration > maxDurationSeconds {
		return nil, fmt.Errorf(
			"video too long (%.0fs), maximum is %ds",
			duration,
			maxDurationSeconds,
		)
	}

	if _, err = tempFile.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}

	uploadID := uuid.New().String()
	tempKey := fmt.Sprintf("temp/%s.mp4", uploadID)
	videoType := "video/mp4"

	//nolint:exhaustruct // too many fields
	_, err = h.storage.S3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      h.bucket,
		Key:         &tempKey,
		Body:        tempFile,
		ContentType: &videoType,
		Metadata:    map[string]string{"original-file-name": header.Filename},
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
		TempKey:         tempKey,
		FinalKey:        finalKey,
		Type:            string(util.ProductAssetVideo),
		ContentType:     videoType,
		OriginalName:    header.Filename,
		DurationSeconds: duration,
	}

	cacheData, err := json.Marshal(pendingUpload)
	if err != nil {
		if deleteErr := h.storage.DeleteObject(ctx, *h.bucket, tempKey); deleteErr != nil {
			h.logger.WarnContext(
				ctx,
				"unable to delete temp object",
				slog.Any("err", deleteErr),
			)
		}
		return nil, err
	}

	if cacheErr := h.cache.CachePendingUpload(ctx, token, cacheData); cacheErr != nil {
		if deleteErr := h.storage.DeleteObject(ctx, *h.bucket, tempKey); deleteErr != nil {
			h.logger.WarnContext(
				ctx,
				"unable to delete temp object",
				slog.Any("err", deleteErr),
			)
		}

		return nil, fmt.Errorf("failed to cache pending upload: %w", cacheErr)
	}

	return pendingUpload, nil
}

func probeDuration(ctx context.Context, path string) (float64, error) {
	//nolint:gosec // ffprobe is trusted system binary
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
