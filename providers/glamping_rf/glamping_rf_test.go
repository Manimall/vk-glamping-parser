package glamping_rf

import (
	"context"
	"testing"
)

// fakeFetcher — источник страниц из памяти (без сети): pages[place][pageIndex].
// details — detail по id (нет в мапе → транзиентный сбой); gone — id, чьи
// detail-страницы отдают 404 (объект снят с каталога).
type fakeFetcher struct {
	pages   map[int][]*apiResponse
	details map[int]*detailData
	gone    map[int]bool
	calls   int
}

func (f *fakeFetcher) fetchPage(_ context.Context, place, page int) (*apiResponse, error) {
	f.calls++
	ps := f.pages[place]
	if page-1 < len(ps) {
		return ps[page-1], nil
	}
	return &apiResponse{HasMore: false}, nil
}

func (f *fakeFetcher) fetchDetail(_ context.Context, id int) (*detailData, error) {
	if f.gone[id] {
		return nil, errDetailGone // объект снят с каталога → должен быть исключён
	}
	if d, ok := f.details[id]; ok {
		return d, nil
	}
	return nil, context.Canceled // транзиентный сбой — объект остаётся с дефолтами
}

func items(ids ...int) []apiItem {
	out := make([]apiItem, len(ids))
	for i, id := range ids {
		out[i] = apiItem{ID: id, Name: "obj", Price: apiPrice{Formatted: "1 ₽"}}
	}
	return out
}

func newTestProvider(f pageFetcher, dirs []direction) *Provider {
	return &Provider{fetcher: f, directions: dirs, delay: 0}
}

func TestParse_DedupAcrossPlaces(t *testing.T) {
	f := &fakeFetcher{pages: map[int][]*apiResponse{
		75: {{Items: items(1, 2), HasMore: false}},
		68: {{Items: items(2, 3), HasMore: false}}, // 2 — дубль
	}}
	p := newTestProvider(f, []direction{{name: "ЗК", places: []int{75, 68}}})

	out, err := p.Parse(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 3 { // {1,2,3}
		t.Fatalf("ожидал 3 уникальных, получил %d", len(out))
	}
}

func TestParse_PaginatesUntilNoMore(t *testing.T) {
	f := &fakeFetcher{pages: map[int][]*apiResponse{
		49: {
			{Items: items(1, 2), HasMore: true},
			{Items: items(3, 4), HasMore: true},
			{Items: items(5), HasMore: false},
		},
	}}
	p := newTestProvider(f, []direction{{name: "МО", places: []int{49}}})

	out, _ := p.Parse(context.Background())
	if len(out) != 5 {
		t.Fatalf("ожидал 5 (3 страницы), получил %d", len(out))
	}
}

func TestParse_FullCollectionDrainsAllDirections(t *testing.T) {
	f := &fakeFetcher{pages: map[int][]*apiResponse{
		75: {{Items: items(1), HasMore: false}},
		68: {{Items: items(2), HasMore: false}},
		49: { // большое направление вычерпывается ЦЕЛИКОМ (ранней остановки нет)
			{Items: items(10, 11), HasMore: true},
			{Items: items(12, 13), HasMore: false},
		},
	}}
	dirs := []direction{
		{name: "ЗК", places: []int{75, 68}},
		{name: "МО", places: []int{49}},
	}
	p := newTestProvider(f, dirs)

	out, _ := p.Parse(context.Background())
	// Полный сбор: ЗК (1,2) + МО обе страницы (10,11,12,13) = 6.
	if len(out) != 6 {
		t.Fatalf("ожидал 6 (полный сбор всех направлений), получил %d", len(out))
	}
}
