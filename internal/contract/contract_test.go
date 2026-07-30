package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

// Пустые адресные поля обязаны ОТСУТСТВОВАТЬ в JSON, а не приезжать пустыми
// строками. Потребитель восстанавливает регион как `o.region ?? вычислить(...)`,
// а пустая строка в JS не nullish — фоллбэк молча выключится, и регион пропадёт
// у всех объектов, которым источник его не сообщил. Ошибок при этом не будет
// ни одной: поле на месте, значение пустое, сборка зелёная.
func TestObject_JSON_OmitsEmptyAddressFields(t *testing.T) {
	raw, err := json.Marshal(Object{Slug: "elkidom", Title: "ЁLKI.DOM", Location: "Иваново"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Проверяем ключи, а не подстроки: слово «region» может однажды оказаться
	// внутри title или about, и поиск по тексту покраснел бы на валидных данных.
	for _, key := range []string{"region", "locality", "nearCity"} {
		if _, ok := got[key]; ok {
			t.Errorf("пустое поле %q попало в JSON: %s", key, raw)
		}
	}
}

func TestObject_JSON_KeepsFilledAddressFields(t *testing.T) {
	raw, err := json.Marshal(Object{
		Slug:     "moy-malenkiy-les-1452",
		Location: "Московская область, Москва",
		Region:   "Московская область",
		NearCity: "Москва",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["region"] != "Московская область" {
		t.Errorf("region=%v", got["region"])
	}
	if got["nearCity"] != "Москва" {
		t.Errorf("nearCity=%v", got["nearCity"])
	}
	// Locality не заполнен — и не должен появиться даже рядом с заполненными
	// соседями: «неизвестно» не превращается в «пусто».
	if _, ok := got["locality"]; ok {
		t.Errorf("locality появился незаполненным: %s", raw)
	}
}

// Классика «добавили поле в структуру, забыли в конструктор»: компилятор
// про это молчит, а список каталога тихо остаётся без региона.
func TestToPreview_CarriesRegion(t *testing.T) {
	p := Object{Slug: "s", Title: "t", Region: "Тверская область"}.ToPreview()
	if p.Region != "Тверская область" {
		t.Fatalf("region=%q не доехал до превью", p.Region)
	}

	if empty := (Object{}).ToPreview(); empty.Region != "" {
		t.Fatalf("region=%q взялся из ниоткуда", empty.Region)
	}
}

// Поля фильтров опускаются, когда источник промолчал. Ноль и пустой список в
// выдаче потребитель прочитает как ответ: «0 ₽», «0 гостей», «питомцы нельзя» —
// и покажет гостю утверждение, которого никто не делал.
func TestObject_JSON_OmitsEmptyFilterFields(t *testing.T) {
	raw, err := json.Marshal(Object{Slug: "x"})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"houseTypes", "surroundings", "petsAllowed",
		"rating", "reviewsCount", "distanceKm", "highway",
		"priceValue", "guestsMax",
	} {
		if strings.Contains(string(raw), `"`+field+`"`) {
			t.Errorf("пустое поле %q попало в JSON: %s", field, raw)
		}
	}
}

// petsAllowed — указатель именно ради этой разницы: «нельзя» обязано доехать
// до фронта, а молчание источника — нет. Для гостя с собакой это разные ответы.
func TestObject_JSON_PetsAllowedFalseIsNotSilence(t *testing.T) {
	no := false
	raw, err := json.Marshal(Object{PetsAllowed: &no})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"petsAllowed":false`) {
		t.Errorf("явное «нельзя» потерялось: %s", raw)
	}
}

func TestObject_JSON_KeepsFilledFilterFields(t *testing.T) {
	yes := true
	raw, err := json.Marshal(Object{
		HouseTypes:   []string{"A-frame"},
		Surroundings: []string{"В лесу"},
		PetsAllowed:  &yes,
		Rating:       4.8,
		ReviewsCount: 129,
		DistanceKm:   70,
		Highway:      "Дмитровское",
		PriceValue:   7360,
		GuestsMax:    6,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"houseTypes":["A-frame"]`, `"surroundings":["В лесу"]`,
		`"petsAllowed":true`, `"rating":4.8`, `"reviewsCount":129`,
		`"distanceKm":70`, `"highway":"Дмитровское"`,
		`"priceValue":7360`, `"guestsMax":6`,
	} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("нет %s в %s", want, raw)
		}
	}
}
