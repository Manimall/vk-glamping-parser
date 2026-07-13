package glamping_rf

import (
	"context"
	"strings"
	"testing"

	"vk-parser/internal/contract"
)

func leanObject() contract.Object {
	return toObject(apiItem{
		ID: 7, Name: "Тест-глэмп",
		Price:  apiPrice{Formatted: "9 000 ₽"},
		Place:  apiPlace{Name: "Тверская область"},
		Images: []apiImage{{Webp: "list-1.webp"}},
	})
}

func fullDetail() *detailData {
	return &detailData{
		Description: "Богатое описание с детальной страницы.",
		CheckIn:     "14:00", CheckOut: "12:00",
		Rating: "5.0", Reviews: 41,
		Photos:    []string{"d1.webp", "d2.webp", "d3.webp"},
		Amenities: []string{"Интернет", "Парковка"},
		Rules:     []string{"Бесплатная отмена за 7 дней."},
		Guests:    6, Area: "80 м²",
	}
}

func TestMergeDetail(t *testing.T) {
	obj := leanObject()
	mergeDetail(&obj, fullDetail())

	if obj.About != "Богатое описание с детальной страницы." {
		t.Errorf("about: %q", obj.About)
	}
	if len(obj.Photos) != 3 { // detail-галерея заменила 1 превью списка
		t.Errorf("photos: %v", obj.Photos)
	}
	cp := obj.Cabins[0].Property
	if cp.Summary != obj.About {
		t.Errorf("summary: %q", cp.Summary)
	}
	// Факты: гости, площадь, заезд, выезд, рейтинг — все реальные.
	if len(cp.Facts) != 5 || cp.Facts[0].Value != "до 6" {
		t.Errorf("facts: %+v", cp.Facts)
	}
	if len(cp.Rules) != 1 || cp.Rules[0] != "Бесплатная отмена за 7 дней." {
		t.Errorf("rules: %v", cp.Rules)
	}
	// Удобства: услуги списка (нет) + amenityFeature (2), без дублей.
	if len(cp.AmenityGroups) != 1 || len(cp.AmenityGroups[0].Items) != 2 {
		t.Errorf("amenities: %+v", cp.AmenityGroups)
	}
	// SEO пересобран из богатого описания (питч после «Имя — » идёт со строчной).
	if obj.Seo == nil || !strings.Contains(obj.Seo.Description, "богатое описание") {
		t.Errorf("seo: %+v", obj.Seo)
	}
	// Данные списка не затёрты.
	if obj.Cabins[0].Price != "9 000 ₽" || obj.Location != "Тверская область" {
		t.Errorf("данные списка потеряны: %+v", obj.Cabins[0])
	}
}

func TestApplyDefaults_OnlyFillsGaps(t *testing.T) {
	// Пустой объект: дефолты добавляют заезд/выезд, гостей и базовые правила.
	lean := leanObject()
	applyDefaults(&lean)
	cp := lean.Cabins[0].Property
	if len(cp.Facts) != 3 { // Заезд, Выезд, Гостей
		t.Fatalf("дефолт-факты: %+v", cp.Facts)
	}
	if len(cp.Rules) != len(defaultRules) {
		t.Fatalf("дефолт-правила: %v", cp.Rules)
	}

	// Обогащённый объект: дефолты НЕ трогают реальные данные.
	rich := leanObject()
	mergeDetail(&rich, fullDetail())
	applyDefaults(&rich)
	rcp := rich.Cabins[0].Property
	if len(rcp.Facts) != 5 { // остались detail-факты, дефолты не добавились
		t.Errorf("дефолты затёрли реальные факты: %+v", rcp.Facts)
	}
	if rcp.Rules[0] != "Бесплатная отмена за 7 дней." {
		t.Errorf("дефолты затёрли реальные правила: %v", rcp.Rules)
	}
}

func TestParse_EnrichesAndAppliesDefaults(t *testing.T) {
	f := &fakeFetcher{
		pages: map[int][]*apiResponse{
			75: {{Items: items(1, 2), HasMore: false}},
		},
		details: map[int]*detailData{1: fullDetail()}, // у 2 detail «падает»
	}
	p := newTestProvider(f, []direction{{name: "ЗК", places: []int{75}}}, 100)

	out, err := p.Parse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("объектов: %d", len(out))
	}
	// Объект 1 обогащён detail-страницей.
	if out[0].About != "Богатое описание с детальной страницы." {
		t.Errorf("объект 1 не обогащён: %q", out[0].About)
	}
	// Объект 2: detail упал → данные списка + дефолты (graceful).
	cp2 := out[1].Cabins[0].Property
	if len(cp2.Rules) != len(defaultRules) || len(cp2.Facts) != 3 {
		t.Errorf("объект 2 без дефолтов: rules=%v facts=%+v", cp2.Rules, cp2.Facts)
	}
}
