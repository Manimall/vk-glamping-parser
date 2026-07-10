package glamping_rf

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	// apiURL — точка внутреннего JSON-API каталога (OpenCart).
	apiURL = "https://xn--c1aaobmgio8j.xn--p1ai/index.php"
	// categoryPath — id категории «глэмпинги» в OpenCart (общая для всех регионов;
	// регион задаётся отдельным фильтром place).
	categoryPath = "82"
	// ajaxHeader/ajaxValue — без них сервер отдаёт HTML-страницу, а не JSON.
	ajaxHeader  = "X-Requested-With"
	ajaxValue   = "XMLHttpRequest"
	userAgent   = "Mozilla/5.0 (compatible; iv-iframes-bot/1.0; +https://iv-iframes.vercel.app)"
	httpTimeout = 20 * time.Second
)

// pageFetcher — то, что провайдеру нужно от источника: получить одну страницу
// выдачи для региона. Интерфейс (а не конкретный Client) — чтобы Parse можно было
// тестировать на фейке без сети (Dependency Inversion).
type pageFetcher interface {
	fetchPage(ctx context.Context, place, page int) (*apiResponse, error)
}

// Client — HTTP-клиент к JSON-API глэмпинги.рф. Переиспользуемый (один на провайдер).
type Client struct {
	hc *http.Client
}

// newClient собирает клиент с разумным таймаутом.
func newClient() *Client {
	return &Client{hc: &http.Client{Timeout: httpTimeout}}
}

// fetchPage тянет одну страницу каталога для фильтра place. Ошибка — только на
// сетевом/HTTP/JSON сбое; вызывающий решает, прерывать ли сбор.
func (c *Client) fetchPage(ctx context.Context, place, page int) (*apiResponse, error) {
	q := url.Values{}
	q.Set("route", "product/category/list")
	q.Set("path", categoryPath)
	q.Set("place", strconv.Itoa(place))
	q.Set("page", strconv.Itoa(page))
	endpoint := apiURL + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("glamping_rf: build request: %w", err)
	}
	req.Header.Set(ajaxHeader, ajaxValue)
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("glamping_rf: do request place=%d page=%d: %w", place, page, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("glamping_rf: status %d place=%d page=%d", resp.StatusCode, place, page)
	}

	var out apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("glamping_rf: decode place=%d page=%d: %w", place, page, err)
	}
	return &out, nil
}
