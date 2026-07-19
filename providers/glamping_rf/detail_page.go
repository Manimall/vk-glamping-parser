package glamping_rf

// Маркеры вёрстки detail-страницы: полное описание, площадь, вместимость,
// фото галереи, точная точка карты (ymaps.Placemark).

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// maxDetailPhotos — предел галереи (как у курируемых объектов фронта).
const maxDetailPhotos = 15

// capacityRe — «Вместимость: 4 + 2 гостя» (доп. места опциональны).
var capacityRe = regexp.MustCompile(`Вместимость:\s*(\d+)(?:\s*\+\s*(\d+))?\s*гост`)

// areaRe — легаси-формат «Площадь: 80 м²» (в тексте бывает узкий пробел  ).
var areaRe = regexp.MustCompile(`Площадь:\s*([\d.,]+)\s*м²`)

// areaAltRe — площадь варианта дома в характеристиках новой вёрстки:
// <img ... alt="Площадь"> 165 м². У комплекса таких блоков несколько (по дому).
var areaAltRe = regexp.MustCompile(`alt="Площадь">\s*(\d+)\s*м`)

// descFullRe — блок ПОЛНОГО описания в вёрстке (data-pv12-desc-full). В ld+json
// LodgingBusiness сайт кладёт обрезанные ~300 символов (meta-описание) — полный
// текст объекта есть только в этом блоке. (?s) — текст многострочный.
var descFullRe = regexp.MustCompile(`(?s)data-pv12-desc-full[^>]*>(.*?)</div>`)

// placemarkRe — точная точка объекта на Яндекс-карте страницы:
// `new ymaps.Placemark([56.773469, 38.874880], …)`. Именно её показывает
// источник (точнее координат списка, часто указывающих на центр города).
var placemarkRe = regexp.MustCompile(`Placemark\(\s*\[\s*([-\d.]+)\s*,\s*([-\d.]+)\s*\]`)

// fullDescription — полный текст описания из блока вёрстки data-pv12-desc-full
// (счищаем теги и entities, схлопываем пробелы). Блок встречается на странице
// дважды (десктоп/мобайл) — берём первый непустой. Нет блока — пустая строка
// (останется описание из ld+json).
func fullDescription(page string) string {
	for _, m := range descFullRe.FindAllStringSubmatch(page, -1) {
		text := html.UnescapeString(tagRe.ReplaceAllString(m[1], " "))
		text = strings.TrimSpace(spacesRe.ReplaceAllString(text, " "))
		if text != "" {
			return text
		}
	}
	return ""
}

// detailArea — площадь объекта из характеристик страницы. У комплекса несколько
// домов с разной площадью → «от {минимум} м²» (как цена «от N ₽»); один дом
// (все значения равны) → «{N} м²». Формат alt="Площадь"> N м (новая вёрстка) с
// фоллбэком на легаси «Площадь: N м²». Нет данных — пустая строка.
func detailArea(page string) string {
	var vals []int
	for _, m := range areaAltRe.FindAllStringSubmatch(page, -1) {
		if v, _ := strconv.Atoi(m[1]); v > 0 {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		if m := areaRe.FindStringSubmatch(page); m != nil {
			return m[1] + " м²"
		}
		return ""
	}
	min, allEqual := vals[0], true
	for _, v := range vals[1:] {
		if v < min {
			min = v
		}
		if v != vals[0] {
			allEqual = false
		}
	}
	if allEqual {
		return fmt.Sprintf("%d м²", min)
	}
	return fmt.Sprintf("от %d м²", min)
}

// detailPlacemark — координаты метки объекта из карты страницы (lat, lng).
func detailPlacemark(page string) (lat, lng float64, ok bool) {
	m := placemarkRe.FindStringSubmatch(page)
	if m == nil {
		return 0, 0, false
	}
	lat, err1 := strconv.ParseFloat(m[1], 64)
	lng, err2 := strconv.ParseFloat(m[2], 64)
	if err1 != nil || err2 != nil || lat == 0 || lng == 0 {
		return 0, 0, false
	}
	return lat, lng, true
}

// detailPhotos — уникальные webp-кадры галереи объекта в порядке появления.
func detailPhotos(page string, id int) []string {
	re := regexp.MustCompile(`https://[^\s"')]+/image/cachewebp/catalog/` + strconv.Itoa(id) + `/[^\s"')]+\.webp`)
	seen := make(map[string]bool)
	var out []string
	for _, u := range re.FindAllString(page, -1) {
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
		if len(out) == maxDetailPhotos {
			break
		}
	}
	return out
}
