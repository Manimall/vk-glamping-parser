package vkprovider

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"vk-parser/internal/contract"
	"vk-parser/internal/extract"
	"vk-parser/internal/objects"
	"vk-parser/internal/vk"
)

const (
	// maxPhotos — сколько лучших фото оставляем в галерее (перенесено из app).
	maxPhotos = 15
	// objectDelay — пауза между объектами при пакетном сборе: VK лимитирует ~3
	// запроса/с, а каждый объект делает несколько вызовов. HTTP-режим (один объект
	// на запрос) задержку не использует — поведение не меняется.
	objectDelay = 400 * time.Millisecond
)

// vkAPI — то, что провайдеру нужно от VK. *vk.Client удовлетворяет структурно;
// в тестах подставляем фейк без сети (интерфейс объявляет потребитель — идиома Go).
type vkAPI interface {
	ResolveOwnerID(ctx context.Context, domain string) (int64, error)
	GetPhotos(ctx context.Context, ownerID int64, limit int) ([]string, error)
	GetGroupInfo(ctx context.Context, domain string) (*vk.GroupInfo, error)
	GetMarketItemsByIDs(ctx context.Context, itemIDs []string) ([]vk.MarketItem, error)
}

// geocoderAPI — геокодер (адрес → координаты). *geocode.Client удовлетворяет.
type geocoderAPI interface {
	Geocode(ctx context.Context, address string) (lat, lon float64, err error)
}

// Query — параметры сбора одного объекта. Приоритет у полей запроса; чего нет —
// берётся из конфига объекта data/<domain>.json.
type Query struct {
	Domain string
	Items  string // товары-домики (URL/id через запятую)
	Coords string // "lat,lon" — если VK не отдал координаты
	MapURL string // ссылка на карту
}

// Parser собирает карточки объектов из VK. Зависимости — интерфейсы (DIP).
type Parser struct {
	client    vkAPI
	extractor extract.Extractor
	geocoder  geocoderAPI
	dataDir   string
}

// New собирает провайдера VK из зависимостей (composition root — в main).
func New(client vkAPI, extractor extract.Extractor, geocoder geocoderAPI, dataDir string) *Parser {
	return &Parser{client: client, extractor: extractor, geocoder: geocoder, dataDir: dataDir}
}

// Name — имя источника (каталог generated/vk).
func (p *Parser) Name() string { return "vk" }

// Parse — пакетный сбор всех настроенных объектов (data/*.json). Сбой одного не
// роняет остальные (graceful, WARN). Реализует providers.Provider.
func (p *Parser) Parse(ctx context.Context) ([]contract.Object, error) {
	domains, err := configuredDomains(p.dataDir)
	if err != nil {
		return nil, err
	}
	out := make([]contract.Object, 0, len(domains))
	for i, d := range domains {
		if i > 0 {
			if !sleepCtx(ctx, objectDelay) {
				return out, ctx.Err() // ctx отменён — отдаём собранное
			}
		}
		obj, err := p.Build(ctx, Query{Domain: d})
		if err != nil {
			slog.Warn("vk: объект пропущен", "domain", d, "err", err)
			continue
		}
		out = append(out, obj)
		slog.Info("vk: объект собран", "domain", d, "cabins", len(obj.Cabins))
	}
	return out, nil
}

