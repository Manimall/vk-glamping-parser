package glamping_rf

// Обогащение объекта detail-страницей /glampings/<id>. Стратегия Smart Fetching:
// страница — HTML, но внутри лежат ГОТОВЫЕ структурированные данные, их и берём
// (парсинг DOM-вёрстки не нужен):
//   - <script type="application/ld+json"> LodgingBusiness — описание, заезд/выезд,
//     адрес, рейтинг; FAQPage — реальные правила (отмена брони, питомцы…);
//   - URL фото галереи /image/cachewebp/catalog/<id>/… — до maxDetailPhotos кадров;
//   - «Вместимость: N + M гостя» / «Площадь: NN м²» — текстовые маркеры вариантов.

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const (
	// detailURL — страница объекта (без query-мусора из href списка).
	detailURL = "https://xn--c1aaobmgio8j.xn--p1ai/glampings/%d"
	// maxDetailPhotos — предел галереи (как у курируемых объектов фронта).
	maxDetailPhotos = 15
	// maxRuleRunes — правило из FAQ обрезаем по границе предложения до этого предела.
	maxRuleRunes = 200
)

// detailData — то, что удалось достать со страницы. Все поля опциональны:
// чего нет — остаётся пустым, merge возьмёт данные списка.
type detailData struct {
	Description string
	CheckIn     string
	CheckOut    string
	Rating      string // «5.0»
	Reviews     int
	Photos      []string
	Amenities   []string // категории amenityFeature
	Rules       []string // правила из FAQ (очищенный текст)
	Guests      int      // вместимость: базовые + доп. места
	Area        string   // «80 м²»
}

// ldJSONRe вырезает содержимое <script type="application/ld+json">.
var ldJSONRe = regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)

// tagRe счищает HTML-теги из текстов FAQ.
var tagRe = regexp.MustCompile(`<[^>]+>`)

// capacityRe — «Вместимость: 4 + 2 гостя» (доп. места опциональны).
var capacityRe = regexp.MustCompile(`Вместимость:\s*(\d+)(?:\s*\+\s*(\d+))?\s*гост`)

// areaRe — «Площадь: 80 м²» (в тексте бывает узкий пробел  ).
var areaRe = regexp.MustCompile(`Площадь:\s*([\d.,]+)\s*м²`)

// ruleKeywordsRe — вопросы FAQ, которые являются ПРАВИЛАМИ проживания.
var ruleKeywordsRe = regexp.MustCompile(`(?i)отмен|заезд|выезд|животн|питомц|курен|тишин|правил`)

// ldLodging — нужные поля LodgingBusiness (schema.org).
type ldLodging struct {
	Type           string `json:"@type"`
	Description    string `json:"description"`
	CheckinTime    string `json:"checkinTime"`
	CheckoutTime   string `json:"checkoutTime"`
	AmenityFeature []struct {
		Name string `json:"name"`
	} `json:"amenityFeature"`
	AggregateRating struct {
		RatingValue string `json:"ratingValue"`
		ReviewCount string `json:"reviewCount"`
	} `json:"aggregateRating"`
}

// ldFAQ — нужные поля FAQPage (schema.org).
type ldFAQ struct {
	Type       string `json:"@type"`
	MainEntity []struct {
		Name           string `json:"name"`
		AcceptedAnswer struct {
			Text string `json:"text"`
		} `json:"acceptedAnswer"`
	} `json:"mainEntity"`
}

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
	d.Photos = detailPhotos(page, id)

	if m := capacityRe.FindStringSubmatch(page); m != nil {
		base, _ := strconv.Atoi(m[1])
		extra, _ := strconv.Atoi(m[2]) // пустая группа → 0
		d.Guests = base + extra
	}
	if m := areaRe.FindStringSubmatch(page); m != nil {
		d.Area = m[1] + " м²"
	}
	return d
}

// parseLdJSON разбирает оба ld+json блока (LodgingBusiness и FAQPage).
func parseLdJSON(page string, d *detailData) {
	for _, m := range ldJSONRe.FindAllStringSubmatch(page, -1) {
		var probe struct {
			Type string `json:"@type"`
		}
		if json.Unmarshal([]byte(m[1]), &probe) != nil {
			continue
		}
		switch probe.Type {
		case "LodgingBusiness", "Campground", "Hotel":
			var lb ldLodging
			if json.Unmarshal([]byte(m[1]), &lb) == nil {
				d.Description = strings.TrimSpace(lb.Description)
				d.CheckIn, d.CheckOut = lb.CheckinTime, lb.CheckoutTime
				d.Rating = formatRating(lb.AggregateRating.RatingValue)
				d.Reviews, _ = strconv.Atoi(lb.AggregateRating.ReviewCount)
				for _, a := range lb.AmenityFeature {
					if n := strings.TrimSpace(a.Name); n != "" {
						d.Amenities = append(d.Amenities, n)
					}
				}
			}
		case "FAQPage":
			var faq ldFAQ
			if json.Unmarshal([]byte(m[1]), &faq) == nil {
				d.Rules = faqRules(faq)
			}
		}
	}
}

// faqRules отбирает из FAQ вопросы-«правила» и превращает ответы в короткие
// тексты (без HTML, обрезка по границе предложения).
func faqRules(faq ldFAQ) []string {
	var rules []string
	for _, q := range faq.MainEntity {
		if !ruleKeywordsRe.MatchString(q.Name) {
			continue
		}
		if text := cleanRule(q.AcceptedAnswer.Text); text != "" {
			rules = append(rules, text)
		}
	}
	return rules
}

// cleanRule чистит ответ FAQ (теги, entities, пробелы) и режет по предложению.
func cleanRule(s string) string {
	s = html.UnescapeString(tagRe.ReplaceAllString(s, " "))
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= maxRuleRunes {
		return s
	}
	cut := string(r[:maxRuleRunes])
	if i := strings.LastIndexAny(cut, ".!?"); i > 0 {
		return cut[:i+1]
	}
	return cut + "…"
}

// detailPhotos — уникальные webp-кадры галереи объекта в порядке появления.
func detailPhotos(page string, id int) []string {
	re := regexp.MustCompile(`https://[^\s"')]+/image/cachewebp/catalog/` + strconv.Itoa(id) + `/[^\s"')]+\.webp`)
	seen := make(map[string]bool)
	var out []string
	for _, u := range re.FindAllString(page, -1) {
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
		if len(out) == maxDetailPhotos {
			break
		}
	}
	return out
}

// formatRating нормализует «5.0000» → «5.0» (пусто, если не число).
func formatRating(v string) string {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', 1, 64)
}
