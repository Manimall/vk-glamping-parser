package glamping_rf

import "testing"

// Теги окружения источника заполнены у всех объектов, а разбор описания давал
// «в лесу» только у 28 из 316 — там «Лесное озеро» чаще название, чем водоём.
func TestSurroundings(t *testing.T) {
	got := surroundings([]string{"forest", "lake", "bnovo"})
	if len(got) != 2 || got[0] != "Лес" || got[1] != "Озеро" {
		t.Fatalf("surroundings = %v", got)
	}

	// bnovo — метка системы бронирования, к окружению отношения не имеет.
	if got := surroundings([]string{"bnovo"}); got != nil {
		t.Errorf("служебный тег попал в окружение: %v", got)
	}

	// Источник вправе завести новый тег — падать из-за этого незачем.
	if got := surroundings([]string{"forest", "неизвестный_тег"}); len(got) != 1 {
		t.Errorf("неизвестный тег не пропущен: %v", got)
	}

	if got := surroundings(nil); got != nil {
		t.Errorf("ожидали nil при пустых тегах, got %#v", got)
	}
}

// Молчание источника о питомцах нельзя показывать как запрет: гость с собакой
// не поедет туда, где на самом деле можно.
func TestPetsAllowed(t *testing.T) {
	if p := petsAllowed("yes"); p == nil || !*p {
		t.Errorf("«yes» → %v", p)
	}
	if p := petsAllowed("no"); p == nil || *p {
		t.Errorf("«no» → %v", p)
	}
	for _, silent := range []string{"", "  ", "maybe", "null"} {
		if p := petsAllowed(silent); p != nil {
			t.Errorf("молчание источника (%q) стало ответом: %v", silent, *p)
		}
	}
	// Регистр источника не гарантирован.
	if p := petsAllowed("YES"); p == nil || !*p {
		t.Errorf("регистр не учтён: %v", p)
	}
}

// Расстояние источник шлёт числом В СТРОКЕ. Ноль означает «неизвестно»:
// у загородного дома расстояние в ноль километров — это ошибка данных,
// а не близость.
func TestDistanceKm(t *testing.T) {
	if got := distanceKm("70"); got != 70 {
		t.Errorf("distanceKm(70) = %d", got)
	}
	if got := distanceKm(" 154 "); got != 154 {
		t.Errorf("пробелы не обрезаны: %d", got)
	}
	for _, bad := range []string{"", "null", "далеко", "0", "-5"} {
		if got := distanceKm(bad); got != 0 {
			t.Errorf("distanceKm(%q) = %d, ожидали 0", bad, got)
		}
	}
}

// Сквозная проверка: признаки доезжают до контракта из полей источника.
func TestToObject_Facets(t *testing.T) {
	it := sampleItem()
	it.Badges = []string{"forest", "river", "bnovo"}
	it.Animal = "yes"
	it.Rating = 4.845
	it.ReviewsCount = 129
	it.City = apiCity{City: "Москва", Highway: "Дмитровское", Distance: "70", Unit: "км от МКАД"}
	it.Price = apiPrice{Value: 7360, Formatted: "7 360 ₽"}

	o := toObject(it)

	if len(o.Surroundings) != 2 {
		t.Errorf("Surroundings = %v", o.Surroundings)
	}
	if o.PetsAllowed == nil || !*o.PetsAllowed {
		t.Errorf("PetsAllowed = %v", o.PetsAllowed)
	}
	if o.Rating != 4.845 || o.ReviewsCount != 129 {
		t.Errorf("рейтинг: %v / %d", o.Rating, o.ReviewsCount)
	}
	if o.DistanceKm != 70 || o.Highway != "Дмитровское" {
		t.Errorf("расстояние: %d км по %q", o.DistanceKm, o.Highway)
	}
	if o.PriceValue != 7360 {
		t.Errorf("PriceValue = %d", o.PriceValue)
	}
	// Строка показа остаётся: по ней рисуют карточку и ищет бот.
	if o.Cabins[0].Price != "7 360 ₽" {
		t.Errorf("строка цены потеряна: %q", o.Cabins[0].Price)
	}
}

// Пустые поля не должны появляться в JSON: потребитель отличает «неизвестно»
// от «ноль». Особенно важно для PetsAllowed — там nil и false разный ответ.
func TestToObject_FacetsOmitEmpty(t *testing.T) {
	it := sampleItem()
	it.Badges = nil
	it.Animal = ""
	it.City = apiCity{}

	o := toObject(it)

	if o.Surroundings != nil {
		t.Errorf("Surroundings = %#v, ожидали nil", o.Surroundings)
	}
	if o.PetsAllowed != nil {
		t.Errorf("PetsAllowed = %v, ожидали nil при молчании источника", *o.PetsAllowed)
	}
	if o.DistanceKm != 0 || o.Highway != "" {
		t.Errorf("расстояние выдумано: %d / %q", o.DistanceKm, o.Highway)
	}
}

// Источник и наш словарь называют одно и то же по-разному: «БарнХаус» против
// «Барнхаус». Без канонизации в каталоге появлялись два раздела на одну форму
// (30 объектов в одном, 19 в другом), и гость видел только половину.
func TestCanonHouseType(t *testing.T) {
	cases := map[string]string{
		"БарнХаус":      "Барнхаус",
		"Барнхаус":      "Барнхаус",
		"barn house":    "Барнхаус",
		"A-frame":       "A-frame",
		"а фрейм":       "A-frame",
		"Купольный дом": "Купольный дом",
		"Сфера":         "Купольный дом",
		"Шатер":         "Сафари-тент",
		"Шатёр":         "Сафари-тент",
		"Типи":          "Сафари-тент",
		"Модом":         "Модульный дом",
	}
	for in, want := range cases {
		if got := canonHouseType(in); got != want {
			t.Errorf("canonHouseType(%q) = %q, ожидали %q", in, got, want)
		}
	}
	// Незнакомую форму не теряем: источник вправе завести новую.
	if got := canonHouseType("Иглу"); got != "Иглу" {
		t.Errorf("незнакомая форма потеряна: %q", got)
	}
}

func TestMergeHouseTypes_DeduplicatesSpelling(t *testing.T) {
	got := mergeHouseTypes([]string{"БарнХаус"}, []string{"Барнхаус", "barn"})
	if len(got) != 1 || got[0] != "Барнхаус" {
		t.Errorf("написания не схлопнулись: %v", got)
	}
}
