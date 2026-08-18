package imageproc

import (
	"image"
	"io"

	"github.com/gen2brain/heic"
)

func decodeHEIC(r io.Reader) (image.Image, error) {
	return heic.Decode(r)
}
