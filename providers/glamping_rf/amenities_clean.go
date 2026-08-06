package glamping_rf

import "strings"

// Источник отдаёт amenityFeature плоским списком, смешивая РУБРИКИ каталога с
// их значениями: рядом с «Мангальная зона» лежат «Территория», «Развлечения»,
// «Дети». В карточке объекта это читается как список удобств, и гость видит
// «Дети» и «Общие» среди бани и рыбалки.
//
// Проверено на выгрузке 318 объектов: «Территория» у 313, «Развлечения» у 262,
// «Дети» у 194 — рубрики есть почти у всех, настоящие удобства так не
// распределены.
var amenityRubrics = map[string]bool{
	"территория":  true,
	"развлечения": true,
	"дети":        true,
	"общие":       true,
	"общее":       true,
	"прочее":      true,
	// Рубрика: рядом всегда лежат конкретные «WI-FI» (289 из 300) и
	// «Мобильный интернет», а сама строка ничего не добавляет.
	"интернет": true,
	// Рубрика «можно ли с питомцем». Её значения приходят отдельными строками
	// и без неё читаются сами: «Можно с Питомцем» — у 268 объектов.
	"домашние животные": true,
}

// Значения рубрики о питомцах: в общем списке теряют вопрос и превращаются в
// «удобство» под названием «Нельзя». Проверено: все 38 объектов с этой строкой
// имеют petsAllowed=false, то есть смысл уже разобран в отдельное поле.
var amenityOrphanValues = map[string]bool{
	"нельзя": true,
	"можно только с согласованием": true,
}

// Разные написания одного и того же. Только бесспорные случаи: «Баня» и
// «Сауна» здесь намеренно НЕ склеены — это разные вещи, как и общая кухня
// против собственной.
var amenityCanonical = map[string]string{
	"wi-fi":         "Wi-Fi",
	"wifi":          "Wi-Fi",
	"микроволновка": "Микроволновая печь",
}

// cleanAmenities убирает рубрики и осиротевшие значения, сводит написания и
// снимает дубли, сохраняя исходный порядок.
func cleanAmenities(names []string) []string {
	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if amenityRubrics[key] || amenityOrphanValues[key] {
			continue
		}
		if canonical, ok := amenityCanonical[key]; ok {
			name = canonical
			key = strings.ToLower(canonical)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
