// Package contract — единый формат карточки объекта, который отдают ВСЕ провайдеры
// (VK, glamping_rf, …) и который потребляет фронтенд. Один контракт на все
// источники: добавление нового провайдера не требует правок на фронте.
//
// Раньше эти типы жили в пакете main (app) под именем GlampingData. Вынесены сюда,
// чтобы их могли использовать изолированные провайдеры (providers/*), не завязываясь
// на main. app держит на них псевдонимы (type GlampingData = contract.Object).
package contract

import "vk-parser/internal/extract"

// Coords — гео-координаты объекта. Указатель у владельца, чтобы omitempty мог их
// «выкинуть»: у структуры-значения нет понятия «пустая», а nil-указатель уберётся.
type Coords struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Cabin — ОДИН домик объекта (A-frame, купольный шатёр и т.п.): своя цена и своё
// структурированное описание (Property — что именно в нём есть).
type Cabin struct {
	Title       string            `json:"title"`
	Price       string            `json:"price,omitempty"`
	Description string            `json:"description,omitempty"`
	Property    *extract.Property `json:"property,omitempty"`
	// Variants — заголовки почти-одинаковых домиков, схлопнутых в этот (дедуп).
	Variants []string `json:"variants,omitempty"`
}

// Object — карточка объекта размещения: ОБЪЕКТ-уровень (название, локация,
// галерея) + список домиков. Единый JSON-контракт для фронта. omitempty убирает
// поля, которых нет.
type Object struct {
	Title    string          `json:"title,omitempty"`    // название объекта
	About    string          `json:"about,omitempty"`    // описание
	Location string          `json:"location,omitempty"` // адрес/город/регион
	Coords   *Coords         `json:"coords,omitempty"`   // координаты (если есть)
	MapURL   string          `json:"mapUrl,omitempty"`   // ссылка на карту
	Contact  string          `json:"contact,omitempty"`  // телефон/контакт
	Photos   []string        `json:"photos"`             // галерея (URL кадров)
	Cabins   []Cabin         `json:"cabins"`             // домики с удобствами
	Extras   []extract.Extra `json:"extras,omitempty"`   // доп.услуги объекта
	Seo      *extract.SEO    `json:"seo,omitempty"`      // SEO/OG-тексты (без бренда фронта)
}
