package glamping_rf

import "testing"

// Вместимость раньше искалась регуляркой по слову «Вместимость», которого в
// вёрстке источника больше нет: capacityRe не совпадал ни на одной странице, и
// у 620 объектов из 622 стояло дефолтное «до 4 гостей». Настоящие данные лежат
// в том же JSON, который парсер уже разбирает ради платных услуг.
func TestParseRoomSpecs(t *testing.T) {
	cases := []struct {
		name   string
		specs  []string
		guests int
		area   int
	}{
		{"площадь и двуспальная", []string{"📐 32 м²", "🛌 1 двуспальная"}, 2, 32},
		{"две двуспальные", []string{"🛌 2 двуспальная"}, 4, 0},
		{"односпальные считаются по одному", []string{"🛏 2 односпальная"}, 2, 0},
		{"диван-кровать — одно место, не два", []string{"🛋 1 диван-кровать"}, 1, 0},
		{"места складываются", []string{"🛌 1 двуспальная", "🛋 1 диван-кровать"}, 3, 0},
		{"без спальных мест — ноль, а не дефолт", []string{"📐 40 м²"}, 0, 40},
		{"пусто", nil, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseRoomSpecs(c.specs)
			if got.Guests != c.guests {
				t.Errorf("гостей = %d, ожидали %d", got.Guests, c.guests)
			}
			if got.AreaM2 != c.area {
				t.Errorf("площадь = %d, ожидали %d", got.AreaM2, c.area)
			}
		})
	}
}

// Значок в начале строки может смениться — опираемся на число и единицу.
func TestParseRoomSpecs_IgnoresEmoji(t *testing.T) {
	got := parseRoomSpecs([]string{"32 м²", "1 двуспальная"})
	if got.AreaM2 != 32 || got.Guests != 2 {
		t.Errorf("без значков разбор сломался: %+v", got)
	}
}

// У объекта несколько домиков. Берём САМЫЙ ВМЕСТИТЕЛЬНЫЙ, а не сумму: суммой
// мы обещали бы компанию из двенадцати там, где три домика по четверо и каждый
// бронируется отдельно.
func TestDetailRoomSpecs_TakesLargestRoom(t *testing.T) {
	page := `<script>window.pv12RoomDetails = {
		"1": {"specs": ["📐 20 м²", "🛌 1 двуспальная"]},
		"2": {"specs": ["📐 55 м²", "🛌 2 двуспальная", "🛋 1 диван-кровать"]},
		"3": {"specs": ["📐 30 м²", "🛏 2 односпальная"]}
	};</script>`

	got := detailRoomSpecs(parseRoomsWithSpecs(page))
	if got.Guests != 5 {
		t.Errorf("гостей = %d, ожидали 5 (самый вместительный домик)", got.Guests)
	}
	if got.AreaM2 != 55 {
		t.Errorf("площадь = %d, ожидали 55 (наибольшая)", got.AreaM2)
	}
}

