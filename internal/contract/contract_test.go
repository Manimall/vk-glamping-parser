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

	for _, key := range []string{"region", "locality", "nearCity"} {
		if strings.Contains(string(raw), key) {
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
