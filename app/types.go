package main

import (
	"strings"

	"vk-parser/internal/extract"
)

// Coords — гео-координаты объекта. Указатель в GlampingData (см. ниже), чтобы
// omitempty мог их «выкинуть»: у структуры-значения нет понятия «пустая», а
// nil-указатель omitempty уберёт.
type Coords struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Cabin — ОДИН домик глэмпинга (A-frame, BALI и т.п.). У каждого своя цена и
// своё описание → свои удобства. Property — структурированный результат: что
// именно есть в этом домике (главное, что нам нужно).
type Cabin struct {
	Title       string            `json:"title"`
	Price       string            `json:"price,omitempty"`
	Description string            `json:"description,omitempty"`
	Property    *extract.Property `json:"property,omitempty"`
	// Variants — заголовки почти-одинаковых домиков, схлопнутых в этот (напр.
	// «тёмный» вариант того же А-фрейма). omitempty: если дублей не было — поля нет.
	Variants []string `json:"variants,omitempty"`
}

// GlampingData — карточка глэмпинга: ОБЪЕКТ-уровень (название, локация, галерея)
// + список домиков. omitempty убирает поля, которых нет.
type GlampingData struct {
	Title    string          `json:"title,omitempty"`    // название глэмпинга (из группы)
	About    string          `json:"about,omitempty"`    // описание сообщества
	Location string          `json:"location,omitempty"` // адрес/город
	Coords   *Coords         `json:"coords,omitempty"`   // координаты (если заданы)
	MapURL   string          `json:"mapUrl,omitempty"`   // ссылка на карту (если задана)
	Contact  string          `json:"contact,omitempty"`  // телефон
	Photos   []string        `json:"photos"`             // общая галерея
	Cabins   []Cabin         `json:"cabins"`             // домики с удобствами
	Extras   []extract.Extra `json:"extras,omitempty"`   // доп.услуги объекта (товары-услуги VK)
}

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
