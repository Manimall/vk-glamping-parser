package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

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
		if m := reItemTail.FindStringSubmatch(tok); m != nil {
			ids = append(ids, fmt.Sprintf("%d_%s", ownerID, m[1]))
		}
	}
	return ids
}

// dupThreshold — порог схожести удобств, при котором домики считаем дублями.
const dupThreshold = 0.8

// dedupCabins схлопывает почти одинаковые домики. Сигнатура домика — набор его
// удобств (из Property). Если у двух домиков удобства совпадают на ≥80%, второй
// считаем вариантом первого: его заголовок уходит в Variants, а отдельной
// карточкой он не дублируется.
func dedupCabins(cabins []Cabin) []Cabin {
	kept := make([]Cabin, 0, len(cabins))
	sigs := make([]map[string]bool, 0, len(cabins))

	for _, c := range cabins {
		sig := amenitySignature(c)
		merged := false
		for i := range kept {
			if jaccard(sig, sigs[i]) >= dupThreshold {
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
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// writeJSON — без зависимостей, поэтому остаётся обычной функцией (не методом).
// DRY: одна запись JSON-ответа для веток HIT и MISS.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}
