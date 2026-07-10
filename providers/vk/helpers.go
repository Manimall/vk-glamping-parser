// Package vkprovider — источник данных из VK (сообщества + товары-домики).
// Изолированная логика сбора одного объекта (Build) и пакетного обхода настроенных
// объектов (Parse). Реализует providers.Provider и отдаёт contract.Object — тот же
// формат, что и остальные источники.
//
// Пакет назван vkprovider (а не vk), чтобы не конфликтовать с низкоуровневым
// клиентом internal/vk, который он же и использует.
package vkprovider

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"vk-parser/internal/contract"
)

// reItemTail — хвост числа из URL/строки товара. У ссылки вида
// .../aframe-svetly-arenda-211011668-6377368 последнее число — это id товара.
var reItemTail = regexp.MustCompile(`(\d+)\D*$`)

// MarketIDsFromParam превращает параметр items (URL или id через запятую) в
// абсолютные market-id вида "<ownerID>_<item>". Принимаем три формата: полный id
// "-211011668_6377368"; ссылку ".../-211011668-6377368"; голый id товара.
// ownerID уже со знаком (минус для групп) — склейка даёт верный market-id.
func MarketIDsFromParam(raw string, ownerID int64) []string {
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

// parseCoords разбирает строку "lat,lon" в Coords. (nil,false) при неверном
// формате — тогда вызывающий просто не трогает координаты.
func parseCoords(raw string) (*contract.Coords, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return nil, false
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return nil, false
	}
	return &contract.Coords{Lat: lat, Lon: lon}, true
}

// dupThreshold — порог схожести удобств, при котором домики считаем дублями.
const dupThreshold = 0.8

// dedupCabins схлопывает почти одинаковые домики ОДНОГО типа: удобства совпадают
// на ≥dupThreshold (Jaccard) И совпадает «семья» названия (первое слово). Второе
// условие защищает от схлопывания РАЗНЫХ домиков (AFRAME и BALI) с общими удобствами.
func dedupCabins(cabins []contract.Cabin) []contract.Cabin {
	kept := make([]contract.Cabin, 0, len(cabins))
	sigs := make([]map[string]bool, 0, len(cabins))

	for _, c := range cabins {
		sig := amenitySignature(c)
		merged := false
		for i := range kept {
			if sameTitleFamily(kept[i].Title, c.Title) && jaccard(sig, sigs[i]) >= dupThreshold {
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

func sameTitleFamily(a, b string) bool { return firstWord(a) == firstWord(b) }

func firstWord(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

// amenitySignature — множество «меток» домика (удобства + доп.услуги): его
// «отпечаток» для сравнения. Пустой Property → пустая сигнатура (не схлопнём).
func amenitySignature(c contract.Cabin) map[string]bool {
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

// jaccard — мера схожести множеств: |пересечение| / |объединение|.
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	return float64(inter) / float64(union)
}

// configuredDomains — домены (screen name) из конфигов dataDir/*.json, кроме
// example.json. Отсортированы — детерминированный порядок пакетного сбора.
func configuredDomains(dataDir string) ([]string, error) {
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, fmt.Errorf("vk: read dataDir %s: %w", dataDir, err)
	}
	var domains []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || filepath.Ext(name) != ".json" || name == "example.json" {
			continue
		}
		domains = append(domains, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(domains)
	return domains, nil
}
