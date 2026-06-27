package vk

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// ResolveOwnerID превращает короткое имя (домен) в owner_id для последующих
// вызовов photos.get / market.get.
//
// VK кодирует тип владельца ЗНАКОМ owner_id:
//   - группа (community)  → owner_id ОТРИЦАТЕЛЬНЫЙ  (-object_id)
//   - пользователь (user) → owner_id ПОЛОЖИТЕЛЬНЫЙ (+object_id)
func (c *Client) ResolveOwnerID(ctx context.Context, domain string) (int64, error) {
	params := url.Values{}
	params.Set("screen_name", domain)

	var resolved resolvedScreenName
	if err := c.call(ctx, "utils.resolveScreenName", params, &resolved); err != nil {
		return 0, fmt.Errorf("resolve %q: %w", domain, err)
	}

	switch resolved.Type {
	case "group":
		return -resolved.ObjectID, nil
	case "user":
		return resolved.ObjectID, nil
	default:
		return 0, fmt.Errorf("screen name %q has unsupported type %q",
			domain, resolved.Type)
	}
}

// GetPhotos возвращает URL фотографий со стены — по одному (самому крупному)
// URL на каждое фото.
func (c *Client) GetPhotos(ctx context.Context, ownerID int64) ([]string, error) {
	params := url.Values{}
	params.Set("owner_id", strconv.FormatInt(ownerID, 10))
	params.Set("album_id", "wall")
	params.Set("count", defaultCount)

	var data photosGetResponse
	if err := c.call(ctx, "photos.get", params, &data); err != nil {
		return nil, fmt.Errorf("get photos for owner %d: %w", ownerID, err)
	}

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

// GetMarketItems возвращает товары из раздела «Товары» владельца.
func (c *Client) GetMarketItems(ctx context.Context, ownerID int64) ([]MarketItem, error) {
	params := url.Values{}
	params.Set("owner_id", strconv.FormatInt(ownerID, 10))
	params.Set("count", defaultCount)

	var data marketGetResponse
	if err := c.call(ctx, "market.get", params, &data); err != nil {
		return nil, fmt.Errorf("get market items for owner %d: %w", ownerID, err)
	}
	return data.Items, nil
}

// GetMarketItemByID тянет ОДИН товар по абсолютному id вида "<owner_id>_<item_id>"
// (например "-211011668_6377368"). Работает даже когда каталог скрыт настройками
// приватности и market.get отдаёт пусто.
func (c *Client) GetMarketItemByID(ctx context.Context, itemID string) (*MarketItem, error) {
	params := url.Values{}
	params.Set("item_ids", itemID)
	params.Set("extended", "1")

	var data marketGetResponse
	if err := c.call(ctx, "market.getById", params, &data); err != nil {
		return nil, fmt.Errorf("get market item %q: %w", itemID, err)
	}

	// items[0] на пустом слайсе — ПАНИКА (index out of range), а не undefined
	// как в JS. Поэтому проверяем длину ПЕРЕД индексацией.
	if len(data.Items) == 0 {
		return nil, fmt.Errorf("market item %q not found", itemID)
	}

	return &data.Items[0], nil
}
