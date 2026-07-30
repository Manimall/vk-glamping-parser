package glamping_rf

import (
	"context"
	"strings"
	"testing"

	"vk-parser/internal/contract"
	"vk-parser/internal/extract"
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
	// Адресные поля обогащение не трогает. Гард на будущее: detail-страница
	// содержит адрес свободным текстом, и соблазн написать здесь
	// `obj.Locality = d.StreetAddress` появится ровно при взятии этой задачи.
	// Адрес там противоречив (объекты с регионом «Тульская область» при адресе
	// «Магаданская область»), поэтому разрешать конфликт надо явной политикой,
	// а не присваиванием по дороге.
	if obj.Region != "Тверская область" {
		t.Errorf("region затёрт обогащением: %q", obj.Region)
	}
	if obj.Locality != "" {
		t.Errorf("locality=%q — заполнен адресом с detail-страницы без политики разбора", obj.Locality)
	}
}

func TestApplyDefaults_OnlyFillsGaps(t *testing.T) {
	// Пустой объект: дефолты добавляют заезд/выезд, гостей и базовые правила.
	lean := leanObject()
	applyDefaults(&lean)
	cp := lean.Cabins[0].Property
	// Только заезд и выезд: они в отрасли действительно стандартны. Вместимость
	// в дефолтах БОЛЬШЕ НЕТ — «до 4 гостей» подставлялось почти всему каталогу
	// при реальном разбросе от 2 до 27, и гость ехал вчетвером туда, где одна
	// двуспальная кровать.
	if len(cp.Facts) != 2 {
		t.Fatalf("дефолт-факты: %+v", cp.Facts)
	}
	for _, f := range cp.Facts {
		if f.Label == "Гостей" {
			t.Errorf("вместимость снова выдумывается: %+v", f)
		}
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
	p := newTestProvider(f, []direction{{name: "ЗК", places: []int{75}}})

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
	// Объект 2: detail упал ТРАНЗИЕНТНО → оставлен с данными списка + дефолты.
	cp2 := out[1].Cabins[0].Property
	if len(cp2.Rules) != len(defaultRules) || len(cp2.Facts) != 2 {
		t.Errorf("объект 2 без дефолтов: rules=%v facts=%+v", cp2.Rules, cp2.Facts)
	}
}

// TestParse_DropsDelistedObjects: объект, чья detail-страница отдаёт 404 (снят
// с каталога-источника), исключается из выдачи целиком — мёртвые источники не
// показываем (продуктовое решение). Транзиентный сбой при этом НЕ дропает.
func TestParse_DropsDelistedObjects(t *testing.T) {
	f := &fakeFetcher{
		pages: map[int][]*apiResponse{
			75: {{Items: items(1, 2, 3), HasMore: false}},
		},
		details: map[int]*detailData{1: fullDetail()}, // 2 — транзиентный сбой
		gone:    map[int]bool{3: true},                // 3 — снят с каталога
	}
	p := newTestProvider(f, []direction{{name: "ЗК", places: []int{75}}})

	out, err := p.Parse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("снятый объект должен быть исключён: ожидал 2, получил %d", len(out))
	}
	for _, o := range out {
		if o.Slug == "obj-3" {
			t.Errorf("объект 3 (404) не должен попасть в выдачу")
		}
	}
}

// Вместимость обязана доехать до объект-уровня числом: по строке «до 6» внутри
// facts каталог не отфильтруешь, а фильтр «нас шестеро» — самый ходовой.
func TestMergeDetail_GuestsMax(t *testing.T) {
	obj := contract.Object{Cabins: []contract.Cabin{{Property: &extract.Property{}}}}
	mergeDetail(&obj, &detailData{Guests: 6})
	if obj.GuestsMax != 6 {
		t.Errorf("GuestsMax = %d, ожидали 6", obj.GuestsMax)
	}
}

// Молчание источника не должно превращаться в ноль-как-ответ: omitempty уберёт
// поле, и сайт покажет «неизвестно» вместо выдуманного числа.
func TestMergeDetail_GuestsMaxAbsentWhenUnknown(t *testing.T) {
	obj := contract.Object{Cabins: []contract.Cabin{{Property: &extract.Property{}}}}
	mergeDetail(&obj, &detailData{})
	if obj.GuestsMax != 0 {
		t.Errorf("GuestsMax = %d, ожидали 0 (поле опустится)", obj.GuestsMax)
	}
}
