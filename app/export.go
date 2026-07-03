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
	// exportPhotoLimit — сколько фото в итоговой галерее.
	exportPhotoLimit = 15
	// albumFetchCount — сколько кадров тянем из альбома (с запасом на дедуп).
	albumFetchCount = 60
	// wallFetchCount — сколько тянем со стены (фоллбэк), тоже с запасом.
	wallFetchCount = 40
)

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

	n, err := images.Process(ctx, raws, outDir, exportPhotoLimit)
	if err != nil {
		return fmt.Errorf("export: process: %w", err)
	}
	slog.Info("export: готово", "domain", domain, "photos", n, "out", outDir)
	return nil
}

// photoURLs выбирает источник: альбом дома (id из описания товара) → фоллбэк
// на стену. Возвращает URL и метку источника (для лога).
func photoURLs(ctx context.Context, client *vk.Client, cfg *config.Config, domain string, ownerID int64) ([]string, string) {
	// id товаров берём из конфига объекта (там же, откуда их берёт хендлер).
	var itemsRaw string
	if obj, err := objects.Load(cfg.DataDir, domain); err != nil {
		slog.Warn("export: object config", "domain", domain, "err", err)
	} else if obj != nil && len(obj.Items) > 0 {
		itemsRaw = strings.Join(obj.Items, ",")
	}

	// Из описаний товаров достаём ссылки на альбомы «ВСЕ ФОТО ДОМА».
	var refs []images.AlbumRef
	if ids := marketIDsFromParam(itemsRaw, ownerID); len(ids) > 0 {
		if items, err := client.GetMarketItemsByIDs(ctx, ids); err != nil {
			slog.Warn("export: market items", "err", err)
		} else {
			for _, it := range items {
				refs = append(refs, images.AlbumRefsFromDescription(it.Description)...)
			}
		}
	}

	if len(refs) > 0 {
		var urls []string
		for _, ref := range refs {
			albumURLs, err := client.GetAlbumPhotos(ctx, ref.OwnerID, ref.AlbumID, albumFetchCount)
			if err != nil {
				slog.Warn("export: album photos", "album", ref.AlbumID, "err", err)
				continue
			}
			urls = append(urls, albumURLs...)
		}
		if len(urls) > 0 {
			return urls, "album"
		}
	}

	// Фоллбэк: стена (когда альбома дома нет — напр. страница-пользователь).
	urls, err := client.GetPhotos(ctx, ownerID, wallFetchCount)
	if err != nil {
		slog.Warn("export: wall photos", "err", err)
		return nil, "wall"
	}
	return urls, "wall"
}

// downloadAll скачивает картинки по URL. Ошибка одной не роняет остальные.
func downloadAll(ctx context.Context, urls []string) [][]byte {
	httpClient := &http.Client{Timeout: 20 * time.Second}
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
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		out = append(out, data)
	}
	return out
}
