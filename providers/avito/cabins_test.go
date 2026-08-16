package avito

import (
	"testing"

	"vk-parser/internal/contract"
)

// Цена на плитке каталога берётся ровно из Cabins[0].Price — и в парсере
// (contract.ToPreview), и на сайте (catalog.toPreview). Пустой список означал
// бы карточку без цены при заполненном priceValue, поэтому объявление кладётся
// в список одной позицией.
func TestCabinsCarryPriceToPreview(t *testing.T) {
	obj := toObject(&buyerItem{Item: avitoItem{
		ID: 1, Title: "Домик у леса", Price: 6000, Address: "Ивановская обл., Иваново",
	}})

	if len(obj.Cabins) != 1 {
		t.Fatalf("домиков: %d, ожидался один", len(obj.Cabins))
	}
	c := obj.Cabins[0]
	if c.Title != "Домик у леса" {
		t.Errorf("Title = %q", c.Title)
	}
	if c.Price != "6 000 ₽" {
		t.Errorf("Price = %q, ожидалось «6 000 ₽»", c.Price)
	}
	if c.Property == nil || c.Property.PriceFrom != c.Price {
		t.Errorf("Property = %+v, ожидалась цена «от» той же строкой", c.Property)
	}
	if p := obj.ToPreview(); p.Price != "6 000 ₽" {
		t.Errorf("превью без цены: %q", p.Price)
	}
}

// Объявление без цены: выдумывать «0 ₽» нельзя, а nil в JSON уедет как null
// вместо [] — фронт ждёт массив.
func TestCabinsEmptyWithoutPrice(t *testing.T) {
	obj := toObject(&buyerItem{Item: avitoItem{ID: 2, Title: "Домик"}})
	if obj.Cabins == nil {
		t.Fatal("Cabins = nil, ожидался пустой срез")
	}
	if len(obj.Cabins) != 0 {
		t.Errorf("Cabins = %+v, ожидалось пусто", obj.Cabins)
	}
}

// Формат цены обязан совпадать с тем, что уже лежит в каталоге от glamping_rf:
// одно и то же число иначе печаталось бы на соседних карточках по-разному.
func TestCabinPriceMatchesCatalogFormat(t *testing.T) {
	got := cabins("Дом", "Иваново", 7650)[0].Price
	want := contract.Cabin{Price: "7 650 ₽"}.Price
	if got != want {
		t.Errorf("цена %q, в каталоге формат %q", got, want)
	}
}
