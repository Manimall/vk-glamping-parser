package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"vk-parser/internal/vk"
)

// fakeExportVK — фейк exportVK без сети. Полями задаём, что вернёт каждый метод.
type fakeExportVK struct {
	items     []vk.MarketItem
	itemsErr  error
	albumURLs map[string][]string // albumID → фото
	albumErr  error
	wallURLs  []string
	wallErr   error
}

func (f *fakeExportVK) GetMarketItemsByIDs(_ context.Context, _ []string) ([]vk.MarketItem, error) {
	return f.items, f.itemsErr
}

func (f *fakeExportVK) GetAlbumPhotos(_ context.Context, _ int64, albumID string, _ int) ([]string, error) {
	if f.albumErr != nil {
		return nil, f.albumErr
	}
	return f.albumURLs[albumID], nil
}

func (f *fakeExportVK) GetPhotos(_ context.Context, _ int64, _ int) ([]string, error) {
	return f.wallURLs, f.wallErr
}

func TestResolveSourceAlbum(t *testing.T) {
	// Товар с ссылкой на альбом дома → берём фото альбома, метка "album".
	fake := &fakeExportVK{
		items:     []vk.MarketItem{{Description: "ВСЕ ФОТО ДОМА: vk.com/album-211011668_282686075"}},
		albumURLs: map[string][]string{"282686075": {"a1", "a2"}},
		wallURLs:  []string{"w1"}, // не должно использоваться
	}
	urls, source := resolveSource(context.Background(), fake, []string{"-211011668_100"}, -211011668)

	if source != "album" {
		t.Fatalf("ожидал источник album, получил %q", source)
	}
	if len(urls) != 2 || urls[0] != "a1" {
		t.Errorf("ожидал фото альбома [a1 a2], получил %v", urls)
	}
}

func TestResolveSourceFallbackToWall(t *testing.T) {
	// В описании нет ссылки на альбом → фоллбэк на стену.
	fake := &fakeExportVK{
		items:    []vk.MarketItem{{Description: "без ссылки на альбом"}},
		wallURLs: []string{"w1", "w2"},
	}
	urls, source := resolveSource(context.Background(), fake, []string{"-1_1"}, -1)

	if source != "wall" {
		t.Fatalf("ожидал источник wall, получил %q", source)
	}
	if len(urls) != 2 {
		t.Errorf("ожидал 2 фото стены, получил %v", urls)
	}
}

func TestResolveSourceAlbumErrorFallsBack(t *testing.T) {
	// Ссылка на альбом есть, но его фото недоступны → фоллбэк на стену, не паника.
	fake := &fakeExportVK{
		items:    []vk.MarketItem{{Description: "vk.com/album-1_2"}},
		albumErr: errors.New("private album"),
		wallURLs: []string{"w1"},
	}
	urls, source := resolveSource(context.Background(), fake, []string{"-1_1"}, -1)

	if source != "wall" || len(urls) != 1 {
		t.Errorf("при ошибке альбома ожидал фоллбэк на стену [w1], получил %q %v", source, urls)
	}
}

func TestDownloadAllGraceful(t *testing.T) {
	// Сервер отдаёт 200 с телом на /ok и 404 на остальном.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.Write([]byte("payload"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	urls := []string{srv.URL + "/ok", srv.URL + "/missing", "http://%zz"} // 200, 404, битый URL
	got := downloadAll(context.Background(), urls)

	if len(got) != 1 {
		t.Fatalf("ожидал 1 успешную загрузку (404 и битый URL пропущены), получил %d", len(got))
	}
	if string(got[0]) != "payload" {
		t.Errorf("получил тело %q, ожидал 'payload'", got[0])
	}
}
