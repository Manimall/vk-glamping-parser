package extract

import (
	"context"
	"testing"
)

// hasAmenity — помощник: есть ли метка label среди всех групп удобств.
func hasAmenity(p *Property, label string) bool {
	for _, g := range p.AmenityGroups {
		for _, item := range g.Items {
			if item == label {
				return true
			}
		}
	}
	return false
}

// TestHeuristicExtract проверяет, что из реального описания достаются ключевые
// удобства, доп.услуги и факты. Table-driven: один прогон на каждое ожидание.
func TestHeuristicExtract(t *testing.T) {
	listing := Listing{
		Title:       "AFRAME светлый",
		Description: "Кухня с приборами: микроволновка, холодильник, чайник. Ванная с полотенцами и халатами. Телевизор, станция Алиса, интернет. Мангальная зона с шампурами. Фурако. Вместимость максимум 4 человека. Растопка фурако 4000.",
		PhotoCount:  12,
	}

	prop, err := NewHeuristic().Extract(context.Background(), listing)
	if err != nil {
		t.Fatalf("Extract вернул ошибку: %v", err)
	}

	// Ожидаемые удобства, которые словарь должен распознать.
	for _, label := range []string{
		"Кухня (оборудованная)",
		"Ванная, полотенца, халаты",
		"ТВ / проектор",
		"Умная колонка (Алиса)",
		"Мангальная зона",
		"Фурако / банный чан",
	} {
		if !hasAmenity(prop, label) {
			t.Errorf("ожидал удобство %q, его нет в %+v", label, prop.AmenityGroups)
		}
	}

	// Доп.услуга «Растопка Фурако / чана» должна попасть в extras.
	foundExtra := false
	for _, e := range prop.Extras {
		if e.Name == "Растопка Фурако / чана" {
			foundExtra = true
		}
	}
	if !foundExtra {
		t.Errorf("ожидал доп.услугу растопки фурако, extras=%+v", prop.Extras)
	}

	// Факты: вместимость и количество фото.
	if !hasFact(prop, "Вместимость", "до 4 чел.") {
		t.Errorf("ожидал вместимость 'до 4 чел.', facts=%+v", prop.Facts)
	}
	if !hasFact(prop, "Фотографий", "12") {
		t.Errorf("ожидал 12 фото в фактах, facts=%+v", prop.Facts)
	}
}

// TestCapacityFromGuests: формат «Количество гостей: N» приоритетнее «на N гостей»
// из других строк (баг, который мы чинили на scandi).
func TestCapacityFromGuests(t *testing.T) {
	listing := Listing{
		Description: "Сервировка на 6 гостей. Количество гостей: 8. Всего 8 спальных мест.",
	}
	prop, _ := NewHeuristic().Extract(context.Background(), listing)
	if !hasFact(prop, "Вместимость", "8 гостей") {
		t.Errorf("ожидал '8 гостей', а не из «на 6 гостей»; facts=%+v", prop.Facts)
	}
}

// TestMicrowaveNotHeating: «микроволновая печь» НЕ должна давать «Отопление»
// (для этого из словаря убрана жадная подстрока "печ").
func TestMicrowaveNotHeating(t *testing.T) {
	listing := Listing{Description: "Есть микроволновая печь и холодильник."}
	prop, _ := NewHeuristic().Extract(context.Background(), listing)
	if hasAmenity(prop, "Отопление") {
		t.Errorf("микроволновка ошибочно распознана как отопление: %+v", prop.AmenityGroups)
	}
}

func hasFact(p *Property, label, value string) bool {
	for _, f := range p.Facts {
		if f.Label == label && f.Value == value {
			return true
		}
	}
	return false
}