// sleepCtx спит d, прерываясь при отмене ctx. Возвращает false, если ctx отменён.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// Build — «бизнес-логика» одного объекта: объект-уровень (инфо группы + галерея) и
// список домиков из VK-товаров и/или конфига. Перенесено из app без изменения
// поведения; HTTP-обработчик теперь делегирует сюда.
func (p *Parser) Build(ctx context.Context, q Query) (contract.Object, error) {
	ownerID, err := p.client.ResolveOwnerID(ctx, q.Domain)
	if err != nil {
		return contract.Object{}, fmt.Errorf("resolve: %w", err)
	}

	photos, err := p.client.GetPhotos(ctx, ownerID, maxPhotos)
	if err != nil {
		return contract.Object{}, fmt.Errorf("photos: %w", err)
	}

	data := contract.Object{Photos: photos, Cabins: []contract.Cabin{}}

	// Объект-уровень: название/локация/контакт из инфо сообщества. Метод только для
	// групп; для пользователей VK вернёт ошибку — graceful.
	if info, err := p.client.GetGroupInfo(ctx, q.Domain); err != nil {
		slog.Warn("group info skipped", "domain", q.Domain, "err", err)
	} else {
		data.Title = info.Name
		data.About = info.Description
		data.Location = info.Address
		data.Contact = info.Phone
		if info.Latitude != 0 || info.Longitude != 0 {
			data.Coords = &contract.Coords{Lat: info.Latitude, Lon: info.Longitude}
		}
	}

	// Пер-объектный конфиг data/<domain>.json — ручные данные, которых нет в VK.
	cfg, err := objects.Load(p.dataDir, q.Domain)
	if err != nil {
		slog.Warn("object config skipped", "domain", q.Domain, "err", err)
	}

	// Слияние источников: приоритет у полей запроса; чего нет — из конфига.
	coordsRaw, mapURL, itemsRaw := q.Coords, q.MapURL, q.Items
	var manual []objects.Cabin
	if cfg != nil {
		if coordsRaw == "" {
			coordsRaw = cfg.Coords
		}
		if mapURL == "" {
			mapURL = cfg.Map
		}
		if itemsRaw == "" && len(cfg.Items) > 0 {
			itemsRaw = strings.Join(cfg.Items, ",")
		}
		if cfg.Address != "" {
			data.Location = cfg.Address
		}
		manual = cfg.Cabins
	}

	if c, ok := parseCoords(coordsRaw); ok {
		data.Coords = c
	}
	data.MapURL = mapURL

	// Фоллбэк: координат нет, но есть адрес → геокодер (бесплатно). Ручные
	// координаты приоритетнее, поэтому в сеть идём только при их отсутствии.
	if data.Coords == nil && data.Location != "" {
		if lat, lon, err := p.geocoder.Geocode(ctx, data.Location); err != nil {
			slog.Warn("geocode failed", "address", data.Location, "err", err)
		} else {
			data.Coords = &contract.Coords{Lat: lat, Lon: lon}
		}
	}

	// «Сырые» домики: товары VK (по id) + ручные домики из конфига.
	raw := make([]objects.Cabin, 0)
	for _, item := range p.fetchMarketItems(ctx, itemsRaw, ownerID, "market items") {
		raw = append(raw, objects.Cabin{
			Title:       item.Title,
			Price:       item.Price.Text,
			Description: item.Description,
		})
	}
	raw = append(raw, manual...)

	data.Cabins = p.structureCabins(ctx, raw, data.Location, len(photos))

	// Товары-услуги (фурако, наполнение…) — доп.услуги объекта.
	if cfg != nil {
		for _, item := range p.fetchMarketItems(ctx, strings.Join(cfg.Extras, ","), ownerID, "extra items") {
			data.Extras = append(data.Extras, extract.Extra{Name: item.Title, Price: item.Price.Text})
		}
	}

	// SEO/OG-тексты из контента (презентует место, без списка удобств; бренд — на фронте).
	if len(data.Cabins) > 0 {
		seo := extract.BuildSEO(extract.SEOInput{
			Name:     data.Cabins[0].Title,
			Location: data.Location,
			About:    data.About,
		})
		data.Seo = &seo
	}

	return data, nil
}

// fetchMarketItems тянет товары VK по строке id/URL. Пусто/ошибка → nil (graceful,
// WARN с меткой what). Общий шаг для домиков и услуг (DRY).
func (p *Parser) fetchMarketItems(ctx context.Context, param string, ownerID int64, what string) []vk.MarketItem {
	ids := MarketIDsFromParam(param, ownerID)
	if len(ids) == 0 {
		return nil
	}
	items, err := p.client.GetMarketItemsByIDs(ctx, ids)
	if err != nil {
		slog.Warn(what+" skipped", "ids", ids, "err", err)
		return nil
	}
	return items
}

// structureCabins прогоняет каждый «сырой» домик через извлекатель и схлопывает
// почти одинаковые варианты (дедуп).
func (p *Parser) structureCabins(ctx context.Context, raw []objects.Cabin, location string, photoCount int) []contract.Cabin {
	cabins := make([]contract.Cabin, 0, len(raw))
	for _, rc := range raw {
		cabin := contract.Cabin{Title: rc.Title, Price: rc.Price, Description: rc.Description}
		listing := extract.Listing{
			Title:       rc.Title,
			Description: rc.Description,
			Location:    location,
			Price:       rc.Price,
			PhotoCount:  photoCount,
		}
		if prop, err := p.extractor.Extract(ctx, listing); err != nil {
			slog.Warn("extract cabin failed", "title", rc.Title, "err", err)
		} else {
			cabin.Property = prop
		}
		cabins = append(cabins, cabin)
	}
	return dedupCabins(cabins)
}
