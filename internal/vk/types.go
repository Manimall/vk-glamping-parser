package vk

// Структуры данных VK-ответов. Объявляем только нужные поля — неизвестные
// ключи JSON encoding/json молча игнорирует.

// --- utils.resolveScreenName ---

type resolvedScreenName struct {
	Type     string `json:"type"`
	ObjectID int64  `json:"object_id"`
}

// --- photos.get ---

type photoSize struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type photo struct {
	Sizes []photoSize `json:"sizes"`
}

type photosGetResponse struct {
	Count int     `json:"count"`
	Items []photo `json:"items"`
}

// --- market.get / market.getById ---

// Price — экспортируемая часть MarketItem (с Большой буквы = публичная).
type Price struct {
	Amount string `json:"amount"` // сумма строкой (служебная)
	Text   string `json:"text"`   // готовая цена, напр. "7 000 ₽"
}

// MarketItem торчит наружу (фигурирует в сигнатуре методов), поэтому он и его
// поля экспортируемые.
type MarketItem struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"` // полный текст описания товара
	Price       Price  `json:"price"`
}

type marketGetResponse struct {
	Count int          `json:"count"`
	Items []MarketItem `json:"items"`
}

// --- groups.getById ---

// GroupInfo — «плоский» удобный результат для вызывающего: название, описание,
// адрес/координаты, телефон. Собираем его из «сырых» полей ответа VK ниже.
type GroupInfo struct {
	Name        string
	Description string
	Address     string
	Latitude    float64
	Longitude   float64
	Phone       string
}

type groupCity struct {
	Title string `json:"title"`
}

type groupPlace struct {
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type groupContact struct {
	Phone string `json:"phone"`
}

type groupByID struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	City        groupCity      `json:"city"`
	Place       groupPlace     `json:"place"`
	Contacts    []groupContact `json:"contacts"`
}
