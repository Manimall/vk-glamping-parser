package vk

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
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

// GetPhotos возвращает URL фотографий со стены: выбирает limit ЛУЧШИХ по
// разрешению (площадь = ширина×высота отсекает мелкие репосты/скриншоты) и
// отдаёт их в исходном порядке стены. limit<=0 — без ограничения.
func (c *Client) GetPhotos(ctx context.Context, ownerID int64, limit int) ([]string, error) {
	params := url.Values{}
	params.Set("owner_id", strconv.FormatInt(ownerID, 10))
	params.Set("album_id", "wall")
	params.Set("count", defaultCount)

	var data photosGetResponse
	if err := c.call(ctx, "photos.get", params, &data); err != nil {
		return nil, fmt.Errorf("get photos for owner %d: %w", ownerID, err)
	}

	return selectBestPhotos(data.Items, limit), nil
}

// GetAlbumPhotos возвращает URL лучших размеров фото из КОНКРЕТНОГО альбома,
// в ПОРЯДКЕ АЛЬБОМА (не пересортировываем: владелец обычно ставит обзорные
// кадры первыми). Используется для «чистого» источника вместо стены.
func (c *Client) GetAlbumPhotos(ctx context.Context, ownerID int64, albumID string, count int) ([]string, error) {
	params := url.Values{}
	params.Set("owner_id", strconv.FormatInt(ownerID, 10))
	params.Set("album_id", albumID)
	params.Set("count", strconv.Itoa(count))

	var data photosGetResponse
	if err := c.call(ctx, "photos.get", params, &data); err != nil {
		return nil, fmt.Errorf("get album %s photos for owner %d: %w", albumID, ownerID, err)
	}

	urls := make([]string, 0, len(data.Items))
	for _, p := range data.Items {
		if u, _ := bestPhotoURL(p.Sizes); u != "" {
			urls = append(urls, u)
		}
	}
	return urls, nil
}

// selectBestPhotos выбирает limit лучших фото по площади и возвращает их URL в
// ИСХОДНОМ порядке (чтобы галерея читалась естественно, а не от крупного к мелкому).
func selectBestPhotos(photos []photo, limit int) []string {
	// scored — фото с его позицией и площадью лучшего размера.
	type scored struct {
		idx  int
		url  string
		area int
	}

	ranked := make([]scored, 0, len(photos))
	for i, p := range photos {
		if u, area := bestPhotoURL(p.Sizes); u != "" {
			ranked = append(ranked, scored{idx: i, url: u, area: area})
		}
	}

	// Меньше лимита (или лимит выключен) — отдаём все как есть.
	if limit > 0 && len(ranked) > limit {
		// По площади убыв. → берём top-limit → возвращаем в исходный порядок.
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].area > ranked[j].area })
		ranked = ranked[:limit]
		sort.Slice(ranked, func(i, j int) bool { return ranked[i].idx < ranked[j].idx })
	}

	urls := make([]string, len(ranked))
	for i, s := range ranked {
		urls[i] = s.url
	}
	return urls
}

// bestPhotoURL — ручной reduce: размер с максимальной площадью + сама площадь.
// Типы "w" и "z" у VK — самые большие, поэтому max по площади их и выберет.
func bestPhotoURL(sizes []photoSize) (string, int) {
	best := ""
	maxArea := -1
	for _, s := range sizes {
		area := s.Width * s.Height
		if area > maxArea {
			maxArea = area
			best = s.URL
		}
	}
	return best, maxArea
}

// GetMarketItemsByIDs тянет товары по абсолютным id вида "<owner_id>_<item_id>"
// за ОДИН вызов (item_ids у VK принимает список через запятую). Работает даже
// когда каталог скрыт настройками приватности и market.get отдаёт пусто.
func (c *Client) GetMarketItemsByIDs(ctx context.Context, itemIDs []string) ([]MarketItem, error) {
	if len(itemIDs) == 0 {
		return nil, nil
	}

	params := url.Values{}
	params.Set("item_ids", strings.Join(itemIDs, ","))
	params.Set("extended", "1") // полные поля товара (описание и т.п.)

	var data marketGetResponse
	if err := c.call(ctx, "market.getById", params, &data); err != nil {
		return nil, fmt.Errorf("get market items by ids: %w", err)
	}
	return data.Items, nil
}

// GetGroupInfo тянет инфо о сообществе по домену: название, описание, адрес/
// координаты, телефон. Работает ТОЛЬКО для групп (для пользователей VK вернёт
// ошибку — вызывающий решает, что с этим делать).
func (c *Client) GetGroupInfo(ctx context.Context, domain string) (*GroupInfo, error) {
	params := url.Values{}
	params.Set("group_id", domain)
	params.Set("fields", "description,place,city,contacts")

	var groups []groupByID
	if err := c.call(ctx, "groups.getById", params, &groups); err != nil {
		return nil, fmt.Errorf("get group info %q: %w", domain, err)
	}
	if len(groups) == 0 {
		return nil, fmt.Errorf("group %q not found", domain)
	}
	g := groups[0]

	// Маппим «сырые» поля VK в наш плоский GroupInfo. Адрес — из place, а если
	// его нет, подставляем город.
	info := &GroupInfo{
		Name:        g.Name,
		Description: g.Description,
		Address:     g.Place.Address,
		Latitude:    g.Place.Latitude,
		Longitude:   g.Place.Longitude,
	}
	if info.Address == "" {
		info.Address = g.City.Title
	}
	if len(g.Contacts) > 0 {
		info.Phone = g.Contacts[0].Phone
	}
	return info, nil
}
