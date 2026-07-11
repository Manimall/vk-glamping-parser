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
	Slug     string          `json:"slug,omitempty"`     // URL-идентификатор (/api/v1/glampings/<slug>)
	Title    string          `json:"title,omitempty"`    // название объекта
	About    string          `json:"about,omitempty"`    // описание
	Location string          `json:"location,omitempty"` // адрес/город/регион
	Coords   *Coords         `json:"coords,omitempty"`   // координаты (если есть)
	MapURL   string          `json:"mapUrl,omitempty"`   // ссылка на карту
	Contact  string          `json:"contact,omitempty"`  // телефон/контакт
	Cover    string          `json:"cover,omitempty"`    // обложка-превью (карточка главной, OG)
	Photos   []string        `json:"photos"`             // галерея (URL кадров)
	Cabins   []Cabin         `json:"cabins"`             // домики с удобствами
	Extras   []extract.Extra `json:"extras,omitempty"`   // доп.услуги объекта
	Seo      *extract.SEO    `json:"seo,omitempty"`      // SEO/OG-тексты (без бренда фронта)
}

// Preview — облегчённая карточка для списков (главная страница каталога):
// только то, что нужно отрисовать плитку — без галереи, удобств и правил.
type Preview struct {
	Slug     string       `json:"slug"`
	Title    string       `json:"title"`
	Location string       `json:"location,omitempty"`
	Cover    string       `json:"cover,omitempty"`
	Price    string       `json:"price,omitempty"` // цена первого домика («7 000 ₽»)
	Seo      *extract.SEO `json:"seo,omitempty"`   // OG-тексты для шаринга ссылки
}

// ToPreview собирает превью из полной карточки (цена — у первого домика).
//
// [Go для изучения] Ресивер (o Object) БЕЗ звёздочки — метод получает КОПИЮ
// структуры: читать можно, а мутировать оригинал — нет, что здесь и нужно
// (чистое преобразование). Указательный ресивер (o *Object) брали бы для
// мутаций или чтобы не копировать крупную структуру.
func (o Object) ToPreview() Preview {
	p := Preview{Slug: o.Slug, Title: o.Title, Location: o.Location, Cover: o.Cover, Seo: o.Seo}
	if len(o.Cabins) > 0 {
		p.Price = o.Cabins[0].Price
	}
	return p
}
