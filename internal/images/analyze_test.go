package images

import (
	"image"
	"image/color"
	"testing"
)

// solidImg — сплошная заливка одним цветом (64x64).
func solidImg(c color.RGBA) image.Image {
	const n = 64
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestOutdoorScore(t *testing.T) {
	green := solidImg(color.RGBA{40, 180, 40, 255})  // листва
	sky := solidImg(color.RGBA{90, 130, 220, 255})   // синее небо
	wall := solidImg(color.RGBA{240, 240, 240, 255}) // белая стена (интерьер)

	gs, ss, ws := OutdoorScore(green), OutdoorScore(sky), OutdoorScore(wall)

	if gs < 0.9 {
		t.Errorf("зелёный кадр должен набрать высокий score, получил %.2f", gs)
	}
	if ss < 0.9 {
		t.Errorf("небо должно набрать высокий score, получил %.2f", ss)
	}
	if ws > 0.1 {
		t.Errorf("белая стена должна набрать низкий score, получил %.2f", ws)
	}
	// Экстерьер обязан обходить интерьер — на этом строится выбор обложки.
	if gs <= ws || ss <= ws {
		t.Errorf("экстерьер (%.2f/%.2f) должен быть выше интерьера (%.2f)", gs, ss, ws)
	}
}

func TestHasFaceNoFalsePositive(t *testing.T) {
	// На сплошных заливках лиц нет — детектор не должен «находить» их.
	// (истинно-положительный случай требует реального фото и здесь не проверяется).
	for _, c := range []color.RGBA{
		{40, 180, 40, 255},   // зелень
		{240, 240, 240, 255}, // белая стена
		{10, 10, 10, 255},    // тёмный кадр
	} {
		if HasFace(solidImg(c)) {
			t.Errorf("ложное срабатывание детекции лица на заливке %v", c)
		}
	}
}
