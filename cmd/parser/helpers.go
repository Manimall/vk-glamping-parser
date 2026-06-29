package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// reDomain — допустимый VK screen name: буквы, цифры, точка, подчёркивание.
// Отсекает path-traversal ("../x" содержит "/") в objects.Load и мусор в VK.
var reDomain = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

// isValidDomain — domain пришёл из URL и идёт в файловый путь + VK, поэтому
// валидируем его перед использованием.
func isValidDomain(domain string) bool {
	return reDomain.MatchString(domain)
}

// parseCoords разбирает строку "lat,lon" в Coords. Возвращает (nil,false), если
// формат неверный — тогда вызывающий просто не трогает координаты.
func parseCoords(raw string) (*Coords, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return nil, false
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return nil, false
	}
	return &Coords{Lat: lat, Lon: lon}, true
}

// reItemTail — хвост числа из URL/строки товара. У ссылки вида
// .../aframe-svetly-arenda-211011668-6377368 последнее число — это id товара.
var reItemTail = regexp.MustCompile(`(\d+)\D*$`)

// marketIDsFromParam превращает параметр items (URL или id через запятую) в
// абсолютные market-id вида "<ownerID>_<item>". Принимаем три формата:
//   - полный id "-211011668_6377368" → как есть;
//   - ссылку ".../...-211011668-6377368" → берём хвостовое число + наш ownerID;
//   - голый id товара "6377368" → тоже + ownerID.
//
// ownerID уже со знаком (минус для групп), поэтому склейка даёт верный market-id.
func marketIDsFromParam(raw string, ownerID int64) []string {
	ids := make([]string, 0)
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if strings.Contains(tok, "_") {
			ids = append(ids, tok) // уже полный market-id
			continue
		}
		// Отрезаем query/fragment (?p=2, #...): иначе хвостовое число из них
		// перебьёт id товара (URL ".../-6377368?p=2" → взяли бы "2").
		if i := strings.IndexAny(tok, "?#"); i >= 0 {
			tok = tok[:i]
		}
		if m := reItemTail.FindStringSubmatch(tok); m != nil {
			ids = append(ids, fmt.Sprintf("%d_%s", ownerID, m[1]))
		}
	}
	return ids
}

// dupThreshold — порог схожести удобств, при котором домики считаем дублями.
const dupThreshold = 0.8

// dedupCabins схлопывает почти одинаковые домики ОДНОГО типа. Домики считаем
// дублями, только если выполнено И то И другое:
//   - удобства совпадают на ≥dupThreshold (Jaccard по их набору);
//   - совпадает «семья» названия (первое слово, напр. «AFRAME светлый» и
//     «AFRAME тёмный»).
//
// Второе условие защищает от схлопывания РАЗНЫХ домиков (AFRAME и BALI), которые
// случайно делят много общих удобств: иначе второй потерял бы цену и описание,
// оставшись лишь строкой в Variants.
func dedupCabins(cabins []Cabin) []Cabin {
	kept := make([]Cabin, 0, len(cabins))
	sigs := make([]map[string]bool, 0, len(cabins))

	for _, c := range cabins {
		sig := amenitySignature(c)
		merged := false
		for i := range kept {
			if sameTitleFamily(kept[i].Title, c.Title) && jaccard(sig, sigs[i]) >= dupThreshold {
				// Дубль: добавляем его название как вариант к уже сохранённому.
				kept[i].Variants = append(kept[i].Variants, c.Title)
				merged = true
				break
			}
		}
		if !merged {
			kept = append(kept, c)
			sigs = append(sigs, sig)
		}
	}
	return kept
}

// sameTitleFamily — у домиков совпадает первое слово названия (без учёта
// регистра). «AFRAME светлый» / «AFRAME тёмный» → одна семья; «AFRAME» / «BALI» → нет.
func sameTitleFamily(a, b string) bool {
	return firstWord(a) == firstWord(b)
}

func firstWord(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

// amenitySignature — множество «меток» домика: удобства + доп.услуги. Это и есть
// его «отпечаток» для сравнения. Пустой Property → пустая сигнатура (не схлопнем).
func amenitySignature(c Cabin) map[string]bool {
	sig := make(map[string]bool)
	if c.Property == nil {
		return sig
	}
	for _, g := range c.Property.AmenityGroups {
		for _, item := range g.Items {
			sig[item] = true
		}
	}
	for _, e := range c.Property.Extras {
		sig[e.Name] = true
	}
	return sig
}

// jaccard — мера схожести двух множеств: |пересечение| / |объединение|.
// 1.0 = одинаковы, 0.0 = ничего общего. Классический способ сравнить наборы.
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0 // обе пустые — НЕ считаем дублями (нечего сравнивать)
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	// union > 0 гарантировано: если бы оба набора были пусты, вышли бы выше.
	union := len(a) + len(b) - inter
	return float64(inter) / float64(union)
}

// writeJSON — без зависимостей, поэтому остаётся обычной функцией (не методом).
// DRY: одна запись JSON-ответа для веток HIT и MISS.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Error("encode response failed", "err", err)
	}
}
