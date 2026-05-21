package imageutil

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"github.com/deepteams/webp"
)

const (
	minImageDimension = 300
	maxPixels         = 50_000_000
	mp                = 1_000_000
)

// ValidateAndStrip validates the image dimensions and limits, strips metadata
// by decoding and re-encoding the image. It returns the processed buffer and an error if validation fails.
func ValidateAndStrip(buf []byte) ([]byte, error) {
	// 1. Decode config first to check dimensions without loading entire image pixels
	cfg, format, err := image.DecodeConfig(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("invalid or corrupted image: %w", err)
	}

	w, h := cfg.Width, cfg.Height
	if w < minImageDimension || h < minImageDimension {
		return nil, fmt.Errorf(
			"image too small (%dx%d), minimum is %dpx on shortest side",
			w,
			h,
			minImageDimension,
		)
	}

	if w*h > maxPixels {
		return nil, fmt.Errorf(
			"image too large (%dx%d), maximum is %d megapixels",
			w,
			h,
			maxPixels/mp,
		)
	}

	// 2. Decode the image full pixels to perform sanitization (metadata stripping) by re-encoding
	img, _, err := image.Decode(bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image for processing: %w", err)
	}

	var out bytes.Buffer

	switch format {
	case "jpeg":
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 90})
	case "png":
		err = png.Encode(&out, img)
	case "webp":
		err = webp.Encode(&out, img, &webp.EncoderOptions{
			Lossless: true,
		})
	default:
		return nil, fmt.Errorf("unsupported image format: %s", format)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to re-encode image: %w", err)
	}

	return out.Bytes(), nil
}
