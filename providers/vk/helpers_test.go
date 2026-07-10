package vkprovider

import (
	"testing"

	"vk-parser/internal/contract"
	"vk-parser/internal/extract"
)

func TestParseCoords(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantOK  bool
		wantLat float64
		wantLon float64
	}{
		{"валидные", "57.07,41.01", true, 57.07, 41.01},
		{"с пробелами", " 57.07 , 41.01 ", true, 57.07, 41.01},
		{"пусто", "", false, 0, 0},
		{"одно число", "57.07", false, 0, 0},
		{"не число", "abc,def", false, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, ok := parseCoords(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, ожидал %v", ok, tt.wantOK)
			}
			if ok && (c.Lat != tt.wantLat || c.Lon != tt.wantLon) {
				t.Errorf("получил (%v,%v), ожидал (%v,%v)", c.Lat, c.Lon, tt.wantLat, tt.wantLon)
			}
		})
	}
}

func TestMarketIDsFromParam(t *testing.T) {
	const owner = int64(-211011668)
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{"полная ссылка", "https://vk.com/market/product/aframe-svetly-arenda-211011668-6377368", []string{"-211011668_6377368"}},
		{"голый id товара + ownerID", "6377368", []string{"-211011668_6377368"}},
		{"полный market-id как есть", "-211011668_6493879", []string{"-211011668_6493879"}},
		{"ссылка с query-хвостом", "https://vk.com/market/product/aframe-211011668-6377368?p=2", []string{"-211011668_6377368"}},
		{"несколько через запятую", "6377368, 6493879", []string{"-211011668_6377368", "-211011668_6493879"}},
		{"пустая строка", "", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarketIDsFromParam(tt.raw, owner)
			if len(got) != len(tt.want) {
				t.Fatalf("получил %v, ожидал %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] получил %q, ожидал %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestJaccard(t *testing.T) {
	a := map[string]bool{"x": true, "y": true, "z": true}
	b := map[string]bool{"x": true, "y": true, "z": true}
	if got := jaccard(a, b); got != 1.0 {
		t.Errorf("одинаковые наборы: получил %v, ожидал 1.0", got)
	}
	c := map[string]bool{"a": true}
	if got := jaccard(a, c); got != 0.0 {
		t.Errorf("непересекающиеся: получил %v, ожидал 0.0", got)
	}
	if got := jaccard(map[string]bool{}, map[string]bool{}); got != 0 {
		t.Errorf("две пустые: получил %v, ожидал 0", got)
	}
}

// cabinWith — домик с заданными удобствами (для теста дедупа).
func cabinWith(title string, amenities ...string) contract.Cabin {
	return contract.Cabin{
		Title: title,
		Property: &extract.Property{
			AmenityGroups: []extract.AmenityGroup{{Title: "В домике", Items: amenities}},
		},
	}
}

func TestDedupCabins(t *testing.T) {
	cabins := []contract.Cabin{
		cabinWith("AFRAME светлый", "Кухня", "Ванная", "ТВ", "Интернет", "Мангал"),
		cabinWith("AFRAME тёмный", "Кухня", "Ванная", "ТВ", "Интернет", "Мангал"), // дубль
		cabinWith("BALI", "Уличный душ", "Проектор", "Костровище"),                // другой
	}

	got := dedupCabins(cabins)
	if len(got) != 2 {
		t.Fatalf("ожидал 2 домика после дедупа, получил %d", len(got))
	}
	if len(got[0].Variants) != 1 || got[0].Variants[0] != "AFRAME тёмный" {
		t.Errorf("ожидал variants=[AFRAME тёмный], получил %v", got[0].Variants)
	}
	if got[1].Title != "BALI" {
		t.Errorf("второй домик должен быть BALI, получил %q", got[1].Title)
	}
}

func TestDedupKeepsDifferentFamilies(t *testing.T) {
	cabins := []contract.Cabin{
		cabinWith("AFRAME светлый", "Кухня", "Ванная", "ТВ", "Интернет", "Мангал"),
		cabinWith("BALI домик", "Кухня", "Ванная", "ТВ", "Интернет", "Мангал"),
	}
	got := dedupCabins(cabins)
	if len(got) != 2 {
		t.Fatalf("разные типы (AFRAME/BALI) не должны схлопываться, получил %d", len(got))
	}
}
