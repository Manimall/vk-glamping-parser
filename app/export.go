package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"vk-parser/internal/config"
	"vk-parser/internal/images"
	"vk-parser/internal/objects"
	"vk-parser/internal/vk"
)

const (
	// albumFetchCount — сколько кадров тянем из альбома (с запасом на дедуп).
	albumFetchCount = 60
	// wallFetchCount — сколько тянем со стены (фоллбэк), тоже с запасом.
	wallFetchCount = 40
	// downloadTimeout — таймаут на скачивание одного фото.
	downloadTimeout = 20 * time.Second
)

// exportVK — то, что нужно экспорту от VK-клиента (accept interfaces). *vk.Client
// удовлетворяет автоматически; в тестах подставляем фейк без сети.
type exportVK interface {
	GetMarketItemsByIDs(ctx context.Context, itemIDs []string) ([]vk.MarketItem, error)
	GetAlbumPhotos(ctx context.Context, ownerID int64, albumID string, count int) ([]string, error)
	GetPhotos(ctx context.Context, ownerID int64, limit int) ([]string, error)
}

// runExport собирает готовую галерею photo-1..N.webp для объекта:
// owner → id альбома дома из описания товара → фото альбома (фоллбэк на стену) →
// скачать → images.Process (дедуп, без людей вперёд, ресайз/webp). Пишет в outDir.
func runExport(ctx context.Context, cfg *config.Config, domain, outDir string) error {
	if outDir == "" {
		outDir = filepath.Join("export", domain)
	}
	client := vk.NewClient(cfg.VKToken)

	ownerID, err := client.ResolveOwnerID(ctx, domain)
	if err != nil {
		return fmt.Errorf("export: resolve %q: %w", domain, err)
	}

	urls, source := photoURLs(ctx, client, cfg, domain, ownerID)
	if len(urls) == 0 {
		return fmt.Errorf("export: no photos for %q", domain)
	}
	slog.Info("export: источник фото", "domain", domain, "source", source, "urls", len(urls))

	raws := downloadAll(ctx, urls)
	slog.Info("export: скачано", "ok", len(raws), "из", len(urls))

	n, err := images.Process(ctx, raws, outDir, maxPhotos)
	if err != nil {
		return fmt.Errorf("export: process: %w", err)
	}
	slog.Info("export: готово", "domain", domain, "photos", n, "out", outDir)
	return nil
}

// photoURLs выбирает источник фото. Тонкая I/O-обёртка: достаёт id товаров из
// конфига объекта, дальше решение принимает чистый (тестируемый) resolveSource.
func photoURLs(ctx context.Context, client exportVK, cfg *config.Config, domain string, ownerID int64) ([]string, string) {
	return resolveSource(ctx, client, configuredItemIDs(cfg, domain, ownerID), ownerID)
}

// configuredItemIDs возвращает абсолютные id товаров объекта из его конфига
// (там же, откуда их берёт хендлер). Нет конфига/товаров — пустой список.
func configuredItemIDs(cfg *config.Config, domain string, ownerID int64) []string {
	obj, err := objects.Load(cfg.DataDir, domain)
	if err != nil {
		slog.Warn("export: object config", "domain", domain, "err", err)
		return nil
	}
	if obj == nil || len(obj.Items) == 0 {
		return nil
	}
	return marketIDsFromParam(strings.Join(obj.Items, ","), ownerID)
}

// resolveSource выбирает источник: альбом дома (id альбома из описания товара) →
// фоллбэк на стену. Возвращает URL и метку источника (для лога). Вся сеть — через
// интерфейс exportVK, поэтому функция покрыта тестами без обращения к VK.
func resolveSource(ctx context.Context, client exportVK, itemIDs []string, ownerID int64) ([]string, string) {
	if urls := albumURLs(ctx, client, itemIDs); len(urls) > 0 {
		return urls, "album"
	}

	// Фоллбэк: стена (когда альбома дома нет — напр. страница-пользователь).
	urls, err := client.GetPhotos(ctx, ownerID, wallFetchCount)
	if err != nil {
		slog.Warn("export: wall photos", "err", err)
		return nil, "wall"
	}
	return urls, "wall"
}

// albumURLs собирает URL из альбомов «ВСЕ ФОТО ДОМА», ссылки на которые указаны
// в описаниях товаров. Пусто, если товаров нет или альбомы недоступны.
func albumURLs(ctx context.Context, client exportVK, itemIDs []string) []string {
	if len(itemIDs) == 0 {
		return nil
	}
	items, err := client.GetMarketItemsByIDs(ctx, itemIDs)
	if err != nil {
		slog.Warn("export: market items", "err", err)
		return nil
	}

	var refs []images.AlbumRef
	for _, it := range items {
		refs = append(refs, images.AlbumRefsFromDescription(it.Description)...)
	}

	var urls []string
	for _, ref := range refs {
		got, err := client.GetAlbumPhotos(ctx, ref.OwnerID, ref.AlbumID, albumFetchCount)
		if err != nil {
			slog.Warn("export: album photos", "album", ref.AlbumID, "err", err)
			continue
		}
		urls = append(urls, got...)
	}
	return urls
}

// downloadAll скачивает картинки по URL. Ошибка одной не роняет остальные.
func downloadAll(ctx context.Context, urls []string) [][]byte {
	httpClient := &http.Client{Timeout: downloadTimeout}
	out := make([][]byte, 0, len(urls))
	for _, u := range urls {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			slog.Warn("export: download", "err", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			slog.Warn("export: download status", "url", u, "status", resp.StatusCode)
			resp.Body.Close()
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		out = append(out, data)
	}
	return out
}
