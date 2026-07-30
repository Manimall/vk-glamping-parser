package glamping_rf

import "testing"

// Вместимость раньше искалась регуляркой по вёрстке и почти не находилась:
// между словом «Вместимость» и числом стоят HTML-теги, поэтому почти весь
// каталог получал дефолтное «до 4 гостей». Настоящие данные лежат в том же
// JSON, который парсер уже разбирает ради платных услуг.
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

// Формы, в которых источник называет вместимость. Все они должны разбираться:
// «до N гостей», «до N-х гостей», «N гостей», «до N человек».
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

// Регулярка без привязки к людям брала первое попавшееся число: площадь из
// «Вместимость домика 32 м²» превращалась в тридцать двух гостей, а число
// спален — в число гостей. И то и другое перезаписывало верный расчёт по
// кроватям, то есть ошибка была громче, чем отсутствие данных.
func TestParseRoom_DeclaredCapacityNeedsPeople(t *testing.T) {
	beds := []string{"🛌 1 двуспальная", "🛏 2 односпальная"} // = 4 по кроватям

	cases := []struct {
		name string
		desc string
		want int
	}{
		{"площадь не вместимость", "Вместимость домика 32 м², спальных мест 4", 4},
		{"число спален не вместимость", "Вместимость дома — 3 спальни, 2 санузла", 4},
		{"настоящая вместимость побеждает кровати", "Вместимость: до 6 гостей", 6},
		{"через «человек»", "Вместимость до 5 человек", 5},
		{"через «персон»", "Вместимость — 7 персон", 7},
		{"склонённое «до 4-х гостей»", "Вместимость: до 4-х гостей", 4},
		// «до 80 гостей» на странице объекта — это банкетный зал, гости уедут.
		{"банкетный зал не вместимость домика", "Вместимость до 80 гостей", 4},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseRoom(pv12RoomWithSpecs{Specs: beds, Desc: c.desc}).Guests
			if got != c.want {
				t.Errorf("гостей = %d, ожидали %d (описание: %q)", got, c.want, c.desc)
			}
		})
	}
}
