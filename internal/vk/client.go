// Package vk — тонкий клиент к VK API (версия 5.131).
package vk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	apiBaseURL = "https://api.vk.com/method"
	apiVersion = "5.131"
	// Сколько объектов тянем за один вызов (у VK максимум обычно 200/1000).
	defaultCount = "50"
)

// Client инкапсулирует токен и переиспользуемый HTTP-клиент.
// Создаётся один раз и шарится между вызовами — см. NewClient.
type Client struct {
	token      string
	httpClient *http.Client
}

// NewClient собирает клиент с разумным таймаутом.
// Возвращаем *Client (указатель): структура несёт состояние (http-клиент),
// и копировать её при каждом вызове не нужно.
func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// --- Конверт ответа VK -------------------------------------------------------
// Любой ответ VK имеет вид {"response": ...} ЛИБО {"error": {...}}.
// Разбираем его в один общий конверт, а полезную нагрузку (response) держим
// как json.RawMessage — «отложенный» кусок JSON, который домаршалим позже
// в конкретную структуру метода.

type apiError struct {
	Code int    `json:"error_code"`
	Msg  string `json:"error_msg"`
}

type apiEnvelope struct {
	Error    *apiError       `json:"error"`
	Response json.RawMessage `json:"response"`
}

// call — единственное место, где реально ходит HTTP. Все публичные методы
// строят параметры и делегируют сюда. dst — куда домаршалить "response".
func (c *Client) call(method string, params url.Values, dst any) error {
	// Общие для всех методов параметры: токен и версия API.
	params.Set("access_token", c.token)
	params.Set("v", apiVersion)

	endpoint := fmt.Sprintf("%s/%s?%s", apiBaseURL, method, params.Encode())

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return fmt.Errorf("vk: GET %s: %w", method, err)
	}
	// defer выполнится при выходе из функции (в т.ч. при любом return ниже).
	// Закрыть тело ОБЯЗАТЕЛЬНО — иначе соединение не вернётся в пул и утечёт.
	defer resp.Body.Close()

	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("vk: decode %s: %w", method, err)
	}

	if env.Error != nil {
		return fmt.Errorf("vk: api error %d on %s: %s",
			env.Error.Code, method, env.Error.Msg)
	}

	// VK отдаёт пустой массив "[]" вместо объекта, когда ничего не найдено
	// (например, resolveScreenName для несуществующего имени). В этом случае
	// оставляем dst нулевым значением, а не падаем на домаршалинге.
	if dst != nil && len(env.Response) > 0 && string(env.Response) != "[]" {
		if err := json.Unmarshal(env.Response, dst); err != nil {
			return fmt.Errorf("vk: unmarshal %s response: %w", method, err)
		}
	}
	return nil
}

// --- GetGroupID --------------------------------------------------------------

type resolvedScreenName struct {
	Type     string `json:"type"`
	ObjectID int64  `json:"object_id"`
}

// ResolveOwnerID превращает короткое имя (домен) в owner_id для последующих
// вызовов photos.get / market.get.
//
// VK кодирует тип владельца ЗНАКОМ owner_id:
//   - группа (community)  → owner_id ОТРИЦАТЕЛЬНЫЙ  (-object_id)
//   - пользователь (user) → owner_id ПОЛОЖИТЕЛЬНЫЙ (+object_id)
func (c *Client) ResolveOwnerID(domain string) (int64, error) {
	params := url.Values{}
	params.Set("screen_name", domain)

	var resolved resolvedScreenName
	if err := c.call("utils.resolveScreenName", params, &resolved); err != nil {
		return 0, fmt.Errorf("resolve %q: %w", domain, err)
	}

	// switch в Go: без break (нет проваливания по умолчанию), ветки —
	// это и есть наша «таблица знаков» owner_id.
	switch resolved.Type {
	case "group":
		return -resolved.ObjectID, nil
	case "user":
		return resolved.ObjectID, nil
	default:
		// page, application или пусто (имя не найдено).
		return 0, fmt.Errorf("screen name %q has unsupported type %q",
			domain, resolved.Type)
	}
}