// Нет данных — ноль, а не догадка. Именно подстановка дефолта и породила
// выдуманное «до 4 гостей» по всему каталогу.
func TestDetailRoomSpecs_NoDataIsZero(t *testing.T) {
	for _, page := range []string{"", "<html>без джейсона</html>", `window.pv12RoomDetails = {};`} {
		if got := detailRoomSpecs(parseRoomsWithSpecs(page)); got.Guests != 0 || got.AreaM2 != 0 {
			t.Errorf("на %q выдумали %+v", page[:min(len(page), 20)], got)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Форма жилья по названию домика. Тег types источника для этого не годится: у
// двух третей каталога там «Эко-дом», потому что у объекта бывает девять
// домиков разных форм и одним тегом их не описать. В названиях форма есть —
// владелец называет домик так, как его продаёт.
func TestHouseTypesFromRooms(t *testing.T) {
	page := `window.pv12RoomDetails = {
		"1": {"name": "Geocupol Deluxe с купелью"},
		"2": {"name": "Barn Family"},
		"3": {"name": "Smart Lodge"}
	};`

	got := houseTypesFromRooms(parseRoomsWithSpecs(page))
	if len(got) != 2 {
		t.Fatalf("houseTypes = %v, ожидали купол и барнхаус", got)
	}
	// Порядок стабилен — по правилам, а не по обходу map (тот произвольный).
	if got[0] != "Купольный дом" || got[1] != "Барнхаус" {
		t.Errorf("порядок нестабилен: %v", got)
	}
}

func TestHouseTypesFromRooms_Vocabulary(t *testing.T) {
	cases := map[string]string{
		"А-фрейм №1 с чаном и баней":   "A-frame",
		"А фрейм №2":                   "A-frame",
		"Geocupol с купелью":           "Купольный дом",
		"Купольный дом с видом на лес": "Купольный дом",
		"Сфера":                 "Купольный дом",
		"БарнХаус Бохо":         "Барнхаус",
		`Дом «Барн»`:            "Барнхаус",
		"Фридом Барн с диваном": "Барнхаус",
		"Модом с 2 спальнями":   "Модульный дом",
		"Домик на дереве":       "Дом на дереве",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			page := `window.pv12RoomDetails = {"1": {"name": "` + name + `"}};`
			got := houseTypesFromRooms(parseRoomsWithSpecs(page))
			if len(got) != 1 || got[0] != want {
				t.Errorf("«%s» → %v, ожидали %q", name, got, want)
			}
		})
	}
}

// Названия без формы жилья типа не дают: гадать по «Дом Семейный» нельзя.
func TestHouseTypesFromRooms_NoGuessing(t *testing.T) {
	for _, name := range []string{"Дом Семейный", "Дом на двоих", "Премиум на двоих", "дом КЕНЗА"} {
		page := `window.pv12RoomDetails = {"1": {"name": "` + name + `"}};`
		if got := houseTypesFromRooms(parseRoomsWithSpecs(page)); got != nil {
			t.Errorf("«%s» → выдумали %v", name, got)
		}
	}
}

// Теги списка и названия домиков дополняют друг друга: ни один не полон.
func TestMergeHouseTypes(t *testing.T) {
	got := mergeHouseTypes([]string{"A-frame"}, []string{"Купольный дом", "A-frame"})
	if len(got) != 2 || got[0] != "A-frame" || got[1] != "Купольный дом" {
		t.Errorf("mergeHouseTypes = %v", got)
	}
	if got := mergeHouseTypes(nil, nil); got != nil {
		t.Errorf("пустой ввод → %#v, ожидали nil", got)
	}
	if got := mergeHouseTypes([]string{"Барнхаус"}, nil); len(got) != 1 {
		t.Errorf("теги списка потеряны: %v", got)
	}
}

// Источник называет вместимость прямо в описании комнаты, и его слово важнее
// нашей арифметики по кроватям. Сверка на живых страницах: «3 двуспальные +
// диван» у него «до 6», а расчёт давал 8 — завышение ровно того рода, от
// которого уходили.
func TestParseRoom_DeclaredCapacityWins(t *testing.T) {
	room := pv12RoomWithSpecs{
		Specs: []string{"📐 64 м²", "🛌 3 двуспальная", "🛋 1 диван-кровать"},
		Desc:  "Просторный коттедж. Вместимость: до 6 гостей (есть возможность доп. места)",
	}
	if got := parseRoom(room).Guests; got != 6 {
		t.Errorf("гостей = %d, ожидали 6 — источник назвал число прямо", got)
	}
}

func TestParseRoom_FallsBackToBeds(t *testing.T) {
	// Источник промолчал — считаем по спальным местам.
	room := pv12RoomWithSpecs{
		Specs: []string{"🛌 1 двуспальная", "🛏 2 односпальная"},
		Desc:  "Уютный домик у леса.",
	}
	if got := parseRoom(room).Guests; got != 4 {
		t.Errorf("гостей = %d, ожидали 4 по кроватям", got)
	}
}

// Формат «Вместимость: ДО N» — тот самый, из-за которого старая регулярка
// перестала находить число: она ждала его сразу за двоеточием.
func TestDeclaredCapacity_Formats(t *testing.T) {
	cases := map[string]int{
		"Вместимость: до 2 гостей":                          2,
		"Вместимость: до 4-х гостей (2 взрослых + 2 детей)": 4,
		"Вместимость: 5 гостей":                             5,
		"вместимость до 8 человек":                          8,
	}
	for desc, want := range cases {
		got := parseRoom(pv12RoomWithSpecs{Desc: desc}).Guests
		if got != want {
			t.Errorf("%q → %d, ожидали %d", desc, got, want)
		}
	}
}

// Подстрока внутри другого слова формой жилья не делает: «Атмосфера» ловилась
// как «сфера» и превращала объект в купол.
func TestHouseTypesFromRooms_NoSubstringFalsePositives(t *testing.T) {
	for _, name := range []string{"Атмосфера", "Биосфера", "Аутентика", "Апартаменты Аутентик"} {
		page := `window.pv12RoomDetails = {"1": {"name": "` + name + `"}};`
		if got := houseTypesFromRooms(parseRoomsWithSpecs(page)); got != nil {
			t.Errorf("«%s» → выдумали %v", name, got)
		}
	}
	// А настоящая форма по-прежнему находится.
	page := `window.pv12RoomDetails = {"1": {"name": "Сфера с камином"}};`
	if got := houseTypesFromRooms(parseRoomsWithSpecs(page)); len(got) != 1 || got[0] != "Купольный дом" {
		t.Errorf("«Сфера с камином» → %v", got)
	}
}

// «32.5 м²» без учёта дробной части совпадало хвостом и давало 5 м².
func TestParseRoomSpecs_FractionalArea(t *testing.T) {
	cases := map[string]int{
		"📐 32 м²":   32,
		"📐 32.5 м²": 32,
		"📐 32,5 м²": 32,
		"📐 105 м²":  105,
	}
	for spec, want := range cases {
		if got := parseRoomSpecs([]string{spec}).AreaM2; got != want {
			t.Errorf("%q → %d м², ожидали %d", spec, got, want)
		}
	}
}
