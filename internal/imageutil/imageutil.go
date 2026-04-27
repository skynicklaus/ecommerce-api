package imageutil

import (
	"fmt"

	"github.com/davidbyttow/govips/v2/vips"
)

const (
	minImageDimension = 300
	maxPixels         = 50_000_000
	mp                = 1_000_000
)

func ValidateAndStrip(buf []byte) ([]byte, error) {
	img, err := vips.NewImageFromBuffer(buf)
	if err != nil {
		return nil, fmt.Errorf("invalid or corrupted image: %w", err)
	}
	defer img.Close()

	w, h := img.Width(), img.Height()
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

	err = img.RemoveMetadata()
	if err != nil {
		return nil, fmt.Errorf("failed to remove image metadata: %w", err)
	}

	out, _, err := img.ExportNative()
	if err != nil {
		return nil, fmt.Errorf("failed to export image: %w", err)
	}

	return out, nil
}
