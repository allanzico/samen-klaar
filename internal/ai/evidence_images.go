package ai

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"math"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

var ErrEvidenceImage = errors.New("the photo could not be prepared for comparison; use a smaller JPEG, PNG or WebP image")

// The API's per-image limit is measured after base64 encoding. Seven MiB of
// binary content fits below ten MB encoded. Large review copies are resized;
// this never writes to evidence storage or mutates the original upload.
const maxReviewImageBytes = 7 << 20

func prepareReviewAttachment(contentType string, data []byte) (Attachment, error) {
	if contentType == "application/pdf" {
		return Attachment{ContentType: contentType, Data: data}, nil
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 {
		return Attachment{}, ErrEvidenceImage
	}
	if len(data) <= maxReviewImageBytes && config.Width <= 8000 && config.Height <= 8000 {
		return Attachment{ContentType: contentType, Data: data}, nil
	}
	// Bound decompression before allocating a decoded image.
	if int64(config.Width)*int64(config.Height) > 32_000_000 {
		return Attachment{}, ErrEvidenceImage
	}
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Attachment{}, ErrEvidenceImage
	}
	scale := math.Min(1, 1568/float64(max(config.Width, config.Height)))
	width, height := max(1, int(math.Round(float64(config.Width)*scale))), max(1, int(math.Round(float64(config.Height)*scale)))
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(target, target.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	xdraw.ApproxBiLinear.Scale(target, target.Bounds(), source, source.Bounds(), draw.Over, nil)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, target, &jpeg.Options{Quality: 90}); err != nil || encoded.Len() > maxReviewImageBytes {
		return Attachment{}, ErrEvidenceImage
	}
	return Attachment{ContentType: "image/jpeg", Data: encoded.Bytes()}, nil
}
