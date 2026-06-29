package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"

	"vk-parser/internal/extract"
	"vk-parser/internal/objects"
)

// handleGlamping — МЕТОД server. Сигнатура — ровно http.HandlerFunc (w, r), без
// зависимостей в аргументах: они доступны через ресивер s.
func (s *server) handleGlamping(w http.ResponseWriter, r *http.Request) {
	q := glampingQuery{
		domain: r.URL.Query().Get("domain"),
		items:  r.URL.Query().Get("items"),
		coords: r.URL.Query().Get("coords"),
		mapURL: r.URL.Query().Get("map"),
	}
	if q.domain == "" {
		http.Error(w, "query param 'domain' is required", http.StatusBadRequest)
		return
	}

	// Cache hit — отдаём из памяти, в VK не ходим.
	if data, ok := s.store.Get(q.cacheKey()); ok {
		w.Header().Set("X-Cache", "HIT")
		writeJSON(w, data)
		return
	}

	// Cache miss → идём в VK. r.Context() отменяется, если клиент отвалился.
	data, err := s.buildGlampingData(r.Context(), q)
	if err != nil {
		log.Printf("build data for %q: %v", q.domain, err)
		http.Error(w, "failed to fetch data from VK", http.StatusBadGateway)
		return
	}

	s.store.Set(q.cacheKey(), data)
	w.Header().Set("X-Cache", "MISS")
	writeJSON(w, data)
}

// buildGlampingData — «бизнес-логика» одного запроса: объект-уровень (инфо
// группы + галерея) и список домиков из VK-товаров и/или конфига.
func (s *server) buildGlampingData(ctx context.Context, q glampingQuery) (GlampingData, error) {
	ownerID, err := s.client.ResolveOwnerID(ctx, q.domain)
	if err != nil {
		return GlampingData{}, fmt.Errorf("resolve: %w", err)
	}

	photos, err := s.client.GetPhotos(ctx, ownerID, maxPhotos)
	if err != nil {
		return GlampingData{}, fmt.Errorf("photos: %w", err)
	}

	data := GlampingData{Photos: photos, Cabins: []Cabin{}}

	// Объект-уровень: название/локация/контакт берём из инфо сообщества.
	// Метод только для групп; для пользователей VK вернёт ошибку — graceful.
	if info, err := s.client.GetGroupInfo(ctx, q.domain); err != nil {
		log.Printf("group info for %q: %v (пропускаю инфо объекта)", q.domain, err)
	} else {
		data.Title = info.Name
		data.About = info.Description
		data.Location = info.Address
		data.Contact = info.Phone
		if info.Latitude != 0 || info.Longitude != 0 {
			data.Coords = &Coords{Lat: info.Latitude, Lon: info.Longitude}
		}
	}

	// Пер-объектный конфиг data/<domain>.json — ручные данные, которых нет в VK
	// (координаты, карта, id товаров, «ручные» домики с Avito). Нет файла — nil.
	cfg, err := objects.Load(s.dataDir, q.domain)
	if err != nil {
		log.Printf("object config %q: %v (пропускаю)", q.domain, err)
	}

	// Слияние источников. Приоритет — у параметров запроса; чего нет в запросе,
	// берём из конфига. Так конфиг = «значения по умолчанию для объекта».
	coordsRaw, mapURL, itemsRaw := q.coords, q.mapURL, q.items
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
		// Адрес из конфига точнее города из VK — если задан, используем его.
		if cfg.Address != "" {
			data.Location = cfg.Address
		}
		manual = cfg.Cabins
	}

	if c, ok := parseCoords(coordsRaw); ok {
		data.Coords = c
	}
	data.MapURL = mapURL

	// Фоллбэк: координат нет, но есть адрес → получаем их геокодером (бесплатно).
	// Ручные координаты приоритетнее, поэтому ходим в сеть только при их отсутствии.
	if data.Coords == nil && data.Location != "" {
		if c, err := s.geocoder.Geocode(ctx, data.Location); err != nil {
			log.Printf("geocode %q: %v (без координат)", data.Location, err)
		} else {
			data.Coords = &Coords{Lat: c.Lat, Lon: c.Lon}
		}
	}

	// «Сырые» домики из двух источников: товары VK (по прямым id) и ручные
	// домики из конфига (например, описание с Avito, который ботами не парсится).
	raw := make([]objects.Cabin, 0)
	if ids := marketIDsFromParam(itemsRaw, ownerID); len(ids) > 0 {
		if items, err := s.client.GetMarketItemsByIDs(ctx, ids); err != nil {
			log.Printf("market items %v: %v (пропускаю товары)", ids, err)
		} else {
			for _, item := range items {
				raw = append(raw, objects.Cabin{
					Title:       item.Title,
					Price:       item.Price.Text,
					Description: item.Description,
				})
			}
		}
	}
	raw = append(raw, manual...)

	data.Cabins = s.structureCabins(ctx, raw, data.Location, len(photos))
	return data, nil
}

// structureCabins прогоняет каждый «сырой» домик через извлекатель (что в нём
// есть) и схлопывает почти одинаковые варианты (дедуп).
func (s *server) structureCabins(ctx context.Context, raw []objects.Cabin, location string, photoCount int) []Cabin {
	cabins := make([]Cabin, 0, len(raw))
	for _, rc := range raw {
		cabin := Cabin{Title: rc.Title, Price: rc.Price, Description: rc.Description}
		// Структурируем описание домика → что в нём есть (главное для нас).
		listing := extract.Listing{
			Title:       rc.Title,
			Description: rc.Description,
			Location:    location,
			Price:       rc.Price,
			PhotoCount:  photoCount,
		}
		if prop, err := s.extractor.Extract(ctx, listing); err != nil {
			log.Printf("extract cabin %q: %v", rc.Title, err)
		} else {
			cabin.Property = prop
		}
		cabins = append(cabins, cabin)
	}
	return dedupCabins(cabins)
}
