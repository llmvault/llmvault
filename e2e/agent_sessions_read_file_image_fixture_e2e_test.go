package e2e

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

func agentSessionsWriteImageFixture(t *testing.T, path, imageText string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1200, 620))
	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	black := color.RGBA{A: 255}
	border := color.RGBA{R: 24, G: 24, B: 24, A: 255}
	for x := 30; x < 1170; x++ {
		for dy := 0; dy < 6; dy++ {
			img.Set(x, 30+dy, border)
			img.Set(x, 584+dy, border)
		}
	}
	for y := 30; y < 590; y++ {
		for dx := 0; dx < 6; dx++ {
			img.Set(30+dx, y, border)
			img.Set(1164+dx, y, border)
		}
	}
	drawScaledBasicText(img, "HIVY IMAGE READ E2E", 92, 145, 7, black)
	drawScaledBasicText(img, imageText, 118, 300, 5, black)
	drawScaledBasicText(img, "LIVE DESCRIBE MODEL SHOULD READ THIS PNG", 96, 430, 4, black)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create image fixture: %v", err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode image fixture: %v", err)
	}
}

func drawScaledBasicText(dst *image.RGBA, text string, x, y, scale int, c color.Color) {
	face := basicfont.Face7x13
	width := font.MeasureString(face, text).Ceil()
	height := face.Metrics().Height.Ceil()
	glyph := image.NewRGBA(image.Rect(0, 0, width, height))
	d := font.Drawer{
		Dst:  glyph,
		Src:  &image.Uniform{C: c},
		Face: face,
		Dot:  fixed.P(0, face.Metrics().Ascent.Ceil()),
	}
	d.DrawString(text)
	for gy := 0; gy < height; gy++ {
		for gx := 0; gx < width; gx++ {
			_, _, _, a := glyph.At(gx, gy).RGBA()
			if a == 0 {
				continue
			}
			for sy := 0; sy < scale; sy++ {
				for sx := 0; sx < scale; sx++ {
					dst.Set(x+gx*scale+sx, y+gy*scale+sy, c)
				}
			}
		}
	}
}
