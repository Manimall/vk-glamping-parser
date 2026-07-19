package glamping_rf

// Разбор <script type="application/ld+json"> detail-страницы (schema.org):
// LodgingBusiness — описание, заезд/выезд, рейтинг, категории удобств;
// FAQPage — реальные правила проживания (отмена брони, питомцы, тишина…).

import (
	"encoding/json"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// maxRuleRunes — правило из FAQ обрезаем по границе предложения до этого предела.
const maxRuleRunes = 200

// ldJSONRe вырезает содержимое <script type="application/ld+json">.
var ldJSONRe = regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)

// ruleKeywordsRe — вопросы FAQ, которые являются ПРАВИЛАМИ проживания.
var ruleKeywordsRe = regexp.MustCompile(`(?i)отмен|заезд|выезд|животн|питомц|курен|тишин|правил`)

// rawControlCharsRe — сайт кладёт в значения JSON-строк БУКВАЛЬНЫЕ переносы
// строк/табы вместо \n/\t (невалидно по JSON-спеке: control-символы обязаны
// быть экранированы). encoding/json Go строг и падает на этом с "invalid
// control character". Вне строк эти же символы — обычные пробелы-разделители
// между токенами, так что безопасно заменить их везде на пробел до парсинга.
var rawControlCharsRe = regexp.MustCompile(`[\x00-\x1F]`)

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

// parseLdJSON разбирает оба ld+json блока (LodgingBusiness и FAQPage).
func parseLdJSON(page string, d *detailData) {
	for _, raw := range ldJSONRe.FindAllStringSubmatch(page, -1) {
		m1 := rawControlCharsRe.ReplaceAllString(raw[1], " ")
		var probe struct {
			Type string `json:"@type"`
		}
		if json.Unmarshal([]byte(m1), &probe) != nil {
			continue
		}
		switch probe.Type {
		case "LodgingBusiness", "Campground", "Hotel":
			var lb ldLodging
			if json.Unmarshal([]byte(m1), &lb) == nil {
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
			if json.Unmarshal([]byte(m1), &faq) == nil {
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

// formatRating нормализует «5.0000» → «5.0» (пусто, если не число).
func formatRating(v string) string {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', 1, 64)
}
