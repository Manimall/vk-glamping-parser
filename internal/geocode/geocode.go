// Package geocode переводит текстовый адрес в координаты через бесплатный
// сервис Nominatim (OpenStreetMap). Используется как фоллбэк, когда координаты
// не заданы вручную.
package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	// nominatimBaseURL — публичный инстанс Nominatim. Политика сервиса требует
	// осмысленный User-Agent и не более ~1 запроса в секунду (нам хватает:
	// результат кэшируется выше по стеку).
	nominatimBaseURL = "https://nominatim.openstreetmap.org/search"
	// userAgent обязателен по правилам Nominatim — иначе банят по запросам.
	userAgent = "vk-glamping-parser/1.0 (iv-iframes aggregator)"
	// resultLimit — берём только лучший результат.
	resultLimit = "1"
	// httpTimeout — таймаут запроса к геокодеру.
	httpTimeout = 10 * time.Second
)

// Client оборачивает HTTP-клиент. Создаётся один раз и переиспользуется.
type Client struct {
	httpClient *http.Client
}

// New собирает клиент с таймаутом.
func New() *Client {
	return &Client{httpClient: &http.Client{Timeout: httpTimeout}}
}

// nominatimResult — нужные поля ответа Nominatim (lat/lon приходят СТРОКАМИ).
type nominatimResult struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// Geocode возвращает координаты (lat, lon) для адреса. Пробует несколько
// вариантов запроса (полный нормализованный, затем «населённый пункт + область»
// — Nominatim плохо ест русские сокращения и районы) и берёт первый сработавший.
// Ошибка — если ни один вариант не нашёлся; вызывающий решает, что делать.
func (c *Client) Geocode(ctx context.Context, address string) (lat, lon float64, err error) {
	for _, q := range candidates(address) {
		if lat, lon, err = c.geocodeOnce(ctx, q); err == nil {
			return lat, lon, nil
		}
	}
	return 0, 0, fmt.Errorf("geocode: nothing resolved for %q", address)
}

// geocodeOnce — один запрос к Nominatim.
func (c *Client) geocodeOnce(ctx context.Context, query string) (lat, lon float64, err error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "jsonv2")
	params.Set("limit", resultLimit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nominatimBaseURL+"?"+params.Encode(), nil)
	if err != nil {
		return 0, 0, fmt.Errorf("geocode: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent) // обязателен по правилам Nominatim

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("geocode: do request: %w", err)
	}
	defer resp.Body.Close()

	var results []nominatimResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return 0, 0, fmt.Errorf("geocode: decode response: %w", err)
	}
	if len(results) == 0 {
		return 0, 0, fmt.Errorf("geocode: %q not found", query)
	}

	lat, err1 := strconv.ParseFloat(results[0].Lat, 64)
	lon, err2 := strconv.ParseFloat(results[0].Lon, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("geocode: parse coords for %q", query)
	}
	return lat, lon, nil
}

// simpleAbbrev — ОДНОЗНАЧНЫЕ сокращения (раскрываем где угодно). Сюда НЕ входят
// «д.»/«с.»/«г.»: они перегружены («д.» = деревня ИЛИ дом), их раскрываем только
// посегментно в expandLocality, иначе номер дома «д. 5» превратился бы в «деревня 5».
var simpleAbbrev = strings.NewReplacer(
	"обл.", "область",
	"р-н", "район",
	"ул.", "улица",
	"пос.", "посёлок",
	"пгт.", "посёлок",
	"пер.", "переулок",
	"пр-т", "проспект",
)

// localityPrefixes — префиксы типа населённого пункта → полное слово. Полные
// формы идут ПЕРЕД сокращениями, чтобы «деревня» матчилось раньше «д.».
var localityPrefixes = []struct{ abbr, full string }{
	{"деревня", "деревня"}, {"дер.", "деревня"}, {"д.", "деревня"},
	{"село", "село"}, {"с.", "село"},
	{"посёлок", "посёлок"}, {"поселок", "посёлок"}, {"пос.", "посёлок"}, {"пгт", "посёлок"},
	{"город", "город"}, {"г.", "город"},
	{"хутор", "хутор"}, {"станица", "станица"},
}

// candidates строит список запросов от точного к грубому. Нормализация делается
// ПОСЕГМЕНТНО (по запятым): однозначные сокращения раскрываются везде, а тип НП
// («д.» и т.п.) — только в сегменте, который реально является населённым пунктом.
func candidates(address string) []string {
	parts := splitTrim(address)
	locality, region := "", ""
	norm := make([]string, len(parts))

	for i, p := range parts {
		expanded := simpleAbbrev.Replace(p)
		if loc := expandLocality(p); loc != "" {
			expanded = loc
			if locality == "" {
				locality = loc
			}
		}
		if region == "" && isRegion(expanded) {
			region = expanded
		}
		norm[i] = expanded
	}

	out := []string{strings.Join(norm, ", ")}
	// «населённый пункт + область» — самый надёжный вариант для деревень.
	if locality != "" && region != "" {
		out = append(out, locality+" "+region)
	}
	return out
}

// expandLocality раскрывает сегмент-населённый пункт («д. Крюково» → «деревня
// Крюково»). Возвращает "" если сегмент НЕ населённый пункт — в частности, если
// после префикса идёт число («д. 5» — это номер дома, а не деревня).
func expandLocality(seg string) string {
	low := strings.ToLower(seg)
	for _, lp := range localityPrefixes {
		if !strings.HasPrefix(low, lp.abbr) {
			continue
		}
		name := strings.TrimSpace(seg[len(lp.abbr):])
		if name == "" {
			continue
		}
		if r, _ := utf8.DecodeRuneInString(name); !unicode.IsLetter(r) {
			continue // после префикса число → это дом, а не НП
		}
		return lp.full + " " + name
	}
	return ""
}

// isRegion — сегмент похож на регион (область/край/республика).
func isRegion(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "область") ||
		strings.Contains(low, "край") ||
		strings.Contains(low, "республик")
}

// splitTrim делит адрес по запятым и обрезает пробелы, выбрасывая пустые куски.
func splitTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