// --- GetPhotos ---------------------------------------------------------------

// Минимальные структуры под photos.get: берём только то, что реально нужно.
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

// GetPhotos возвращает URL фотографий со стены группы — по одному (самому
// крупному) URL на каждое фото.
func (c *Client) GetPhotos(ownerID int64) ([]string, error) {
	params := url.Values{}
	params.Set("owner_id", strconv.FormatInt(ownerID, 10))
	params.Set("album_id", "wall")
	params.Set("count", defaultCount)

	var data photosGetResponse
	if err := c.call("photos.get", params, &data); err != nil {
		return nil, fmt.Errorf("get photos for owner %d: %w", ownerID, err)
	}

	// Аналог .map(...).filter(Boolean): идём циклом, для каждого фото берём
	// лучший размер, пустые пропускаем. Предалоцируем слайс под len(Items).
	urls := make([]string, 0, len(data.Items))
	for _, p := range data.Items {
		if best := bestPhotoURL(p.Sizes); best != "" {
			urls = append(urls, best)
		}
	}
	return urls, nil
}

// bestPhotoURL — ручной reduce: ищем размер с максимальной площадью.
// Типы "w" и "z" у VK — самые большие, поэтому max по площади их и выберет.
func bestPhotoURL(sizes []photoSize) string {
	best := ""
	maxArea := -1
	for _, s := range sizes {
		area := s.Width * s.Height
		if area > maxArea {
			maxArea = area
			best = s.URL
		}
	}
	return best
}

// --- GetMarketItems ----------------------------------------------------------

// Price — экспортируемая часть MarketItem (с Большой буквы = публичная).
type Price struct {
	Amount string `json:"amount"` // VK отдаёт сумму строкой (в копейках)
	Text   string `json:"text"`   // уже отформатированная цена, напр. "5 000 ₽"
}

// MarketItem торчит наружу (фигурирует в сигнатуре метода), поэтому он и его
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

// GetMarketItems возвращает товары из раздела «Товары» группы.
func (c *Client) GetMarketItems(ownerID int64) ([]MarketItem, error) {
	params := url.Values{}
	params.Set("owner_id", strconv.FormatInt(ownerID, 10))
	params.Set("count", defaultCount)

	var data marketGetResponse
	if err := c.call("market.get", params, &data); err != nil {
		return nil, fmt.Errorf("get market items for owner %d: %w", ownerID, err)
	}
	return data.Items, nil
}

// GetMarketItemByID тянет ОДИН товар по абсолютному id вида "<owner_id>_<item_id>"
// (например "-211011668_6377368"). Работает даже когда каталог скрыт настройками
// приватности и market.get отдаёт пусто.
//
// market.getById, как и большинство VK-методов, возвращает {count, items:[...]}
// — массив, даже если запрошен один товар. Поэтому переиспользуем ту же
// marketGetResponse и аккуратно достаём первый элемент.
func (c *Client) GetMarketItemByID(itemID string) (*MarketItem, error) {
	params := url.Values{}
	params.Set("item_ids", itemID)
	params.Set("extended", "1")

	var data marketGetResponse
	if err := c.call("market.getById", params, &data); err != nil {
		return nil, fmt.Errorf("get market item %q: %w", itemID, err)
	}

	// КЛЮЧЕВОЕ: в Go обращение к items[0] на пустом слайсе — это ПАНИКА
	// (runtime: index out of range), а не undefined как в JS. Поэтому ВСЕГДА
	// проверяем длину ПЕРЕД индексацией.
	if len(data.Items) == 0 {
		return nil, fmt.Errorf("market item %q not found", itemID)
	}

	// &data.Items[0] — указатель на элемент слайса, без копирования структуры.
	// Возвращаем *MarketItem: nil + error на «не найдено», валидный указатель иначе.
	return &data.Items[0], nil
}
