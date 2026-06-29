package main

import (
	"context"
	"testing"
	"time"

	"vk-parser/internal/cache"
	"vk-parser/internal/extract"
	"vk-parser/internal/vk"
)

// --- Фейки зависимостей (удовлетворяют vkAPI / geocoderAPI без сети) ----------

type fakeVK struct {
	ownerID  int64
	photos   []string
	group    *vk.GroupInfo
	groupErr error
	items    []vk.MarketItem
}

func (f *fakeVK) ResolveOwnerID(_ context.Context, _ string) (int64, error) {
	return f.ownerID, nil
}
func (f *fakeVK) GetPhotos(_ context.Context, _ int64, _ int) ([]string, error) {
	return f.photos, nil
}
func (f *fakeVK) GetGroupInfo(_ context.Context, _ string) (*vk.GroupInfo, error) {
	return f.group, f.groupErr
}
func (f *fakeVK) GetMarketItemsByIDs(_ context.Context, _ []string) ([]vk.MarketItem, error) {
	return f.items, nil
}

type fakeGeocoder struct {
	lat, lon float64
	err      error
	called   bool
}

func (f *fakeGeocoder) Geocode(_ context.Context, _ string) (lat, lon float64, err error) {
	f.called = true
	return f.lat, f.lon, f.err
}

// newTestServer собирает server на фейках + реальной (офлайн) эвристике.
func newTestServer(fvk *fakeVK, fgeo *fakeGeocoder) *server {
	return &server{
		client:    fvk,
		store:     cache.New[GlampingData](time.Minute),
		extractor: extract.NewHeuristic(),
		geocoder:  fgeo,
		dataDir:   "testdata",
	}
}

// --- Тесты buildGlampingData -------------------------------------------------

// Домики из VK-товаров (как elkidom): передан items, конфига нет.
func TestBuildFromItems(t *testing.T) {
	fvk := &fakeVK{
		ownerID: -123,
		photos:  []string{"p1", "p2"},
		group:   &vk.GroupInfo{Name: "Тест Глэмпинг"}, // без адреса → геокодер не нужен
		items: []vk.MarketItem{
			{Title: "AFRAME", Description: "Кухня, баня, мангал.", Price: vk.Price{Text: "7000 ₽"}},
		},
	}
	fgeo := &fakeGeocoder{}
	srv := newTestServer(fvk, fgeo)

	data, err := srv.buildGlampingData(context.Background(), glampingQuery{
		domain: "noconfig", // нет testdata/noconfig.json
		items:  "6377368",
	})
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if data.Title != "Тест Глэмпинг" {
		t.Errorf("title = %q, ожидал из инфо группы", data.Title)
	}
	if len(data.Cabins) != 1 || data.Cabins[0].Title != "AFRAME" {
		t.Fatalf("ожидал 1 домик 'AFRAME', получил %+v", data.Cabins)
	}
	if fgeo.called {
		t.Error("геокодер не должен вызываться без адреса")
	}
}

// Домик из конфига с Avito-описанием (как scandi): группа недоступна (VK-юзер),
// координаты и адрес — из конфига, геокодер не нужен.
func TestBuildFromConfig(t *testing.T) {
	fvk := &fakeVK{
		ownerID:  883778506,
		photos:   []string{"p1"},
		groupErr: errFake, // имитируем ошибку groups.getById для страницы-юзера
	}
	fgeo := &fakeGeocoder{}
	srv := newTestServer(fvk, fgeo)

	data, err := srv.buildGlampingData(context.Background(), glampingQuery{domain: "cfg_coords"})
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if data.Location != "Тестовый адрес, д. Тест" {
		t.Errorf("location = %q, ожидал из конфига", data.Location)
	}
	if data.Coords == nil || data.Coords.Lat != 57.0 || data.Coords.Lon != 41.0 {
		t.Errorf("coords = %+v, ожидал {57,41} из конфига", data.Coords)
	}
	if data.MapURL != "https://map.example/test" {
		t.Errorf("mapUrl = %q, ожидал из конфига", data.MapURL)
	}
	if len(data.Cabins) != 1 || data.Cabins[0].Title != "Тест-домик" {
		t.Fatalf("ожидал домик из конфига, получил %+v", data.Cabins)
	}
	if fgeo.called {
		t.Error("геокодер не должен вызываться — координаты есть в конфиге")
	}
}

// Координат нет нигде, но есть адрес → срабатывает геокодер.
func TestGeocoderFallback(t *testing.T) {
	fvk := &fakeVK{ownerID: 1, groupErr: errFake}
	fgeo := &fakeGeocoder{lat: 56.99, lon: 40.98}
	srv := newTestServer(fvk, fgeo)

	data, err := srv.buildGlampingData(context.Background(), glampingQuery{domain: "cfg_nocoords"})
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if !fgeo.called {
		t.Fatal("геокодер должен был вызваться — координат нет, адрес есть")
	}
	if data.Coords == nil || data.Coords.Lat != 56.99 {
		t.Errorf("coords = %+v, ожидал от геокодера", data.Coords)
	}
}

// Ручные координаты приоритетнее геокодера: при наличии координат в сеть не идём.
func TestManualCoordsBeatGeocoder(t *testing.T) {
	fvk := &fakeVK{ownerID: 1, groupErr: errFake}
	fgeo := &fakeGeocoder{lat: 99, lon: 99} // не должен быть использован
	srv := newTestServer(fvk, fgeo)

	data, err := srv.buildGlampingData(context.Background(), glampingQuery{domain: "cfg_coords"})
	if err != nil {
		t.Fatalf("ошибка: %v", err)
	}
	if fgeo.called {
		t.Error("геокодер не должен вызываться при заданных координатах")
	}
	if data.Coords.Lat != 57.0 {
		t.Errorf("coords = %+v, ожидал ручные {57,41}", data.Coords)
	}
}

// errFake — простая ошибка для имитации недоступного groups.getById.
var errFake = &fakeError{}

type fakeError struct{}

func (*fakeError) Error() string { return "fake: not available" }
