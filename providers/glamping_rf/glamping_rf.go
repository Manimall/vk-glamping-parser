// Package glamping_rf — провайдер данных с сайта глэмпинги.рф.
//
// Источник — внутренний JSON-API каталога (OpenCart, route=product/category/list):
// чистые данные без парсинга HTML DOM («умное получение данных»). Регион задаётся
// фильтром place, выдача постранична (has_more) — идём по страницам с задержкой,
// пока не наберём minObjects уникальных объектов или не кончатся страницы.
//
// Реализует providers.Provider и отдаёт contract.Object — тот же формат, что VK.
package glamping_rf

import (
	"context"
	"log/slog"
	"time"

	"vk-parser/internal/contract"
	"vk-parser/providers"
)

const (
	// minObjects — целевой минимум уникальных объектов по ТЗ.
	minObjects = 100
	// pageDelay — пауза между запросами страниц (вежливость к серверу, анти-бан).
	pageDelay = 700 * time.Millisecond
	// maxPagesPerPlace — предохранитель от бесконечной пагинации (328/40 ≈ 9 стр.).
	maxPagesPerPlace = 50
)

// direction — направление сбора по ТЗ: имя + фильтры place (регионы каталога).
type direction struct {
	name   string
	places []int
}

// defaultDirections — ДВА направления из ТЗ. Порядок намеренный: меньшее «Золотое
// кольцо» раньше большого «Подмосковья» — чтобы к моменту ранней остановки по
// minObjects оба направления уже были представлены в выборке. Регион = place-id
// каталога (найдены при разведке: 49 — near-Moscow ~328, 75 — Ярославская обл.,
// 68 — Тверская обл.).
var defaultDirections = []direction{
	{name: "Золотое кольцо", places: []int{75, 68}},
	{name: "Московская область", places: []int{49}},
}

// Provider собирает объекты глэмпинги.рф. fetcher — интерфейс (в тестах фейк).
type Provider struct {
	fetcher    pageFetcher
	directions []direction
	minObjects int
	delay      time.Duration
}

// New собирает провайдера с реальным HTTP-клиентом и конфигом по умолчанию.
func New() *Provider {
	return &Provider{
		fetcher:    newClient(),
		directions: defaultDirections,
		minObjects: minObjects,
		delay:      pageDelay,
	}
}

// Name — имя источника (каталог вывода generated/glamping_rf).
func (p *Provider) Name() string { return "glamping_rf" }

// Parse обходит направления и их регионы, набирая уникальные (по id) объекты.
// Останавливается, как только собрано minObjects, либо когда страницы кончились.
func (p *Provider) Parse(ctx context.Context) ([]contract.Object, error) {
	seen := make(map[int]bool)
	out := make([]contract.Object, 0, p.minObjects)

	for _, dir := range p.directions {
		for _, place := range dir.places {
			p.collectPlace(ctx, dir.name, place, seen, &out)
			if len(out) >= p.minObjects {
				slog.Info("glamping_rf: собрано достаточно",
					"unique", len(out), "min", p.minObjects)
				return out, nil
			}
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
		}
	}
	slog.Info("glamping_rf: сбор завершён (страницы исчерпаны)", "unique", len(out))
	return out, nil
}

// collectPlace листает страницы одного региона (place), добавляя новые объекты в
// out. Останавливается на конце выдачи, достижении minObjects, сбое страницы или
// предохранителе maxPagesPerPlace. Сбой страницы не фатален — просто выходим.
func (p *Provider) collectPlace(ctx context.Context, direction string, place int, seen map[int]bool, out *[]contract.Object) {
	for page := 1; page <= maxPagesPerPlace; page++ {
		resp, err := p.fetcher.fetchPage(ctx, place, page)
		if err != nil {
			slog.Warn("glamping_rf: страница пропущена",
				"direction", direction, "place", place, "page", page, "err", err)
			return
		}

		added := 0
		for _, it := range resp.Items {
			if it.ID == 0 || seen[it.ID] {
				continue
			}
			seen[it.ID] = true
			*out = append(*out, toObject(it))
			added++
		}
		slog.Info("glamping_rf: страница собрана",
			"direction", direction, "place", place, "page", page,
			"items", len(resp.Items), "new", added, "unique_total", len(*out))

		if !resp.HasMore || len(resp.Items) == 0 || len(*out) >= p.minObjects {
			return
		}
		if !providers.SleepCtx(ctx, p.delay) {
			return // ctx отменён — сворачиваем сбор
		}
	}
}
