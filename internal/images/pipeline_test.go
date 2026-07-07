package images

import (
	"image"
	"image/color"
	"testing"
)

// patternImg — 64x64 картинка с заданным паттерном. Вертикальный сплит и
// шахматка дают заметно разные перцептивные хэши (проверено: расстояние 18),
// поэтому годятся для теста дедупа.
func patternImg(checker bool) image.Image {
	const n = 64
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			on := x < n/2 // вертикальный сплит
			if checker {
				on = (x/8+y/8)%2 == 0
			}
			c := color.RGBA{0, 0, 0, 255}
			if on {
				c = color.RGBA{255, 255, 255, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

func mustHashPhoto(t *testing.T, img image.Image) photo {
	t.Helper()
	h, err := Hash(img)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return photo{hash: h}
}

func TestDedup(t *testing.T) {
	a := mustHashPhoto(t, patternImg(false)) // вертикальный сплит
	b := mustHashPhoto(t, patternImg(false)) // такой же → дубль a
	c := mustHashPhoto(t, patternImg(true))  // шахматка → другой

	out := dedup([]photo{a, b, c})
	if len(out) != 2 {
		t.Fatalf("ожидал 2 кадра после дедупа (a,c), получил %d", len(out))
	}
}

func TestSplitCoverGallery(t *testing.T) {
	photos := []photo{
		{hasFace: false, outdoor: 0.5, w: 720, h: 1280}, // портрет
		{hasFace: false, outdoor: 0.6, w: 1280, h: 720}, // ландшафт — лучшая обложка
		{hasFace: true, outdoor: 0.9, w: 1280, h: 720},  // с лицом — не обложка
		{hasFace: false, outdoor: 0.4, w: 720, h: 1280}, // портрет
	}
	cover, gallery := splitCoverGallery(photos, 10)

	if cover == nil || cover.w != 1280 || cover.hasFace {
		t.Fatalf("обложка — ландшафтный экстерьер без лица, получил %+v", cover)
	}
	if len(gallery) != 3 {
		t.Fatalf("галерея = все кроме обложки (3), получил %d", len(gallery))
	}
	for _, g := range gallery {
		if !g.hasFace && g.w == 1280 && g.outdoor == 0.6 {
			t.Errorf("обложка не должна попадать в галерею")
		}
	}
}

func TestOrder(t *testing.T) {
	in := []photo{
		{hasFace: true, outdoor: 0.9},  // с лицом → в конец, несмотря на высокий score
		{hasFace: false, outdoor: 0.1}, // интерьер
		{hasFace: false, outdoor: 0.8}, // экстерьер → должен стать обложкой
	}
	out := order(in)

	if out[0].outdoor != 0.8 || out[0].hasFace {
		t.Errorf("обложка должна быть экстерьером без лица, получил %+v", out[0])
	}
	if !out[len(out)-1].hasFace {
		t.Errorf("кадр с лицом должен быть последним, получил %+v", out[len(out)-1])
	}
	// первые кадры — без лиц
	for i := 0; i < 2; i++ {
		if out[i].hasFace {
			t.Errorf("кадр %d не должен содержать лицо", i)
		}
	}
}
