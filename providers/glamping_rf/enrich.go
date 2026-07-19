package glamping_rf

// Слияние detail-данных в contract.Object + дефолты. Иерархия честности (MVP):
//   1) реальное со страницы объекта (описание, заезд/выезд, фото, правила, рейтинг);
//   2) реальное из списка (уже в объекте: цена, контакты, координаты);
//   3) синтетические СТРУКТУРНЫЕ дефолты — ТОЛЬКО здесь, в applyDefaults, с
//      пометкой: заменить реальными данными, когда появятся. Цены/контакты не
//      выдумываем никогда.

import (
	"fmt"

	"vk-parser/internal/contract"
	"vk-parser/internal/extract"
)

// Дефолты MVP (см. applyDefaults). Типовые для глэмпингов значения.
const (
	defaultCheckIn  = "15:00"
	defaultCheckOut = "12:00"
	defaultGuests   = 4
)

// defaultRules — базовые правила, если у объекта не нашлось своих (FAQ пуст).
var defaultRules = []string{
	"Тишина на территории после 23:00.",
	"Курение в домиках запрещено.",
}

// mergeDetail дополняет объект данными detail-страницы. Ничего не затирает:
// поля списка (цена, контакты, координаты) остаются, detail только добавляет.
func mergeDetail(obj *contract.Object, d *detailData) {
	if d.Description != "" {
		obj.About = d.Description
	}
	if len(d.Photos) > 0 {
		obj.Photos = d.Photos // полная галерея вместо 4 превью списка
	}
	if len(d.Extras) > 0 {
		obj.Extras = d.Extras // платные услуги (баня/чан/питомец) с ценой
	}
	// Точная точка объекта из placemark карты перекрывает координаты списка
	// (те часто указывают на центр города — метка «уезжает» от объекта).
	if d.Lat != 0 && d.Lng != 0 {
		obj.Coords = &contract.Coords{Lat: d.Lat, Lon: d.Lng}
	}
	if len(obj.Cabins) == 0 {
		return
	}
	cp := obj.Cabins[0].Property
	if cp == nil {
		return
	}
	if d.Description != "" {
		cp.Summary = d.Description
	}
	cp.Facts = detailFacts(d)
	if len(d.Rules) > 0 {
		cp.Rules = d.Rules
	}
	if len(d.Amenities) > 0 {
		cp.AmenityGroups = mergeAmenities(cp.AmenityGroups, d.Amenities)
	}
	rebuildSEO(obj)
}

// rebuildSEO пересобирает SEO после обогащения: описание стало богаче,
// а BuildSEO берёт «питч» именно из About. Общий шаг для обоих источников
// обогащения (detail-страница агрегатора / собственный сайт объекта).
func rebuildSEO(obj *contract.Object) {
	if len(obj.Cabins) == 0 {
		return
	}
	seo := extract.BuildSEO(extract.SEOInput{
		Name: obj.Cabins[0].Title, Location: obj.Location, About: obj.About,
	})
	obj.Seo = &seo
}

// detailFacts — блок фактов из реальных данных страницы (пустые опускаются).
func detailFacts(d *detailData) []extract.Fact {
	var facts []extract.Fact
	add := func(label, value string) {
		if value != "" {
			facts = append(facts, extract.Fact{Label: label, Value: value})
		}
	}
	if d.Guests > 0 {
		add("Гостей", fmt.Sprintf("до %d", d.Guests))
	}
	add("Площадь", d.Area)
	add("Заезд", d.CheckIn)
	add("Выезд", d.CheckOut)
	if d.Rating != "" && d.Reviews > 0 {
		add("Рейтинг", fmt.Sprintf("%s · %d отзывов", d.Rating, d.Reviews))
	}
	return facts
}

// mergeAmenities добавляет категории amenityFeature к услугам списка (дедуп).
func mergeAmenities(groups []extract.AmenityGroup, names []string) []extract.AmenityGroup {
	seen := make(map[string]bool)
	var items []string
	take := func(vals []string) {
		for _, v := range vals {
			if v != "" && !seen[v] {
				seen[v] = true
				items = append(items, v)
			}
		}
	}
	for _, g := range groups {
		take(g.Items)
	}
	take(names)
	if len(items) == 0 {
		return nil
	}
	return []extract.AmenityGroup{{Title: amenitiesGroupTitle, Items: items}}
}

// applyDefaults — ЕДИНСТВЕННОЕ место синтетики (MVP-прототип, показательный):
// структурные дефолты для полей, которых нет ни на странице, ни в списке.
// TODO: заменять реальными данными по мере появления источников.
func applyDefaults(obj *contract.Object) {
	if len(obj.Cabins) == 0 || obj.Cabins[0].Property == nil {
		return
	}
	cp := obj.Cabins[0].Property

	hasFact := func(label string) bool {
		for _, f := range cp.Facts {
			if f.Label == label {
				return true
			}
		}
		return false
	}
	if !hasFact("Заезд") {
		cp.Facts = append(cp.Facts,
			extract.Fact{Label: "Заезд", Value: defaultCheckIn},
			extract.Fact{Label: "Выезд", Value: defaultCheckOut},
		)
	}
	if !hasFact("Гостей") {
		cp.Facts = append(cp.Facts, extract.Fact{Label: "Гостей", Value: fmt.Sprintf("до %d", defaultGuests)})
	}
	if len(cp.Rules) == 0 {
		cp.Rules = defaultRules
	}
}
