package glamping_rf

import "testing"

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
