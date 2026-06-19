package handler

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/storage"
	_ "golang.org/x/image/webp"
)

const (
	imageMetadataMaxDecodePixels = 12_000_000
	imageMetadataMaxScanPixels   = 3_000_000
	imageMetadataMaxColors       = 8
)

func extractImageMetadataFromAsset(ctx context.Context, reader storage.Reader, asset model.AgentAsset) (map[string]any, error) {
	if reader == nil {
		return nil, fmt.Errorf("asset reader unavailable")
	}
	if strings.TrimSpace(asset.Key) == "" {
		return nil, fmt.Errorf("asset key is required")
	}
	if asset.Bytes > maxDriveAssetUploadBytes {
		return map[string]any{
			"content_type":                  normalizeContentType(asset.ContentType),
			"filename":                      asset.Filename,
			"bytes":                         asset.Bytes,
			"pixel_analysis_skipped":        true,
			"pixel_analysis_skip_reason":    "asset exceeds metadata byte budget",
			"metadata_extraction_max_bytes": maxDriveAssetUploadBytes,
		}, nil
	}

	body, err := reader.Open(ctx, asset.Key)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	data, err := io.ReadAll(io.LimitReader(body, maxDriveAssetUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDriveAssetUploadBytes {
		return nil, fmt.Errorf("asset exceeds metadata byte budget")
	}
	return extractImageMetadataFromBytes(data, asset.ContentType, asset.Filename, asset.Bytes)
}

func extractImageMetadataFromBytes(data []byte, contentType, filename string, declaredBytes int64) (map[string]any, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("image bytes are empty")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	width := cfg.Width
	height := cfg.Height
	pixelCount := int64(width) * int64(height)
	metadata := map[string]any{
		"format":       format,
		"content_type": normalizeContentType(contentType),
		"filename":     filename,
		"bytes":        declaredBytes,
		"width":        width,
		"height":       height,
		"pixel_count":  pixelCount,
		"color_model":  fmt.Sprintf("%T", cfg.ColorModel),
	}
	if declaredBytes <= 0 {
		metadata["bytes"] = len(data)
	}
	if width <= 0 || height <= 0 || pixelCount <= 0 {
		metadata["pixel_analysis_skipped"] = true
		metadata["pixel_analysis_skip_reason"] = "image dimensions are invalid"
		return metadata, nil
	}
	if pixelCount > imageMetadataMaxDecodePixels {
		metadata["pixel_analysis_skipped"] = true
		metadata["pixel_analysis_skip_reason"] = "image dimensions exceed in-memory analysis budget"
		metadata["metadata_extraction_max_decode_pixels"] = imageMetadataMaxDecodePixels
		return metadata, nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		metadata["pixel_analysis_skipped"] = true
		metadata["pixel_analysis_skip_reason"] = "image decode failed after config read"
		return metadata, nil
	}
	for key, value := range analyzeImagePixels(img) {
		metadata[key] = value
	}
	return metadata, nil
}

func analyzeImagePixels(img image.Image) map[string]any {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	pixelCount := int64(width) * int64(height)
	step := 1
	if pixelCount > imageMetadataMaxScanPixels {
		step = int(math.Ceil(math.Sqrt(float64(pixelCount) / float64(imageMetadataMaxScanPixels))))
	}
	if step < 1 {
		step = 1
	}

	var total, transparent, semiTransparent, opaque, visible int64
	minX, minY := bounds.Max.X, bounds.Max.Y
	maxX, maxY := bounds.Min.X, bounds.Min.Y
	colors := map[string]*imageColorBucket{}

	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			total++
			c := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
			switch {
			case c.A == 0:
				transparent++
				continue
			case c.A < 255:
				semiTransparent++
			default:
				opaque++
			}
			visible++
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
			key := quantizedColorKey(c)
			bucket := colors[key]
			if bucket == nil {
				bucket = &imageColorBucket{hex: key}
				colors[key] = bucket
			}
			bucket.count++
		}
	}

	out := map[string]any{
		"pixel_analysis_skipped":          false,
		"pixel_analysis_sampled":          step > 1,
		"pixel_analysis_sample_step":      step,
		"analyzed_pixel_sample_count":     total,
		"has_alpha":                       transparent > 0 || semiTransparent > 0,
		"has_transparency":                transparent > 0 || semiTransparent > 0,
		"transparent_pixel_ratio":         ratio(transparent, total),
		"semi_transparent_pixel_ratio":    ratio(semiTransparent, total),
		"opaque_pixel_ratio":              ratio(opaque, total),
		"visible_nontransparent_ratio":    ratio(visible, total),
		"dominant_visible_colors_sampled": dominantVisibleColors(colors, visible),
	}
	if visible > 0 {
		out["visible_bounds"] = map[string]any{
			"x":      minX - bounds.Min.X,
			"y":      minY - bounds.Min.Y,
			"width":  maxX - minX + 1,
			"height": maxY - minY + 1,
		}
	}
	return out
}

type imageColorBucket struct {
	hex   string
	count int64
}

func dominantVisibleColors(colors map[string]*imageColorBucket, visible int64) []map[string]any {
	if visible <= 0 || len(colors) == 0 {
		return nil
	}
	buckets := make([]*imageColorBucket, 0, len(colors))
	for _, bucket := range colors {
		buckets = append(buckets, bucket)
	}
	sort.Slice(buckets, func(i, j int) bool {
		if buckets[i].count == buckets[j].count {
			return buckets[i].hex < buckets[j].hex
		}
		return buckets[i].count > buckets[j].count
	})
	if len(buckets) > imageMetadataMaxColors {
		buckets = buckets[:imageMetadataMaxColors]
	}
	out := make([]map[string]any, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, map[string]any{
			"hex":   bucket.hex,
			"ratio": ratio(bucket.count, visible),
		})
	}
	return out
}

func quantizedColorKey(c color.NRGBA) string {
	return fmt.Sprintf("#%02X%02X%02X", quantizeColor(c.R), quantizeColor(c.G), quantizeColor(c.B))
}

func quantizeColor(v uint8) uint8 {
	q := int(v)/32*32 + 16
	if q > 255 {
		q = 255
	}
	return uint8(q)
}

func ratio(part, total int64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round((float64(part)/float64(total))*10_000) / 10_000
}

func normalizeContentType(contentType string) string {
	return strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
}
