package avito

import (
	"vk-parser/internal/contract"
	"vk-parser/internal/extract"
)

// cabins — объявление одной позицией списка домиков.
//
// У Авито разбивки по домикам нет: объявление и есть объект целиком, цена одна.
// Пустой список сюда положить нельзя — на карточке каталога цена берётся ровно
// из Cabins[0].Price (catalog.toPreview на сайте, contract.ToPreview здесь), и
// объект приехал бы в выдачу без цены, хотя PriceValue заполнен. Ровно так же —
// одной позицией с названием объекта — заполняет список и glamping_rf, у
// которого домики тоже приходят не всегда.
func cabins(title, location, about string, price int) []contract.Cabin {
	if price <= 0 {
		return []contract.Cabin{}
	}
	formatted := extract.FormatPrice(price)
	return []contract.Cabin{{
		Title: title,
		Price: formatted,
		Property: &extract.Property{
			Title:    title,
			Location: location,
			// Summary — описание объекта, а не пустая строка «на потом».
			// Страница объекта на сайте берёт описание как `summary ?? about`,
			// и пустая строка в JS проходит эту проверку насквозь: описание
			// исчезало целиком (566 символов → 0), тело страницы худело до
			// полутора сотен знаков. Поймано сквозным прогоном каталога.
			Summary:   about,
			PriceFrom: formatted,
		},
	}}
}
