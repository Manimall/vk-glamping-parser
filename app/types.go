package main

import (
	"strings"

	"vk-parser/internal/contract"
)

// Типы карточки объекта вынесены в общий пакет contract (их используют все
// провайдеры). Здесь — псевдонимы, чтобы существующий код app (handler/export)
// остался без изменений, а формат JSON жил в одном месте.
type (
	Coords       = contract.Coords
	Cabin        = contract.Cabin
	GlampingData = contract.Object
)

// glampingQuery — разобранные параметры запроса. Объект-параметр вместо длинного
// списка аргументов buildGlampingData(domain, items, coords, map, ...): добавить
// новый параметр = новое поле здесь, сигнатуры функций не пухнут (тот же приём,
// что и со структурой server).
type glampingQuery struct {
	domain string
	items  string // товары-домики (URL/id через запятую)
	coords string // "lat,lon" — если VK не отдал координаты
	mapURL string // ссылка на карту (Яндекс/Google)
}

// cacheKey — детерминированный ключ кэша из всех параметров: разный ввод = разный
// ответ. Поля простые строки, порядок фиксирован, так что ключ стабилен.
func (q glampingQuery) cacheKey() string {
	return strings.Join([]string{q.domain, q.items, q.coords, q.mapURL}, "|")
}
