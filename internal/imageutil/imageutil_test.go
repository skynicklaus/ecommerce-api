package imageutil

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/deepteams/webp"
	"github.com/stretchr/testify/require"
)

func createTestImage(w, h int, format string) ([]byte, error) {
	rect := image.Rect(0, 0, w, h)
	img := image.NewRGBA(rect)
	draw.Draw(img, rect, &image.Uniform{color.RGBA{255, 0, 0, 255}}, image.Point{}, draw.Src)

	var buf bytes.Buffer
	var err error
	switch format {
	case "jpeg":
		err = jpeg.Encode(&buf, img, nil)
	case "png":
		err = png.Encode(&buf, img)
	case "webp":
		err = webp.Encode(&buf, img, &webp.EncoderOptions{
			Lossless: true,
			Quality:  75,
		})
	default:
		return nil, fmt.Errorf("unsupported format in test helper")
	}
	return buf.Bytes(), err
}

// injectEXIF inserts an APP1 (EXIF) segment after the SOI marker of a JPEG so we can
// verify ValidateAndStrip removes it.
func injectEXIF(jpegBytes []byte) []byte {
	exifSig := []byte("Exif\x00\x00")
	fakeData := []byte{0x01, 0x02, 0x03, 0x04}
	segmentLen := 2 + len(exifSig) + len(fakeData)

	out := make([]byte, 0, len(jpegBytes)+4+segmentLen)
	out = append(out, jpegBytes[:2]...)
	out = append(out, 0xFF, 0xE1)
	out = append(out, byte(segmentLen>>8), byte(segmentLen))
	out = append(out, exifSig...)
	out = append(out, fakeData...)
	out = append(out, jpegBytes[2:]...)
	return out
}

func TestValidateAndStrip(t *testing.T) {
	t.Run("valid jpeg", func(t *testing.T) {
		buf, err := createTestImage(300, 300, "jpeg")
		require.NoError(t, err)

		out, err := ValidateAndStrip(buf)
		require.NoError(t, err)
		require.NotEmpty(t, out)
	})

	t.Run("valid webp", func(t *testing.T) {
		buf, err := createTestImage(300, 300, "webp")
		require.NoError(t, err)

		out, err := ValidateAndStrip(buf)
		require.NoError(t, err)
		require.NotEmpty(t, out)
	})

	t.Run("too small", func(t *testing.T) {
		buf, err := createTestImage(299, 300, "jpeg")
		require.NoError(t, err)

		_, err = ValidateAndStrip(buf)
		require.Error(t, err)
	})

	t.Run("too large", func(t *testing.T) {
		buf, err := createTestImage(8000, 8000, "jpeg")
		require.NoError(t, err)

		_, err = ValidateAndStrip(buf)
		require.Error(t, err)
	})

	t.Run("valid png", func(t *testing.T) {
		buf, err := createTestImage(300, 300, "png")
		require.NoError(t, err)

		out, err := ValidateAndStrip(buf)
		require.NoError(t, err)
		require.NotEmpty(t, out)
	})

	t.Run("strips jpeg metadata", func(t *testing.T) {
		base, err := createTestImage(300, 300, "jpeg")
		require.NoError(t, err)
		tagged := injectEXIF(base)

		require.True(t, bytes.Contains(tagged, []byte("Exif")), "test setup: injected EXIF marker not found")

		out, err := ValidateAndStrip(tagged)
		require.NoError(t, err)
		require.False(t, bytes.Contains(out, []byte("Exif")), "output still contains EXIF metadata after strip")

		_, _, decodeErr := image.DecodeConfig(bytes.NewReader(out))
		require.NoError(t, decodeErr, "output is not a valid image")
	})

	t.Run("corrupted input", func(t *testing.T) {
		_, err := ValidateAndStrip([]byte("this is not an image"))
		require.Error(t, err)
	})

	t.Run("truncated header", func(t *testing.T) {
		pngSignature := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
		_, err := ValidateAndStrip(pngSignature)
		require.Error(t, err)
	})
}
