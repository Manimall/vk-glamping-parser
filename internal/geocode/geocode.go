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
)

// nominatimBaseURL — публичный инстанс Nominatim. Политика сервиса требует
// осмысленный User-Agent и не более ~1 запроса в секунду (нам хватает: результат
// кэшируется выше по стеку).
const nominatimBaseURL = "https://nominatim.openstreetmap.org/search"

// userAgent обязателен по правилам Nominatim — иначе банят по запросам.
const userAgent = "vk-glamping-parser/1.0 (iv-iframes aggregator)"

// Coords — результат геокодирования.
type Coords struct {
	Lat float64
	Lon float64
}

// Client оборачивает HTTP-клиент. Создаётся один раз и переиспользуется.
type Client struct {
	httpClient *http.Client
}

// New собирает клиент с таймаутом.
func New() *Client {
	return &Client{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

// nominatimResult — нужные поля ответа Nominatim (lat/lon приходят СТРОКАМИ).
type nominatimResult struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// Geocode возвращает координаты для адреса. Пробует несколько вариантов запроса
// (полный нормализованный, затем «населённый пункт + область» — Nominatim плохо
// ест русские сокращения и районы) и берёт первый сработавший. Ошибка — если ни
// один вариант не нашёлся; вызывающий решает, что делать.
func (c *Client) Geocode(ctx context.Context, address string) (*Coords, error) {
	for _, q := range candidates(address) {
		if coords, err := c.geocodeOnce(ctx, q); err == nil {
			return coords, nil
		}
	}
	return nil, fmt.Errorf("geocode: nothing resolved for %q", address)
}

// geocodeOnce — один запрос к Nominatim.
func (c *Client) geocodeOnce(ctx context.Context, query string) (*Coords, error) {
	params := url.Values{}
	params.Set("q", query)
	params.Set("format", "jsonv2")
	params.Set("limit", "1")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, nominatimBaseURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("geocode: build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent) // обязателен по правилам Nominatim

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("geocode: do request: %w", err)
	}
	defer resp.Body.Close()

	var results []nominatimResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, fmt.Errorf("geocode: decode response: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("geocode: %q not found", query)
	}

	lat, err1 := strconv.ParseFloat(results[0].Lat, 64)
	lon, err2 := strconv.ParseFloat(results[0].Lon, 64)
	if err1 != nil || err2 != nil {
		return nil, fmt.Errorf("geocode: parse coords for %q", query)
	}
	return &Coords{Lat: lat, Lon: lon}, nil
}

// abbreviations — частые русские сокращения в адресах, которые Nominatim не
// понимает. Раскрываем их в полные слова.
var abbreviations = strings.NewReplacer(
	"обл.", "область",
	"р-н", "район",
	"пос.", "посёлок",
	"пгт.", "посёлок",
	"д.", "деревня",
	"с.", "село",
	"г.", "город",
	"ул.", "улица",
	"пер.", "переулок",
	"пр-т", "проспект",
)

// candidates строит список запросов от точного к грубому. Первый — нормализованный
// полный адрес; второй — «населённый пункт + область» (без района/улицы/дома),
// который для деревень резолвится надёжнее всего.
func candidates(address string) []string {
	norm := abbreviations.Replace(address)
	out := []string{norm}

	parts := strings.Split(norm, ",")
	var locality, region string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		low := strings.ToLower(p)
		switch {
		case locality == "" && hasAnyPrefixWord(low, "деревня", "село", "посёлок", "поселок", "город", "хутор", "станица"):
			locality = p
		case region == "" && (strings.Contains(low, "область") || strings.Contains(low, "край") || strings.Contains(low, "республик")):
			region = p
		}
	}
	if locality != "" && region != "" {
		out = append(out, locality+" "+region)
	}
	return out
}

// hasAnyPrefixWord проверяет, начинается ли строка с одного из слов (тип НП).
func hasAnyPrefixWord(s string, words ...string) bool {
	for _, w := range words {
		if strings.HasPrefix(s, w) {
			return true
		}
	}
	return false
}
