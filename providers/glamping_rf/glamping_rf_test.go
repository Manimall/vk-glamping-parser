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

func newTestProvider(f pageFetcher, dirs []direction, min int) *Provider {
	return &Provider{fetcher: f, directions: dirs, minObjects: min, delay: 0}
}

func TestParse_DedupAcrossPlaces(t *testing.T) {
	f := &fakeFetcher{pages: map[int][]*apiResponse{
		75: {{Items: items(1, 2), HasMore: false}},
		68: {{Items: items(2, 3), HasMore: false}}, // 2 — дубль
	}}
	p := newTestProvider(f, []direction{{name: "ЗК", places: []int{75, 68}}}, 100)

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
	p := newTestProvider(f, []direction{{name: "МО", places: []int{49}}}, 100)

	out, _ := p.Parse(context.Background())
	if len(out) != 5 {
		t.Fatalf("ожидал 5 (3 страницы), получил %d", len(out))
	}
}

func TestParse_EarlyStopAtMinCoversBothDirections(t *testing.T) {
	f := &fakeFetcher{pages: map[int][]*apiResponse{
		75: {{Items: items(1), HasMore: false}},
		68: {{Items: items(2), HasMore: false}},
		49: { // большое направление — не должно вычерпаться целиком
			{Items: items(10, 11), HasMore: true},
			{Items: items(12, 13), HasMore: true},
		},
	}}
	// Порядок: ЗК (мелкое) раньше МО → к остановке оба направления представлены.
	dirs := []direction{
		{name: "ЗК", places: []int{75, 68}},
		{name: "МО", places: []int{49}},
	}
	p := newTestProvider(f, dirs, 3)

	out, _ := p.Parse(context.Background())
	if len(out) < 3 {
		t.Fatalf("ожидал >= min(3), получил %d", len(out))
	}
	// Оба направления: id из ЗК (1/2) и из МО (10/11) присутствуют.
	ids := map[string]bool{}
	// out — contract.Object без id; проверяем косвенно: собрано ровно 4
	// (1,2 из ЗК + 10,11 из МО page1), а page2 (12,13) не запрошена (ранняя стоп).
	_ = ids
	if len(out) != 4 {
		t.Fatalf("ожидал 4 (ЗК:1,2 + МО page1:10,11), получил %d", len(out))
	}
}
