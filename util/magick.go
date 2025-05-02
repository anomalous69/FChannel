package util

import (
	"os/exec"

	"github.com/anomalous69/fchannel/config"
)

var MagickBinary string

func init() {
	// Check for magick binary (ImageMagick 7+)
	if _, err := exec.LookPath("magick"); err == nil {
		MagickBinary = "magick"
		return
	}

	// Check for convert binary
	if _, err := exec.LookPath("convert"); err == nil {
		MagickBinary = "convert"
		return
	}

	// We should stop the server here as ImageMagick is required for captchas and thumbnails
	config.Log.Fatal("ImageMagick not detected in path (convert or magick). Please install ImageMagick and add it to your PATH.")
}
