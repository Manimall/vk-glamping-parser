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
