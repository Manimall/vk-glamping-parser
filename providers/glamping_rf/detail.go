package glamping_rf

// Обогащение объекта detail-страницей /glampings/<id>. Стратегия Smart Fetching:
// страница — HTML, но внутри лежат ГОТОВЫЕ структурированные данные, их и берём
// (парсинг DOM-вёрстки не нужен). Модули разбора:
//   - detail_ld.go   — <script type="application/ld+json">: LodgingBusiness
//     (описание, заезд/выезд, рейтинг), FAQPage (правила);
//   - detail_page.go — маркеры вёрстки: полное описание, площадь, вместимость,
//     фото галереи, точная точка карты (Placemark);
//   - detail_pv12.go — встроенный JSON window.pv12RoomDetails: платные услуги
//     (баня/чан/питомец) с ценами.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"vk-parser/internal/extract"
)

// errDetailGone — detail-страница отдала 404: объект СНЯТ с каталога-источника.
// Сигнал вызывающему исключить объект из выдачи (мёртвый источник ≠ сетевой
// глюк: таймауты/5xx этим НЕ считаются — по ним объект остаётся с дефолтами).
var errDetailGone = errors.New("glamping_rf: объект снят с каталога (404)")

// detailURL — страница объекта (без query-мусора из href списка).
const detailURL = "https://xn--c1aaobmgio8j.xn--p1ai/glampings/%d"

// detailData — то, что удалось достать со страницы. Все поля опциональны:
// чего нет — остаётся пустым, merge возьмёт данные списка.
type detailData struct {
	Description string
	CheckIn     string
	CheckOut    string
	Rating      string // «5.0»
	Reviews     int
	Photos      []string
	Amenities   []string        // категории amenityFeature
	Extras      []extract.Extra // платные услуги (баня/чан/питомец) с ценой
	Rules       []string        // правила из FAQ (очищенный текст)
	Guests      int             // вместимость: базовые + доп. места
	Area        string          // «80 м²»
	Lat         float64         // точная точка объекта из placemark карты
	Lng         float64
}

// tagRe счищает HTML-теги из текстов (описание, FAQ).
var tagRe = regexp.MustCompile(`<[^>]+>`)

// spacesRe — схлопывание пробельных последовательностей в один пробел.
var spacesRe = regexp.MustCompile(`\s+`)

// fetchDetail тянет и парсит страницу объекта. Ошибка — только на сетевом сбое;
// «не распарсилось» — не ошибка (вернётся частично пустой detailData).
func (c *Client) fetchDetail(ctx context.Context, id int) (*detailData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(detailURL, id), nil)
	if err != nil {
		return nil, fmt.Errorf("glamping_rf: build detail request id=%d: %w", id, err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("glamping_rf: detail id=%d: %w", id, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("id=%d: %w", id, errDetailGone)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("glamping_rf: detail id=%d status %d", id, resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("glamping_rf: read detail id=%d: %w", id, err)
	}
	return parseDetailHTML(string(raw), id), nil
}

// parseDetailHTML — чистый разбор HTML карточки (тестируется без сети).
func parseDetailHTML(page string, id int) *detailData {
	d := &detailData{}
	parseLdJSON(page, d)
	// Полное описание из вёрстки перекрывает обрезанный ld+json (см. descFullRe).
	if full := fullDescription(page); full != "" {
		d.Description = full
	}
	d.Photos = detailPhotos(page, id)

	if m := capacityRe.FindStringSubmatch(page); m != nil {
		base, _ := strconv.Atoi(m[1])
		extra, _ := strconv.Atoi(m[2]) // пустая группа → 0
		d.Guests = base + extra
	}
	d.Area = detailArea(page)
	d.Extras = detailPaidExtras(page)
	if lat, lng, ok := detailPlacemark(page); ok {
		d.Lat, d.Lng = lat, lng
	}
	return d
}
