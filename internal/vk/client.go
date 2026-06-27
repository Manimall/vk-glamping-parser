// Package vk — тонкий клиент к VK API (версия 5.131).
package vk

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	apiBaseURL = "https://api.vk.com/method"
	apiVersion = "5.131"
	// Сколько объектов тянем за один вызов (у VK максимум обычно 200/1000).
	defaultCount = "50"
)

// Client инкапсулирует токен и переиспользуемый HTTP-клиент.
// Создаётся один раз и шарится между вызовами — см. NewClient.
type Client struct {
	token      string
	httpClient *http.Client
}

// NewClient собирает клиент с разумным таймаутом.
func NewClient(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// --- Конверт ответа VK -------------------------------------------------------
// Любой ответ VK имеет вид {"response": ...} ЛИБО {"error": {...}}.
// Полезную нагрузку держим как json.RawMessage — «отложенный» кусок JSON,
// который домаршалим позже в конкретную структуру метода.

type apiError struct {
	Code int    `json:"error_code"`
	Msg  string `json:"error_msg"`
}

type apiEnvelope struct {
	Error    *apiError       `json:"error"`
	Response json.RawMessage `json:"response"`
}

// call — единственное место, где реально ходит HTTP. Все публичные методы
// строят параметры и делегируют сюда. dst — куда домаршалить "response".
func (c *Client) call(method string, params url.Values, dst any) error {
	params.Set("access_token", c.token)
	params.Set("v", apiVersion)

	endpoint := fmt.Sprintf("%s/%s?%s", apiBaseURL, method, params.Encode())

	resp, err := c.httpClient.Get(endpoint)
	if err != nil {
		return fmt.Errorf("vk: GET %s: %w", method, err)
	}
	// Закрыть тело ОБЯЗАТЕЛЬНО — иначе соединение не вернётся в пул и утечёт.
	defer resp.Body.Close()

	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return fmt.Errorf("vk: decode %s: %w", method, err)
	}

	if env.Error != nil {
		return fmt.Errorf("vk: api error %d on %s: %s",
			env.Error.Code, method, env.Error.Msg)
	}

	// VK отдаёт "[]" вместо объекта, когда ничего не найдено — тогда оставляем
	// dst нулевым значением, а не падаем на домаршалинге.
	if dst != nil && len(env.Response) > 0 && string(env.Response) != "[]" {
		if err := json.Unmarshal(env.Response, dst); err != nil {
			return fmt.Errorf("vk: unmarshal %s response: %w", method, err)
		}
	}
	return nil
}
