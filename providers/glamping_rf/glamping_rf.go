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
	"errors"
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

// collected — объект + его id в источнике (нужен для detail-страницы обогащения).
type collected struct {
	obj contract.Object
	id  int
}

// Parse: фаза 1 — обход направлений/регионов, набор уникальных (по id) объектов
// до minObjects; фаза 2 — обогащение каждого detail-страницей (описание, полная
// галерея, заезд/выезд, правила из FAQ) + дефолты. Сбой обогащения одного
// объекта не фатален — он остаётся с данными списка и дефолтами.
func (p *Provider) Parse(ctx context.Context) ([]contract.Object, error) {
	// [Go для изучения] map[int]bool — идиома «множество» (Set из JS): ключ есть →
	// объект уже видели. make(slice, 0, cap) предвыделяет ёмкость: append не будет
	// перевыделять память, пока не наберём minObjects элементов.
	seen := make(map[int]bool)
	items := make([]collected, 0, p.minObjects)

collect:
	for _, dir := range p.directions {
		for _, place := range dir.places {
			p.collectPlace(ctx, dir.name, place, seen, &items)
			if len(items) >= p.minObjects {
				slog.Info("glamping_rf: собрано достаточно",
					"unique", len(items), "min", p.minObjects)
				break collect
			}
			if ctx.Err() != nil {
				// Отмена во время сбора: обогащать поздно — отдаём как есть.
				return rawObjects(items), ctx.Err()
			}
		}
	}

	kept := p.enrichAll(ctx, items)
	slog.Info("glamping_rf: сбор завершён",
		"unique", len(kept), "снято_с_каталога", len(items)-len(kept))
	return kept, ctx.Err()
}

// enrichAll — фаза обогащения: detail-страница на объект, с паузой между
// запросами (анти-бан). Объекты, СНЯТЫЕ с каталога (detail → 404), в выдачу
// не попадают — мёртвый источник не показываем (продуктовое решение). Прочие
// сбои (таймаут/5xx) объект не выкидывают: остаётся с данными списка + дефолты.
func (p *Provider) enrichAll(ctx context.Context, items []collected) []contract.Object {
	out := make([]contract.Object, 0, len(items))
	for i := range items {
		if ctx.Err() != nil {
			return out
		}
		if p.enrichOne(ctx, &items[i]) {
			out = append(out, items[i].obj)
		}
		if i%10 == 9 {
			slog.Info("glamping_rf: обогащение", "готово", i+1, "из", len(items))
		}
		if i < len(items)-1 && !providers.SleepCtx(ctx, p.delay) {
			return out
		}
	}
	return out
}

// rawObjects — собранное без обогащения (путь досрочной отмены ctx).
func rawObjects(items []collected) []contract.Object {
	out := make([]contract.Object, len(items))
	for i, it := range items {
		out[i] = it.obj
	}
	return out
}

// enrichOne — обогащение одного объекта. false → объект снят с каталога
// (исключить из выдачи); true → оставить (обогащён либо с дефолтами).
func (p *Provider) enrichOne(ctx context.Context, it *collected) bool {
	d, err := p.fetcher.fetchDetail(ctx, it.id)
	switch {
	case err == nil:
		mergeDetail(&it.obj, d)
	case errors.Is(err, errDetailGone):
		slog.Info("glamping_rf: объект исключён (снят с каталога)",
			"id", it.id, "title", it.obj.Title)
		return false
	default:
		slog.Warn("glamping_rf: detail пропущен (объект оставлен)", "id", it.id, "err", err)
	}
	applyDefaults(&it.obj)
	return true
}

// collectPlace листает страницы одного региона (place), добавляя новые объекты в
// out. Останавливается на конце выдачи, достижении minObjects, сбое страницы или
// предохранителе maxPagesPerPlace. Сбой страницы не фатален — просто выходим.
//
// [Go для изучения] out *[]collected — указатель на слайс: append может
// перевыделить внутренний массив, и без указателя вызывающий не увидел бы
// добавленного (слайс передаётся по значению — копируется заголовок). map такого
// не требует: seen — ссылочный тип, правки видны снаружи и так.
func (p *Provider) collectPlace(ctx context.Context, direction string, place int, seen map[int]bool, out *[]collected) {
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
			*out = append(*out, collected{obj: toObject(it), id: it.ID})
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
