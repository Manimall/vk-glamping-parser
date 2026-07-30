package glamping_rf

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// retryTestClient — клиент с коротким таймаутом и без пауз между попытками:
// проверяем логику повторов, а не терпение.
func retryTestClient(timeout time.Duration) *Client {
	return &Client{hc: &http.Client{Timeout: timeout}}
}

// Главный сценарий, ради которого повторы и написаны: источник отвечает
// медленнее нашего таймаута.
//
// Первая версия его НЕ повторяла. При срабатывании http.Client.Timeout Go
// возвращает ошибку, для которой errors.Is(err, context.DeadlineExceeded)
// истинна — а по ней стояла ветка «отмена контекста, повторять нечего». То
// есть код повторов существовал, а повторов не было: прогон по-прежнему
// заканчивался пустым регионом.
func TestDoWithRetry_RetriesSlowSource(t *testing.T) {
	// Паузы между попытками не выдерживаем: проверяем, что повтор ПРОИСХОДИТ, а
	// не сколько мы готовы ждать. Без этого тест занимал девять секунд — ровно
	// те, ради экономии которых retryBaseDelay и сделана переменной.
	saved := retryBaseDelay
	retryBaseDelay = time.Millisecond
	defer func() { retryBaseDelay = saved }()

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			time.Sleep(300 * time.Millisecond) // медленнее таймаута клиента
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := retryTestClient(80 * time.Millisecond)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)

	resp, err := c.doWithRetry(req, "тест")
	if err != nil {
		t.Fatalf("после повторов ожидали успех, got %v (попыток: %d)", err, atomic.LoadInt32(&hits))
	}
	defer resp.Body.Close()

	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("попыток: %d, ожидали 3 — медленный ответ обязан повторяться", got)
	}
}

// Отмена гостем или завершение прогона — не сбой источника: повторять нечего,
// выходим сразу.
func TestDoWithRetry_StopsOnContextCancel(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	c := retryTestClient(5 * time.Second)
	if _, err := c.doWithRetry(req, "тест"); err == nil {
		t.Fatal("ожидали ошибку отмены")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("попыток: %d, ожидали 1 — отменённый запрос повторять нечего", got)
	}
}

// Серверные сбои повторяются, ответы — нет.
func TestDoWithRetry_StatusHandling(t *testing.T) {
	// Паузы не ждём: проверяем число попыток, а не терпение.
	saved := retryBaseDelay
	retryBaseDelay = time.Millisecond
	defer func() { retryBaseDelay = saved }()

	cases := []struct {
		name     string
		status   int
		attempts int32
	}{
		{"503 повторяется", http.StatusServiceUnavailable, maxAttempts},
		{"429 повторяется", http.StatusTooManyRequests, maxAttempts},
		{"404 — ответ, а не сбой", http.StatusNotFound, 1},
		{"200 сразу", http.StatusOK, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				w.WriteHeader(c.status)
			}))
			defer srv.Close()

			req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
			resp, _ := retryTestClient(2*time.Second).doWithRetry(req, "тест")
			if resp != nil {
				resp.Body.Close()
			}
			if got := atomic.LoadInt32(&hits); got != c.attempts {
				t.Errorf("попыток: %d, ожидали %d", got, c.attempts)
			}
		})
	}
}
