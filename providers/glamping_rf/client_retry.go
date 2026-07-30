package glamping_rf

// Повтор запроса при временном сбое источника.
//
// Сайт-источник периодически отвечает по 15 секунд вместо одной, и тогда даже
// TLS-рукопожатие не укладывается в таймаут. Без повторов один такой момент
// стоит целого региона: collectPlace получает ошибку и молча прекращает обход,
// а прогон завершается словами «объектов 0» — без единой строки ERROR.
//
// Повторяем только сетевые и серверные сбои. Ответ 404 (объект снят с
// каталога) и битый JSON повторять бессмысленно: со второго раза они не
// починятся.

import (
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	// maxAttempts — сколько раз пробуем один запрос. Три попытки с растущей
	// паузой перекрывают типичный «источник задумался», а на настоящем падении
	// сдаются за приемлемое время.
	maxAttempts = 3
)

// retryBaseDelay — пауза перед вторым заходом; перед третьим она удваивается.
// Дальше наращивать нет смысла: если сайт лежит, он лежит.
//
// Переменная, а не константа, только ради тестов: они проверяют ЧИСЛО попыток,
// и ждать девять секунд на каждый случай незачем.
var retryBaseDelay = 3 * time.Second

// doWithRetry выполняет запрос, повторяя его при временных сбоях.
//
// Тело ответа НЕ читает — это забота вызывающего, он же его и закрывает.
func (c *Client) doWithRetry(req *http.Request, what string) (*http.Response, error) {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			delay := retryBaseDelay * time.Duration(attempt-1)
			slog.Warn("glamping_rf: повтор запроса",
				"что", what, "попытка", attempt, "пауза", delay, "прошлая_ошибка", lastErr)
			select {
			case <-time.After(delay):
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}
		}

		resp, err := c.hc.Do(req)
		if err == nil && !isRetryableStatus(resp.StatusCode) {
			return resp, nil
		}

		if err != nil {
			// Отмена решается ПО КОНТЕКСТУ, а не по типу ошибки.
			//
			// Проверять errors.Is(err, context.DeadlineExceeded) здесь нельзя:
			// при срабатывании http.Client.Timeout Go возвращает ошибку, для
			// которой эта проверка тоже истинна. То есть повтор отключался
			// ровно в том случае, ради которого написан, — «источник отвечает
			// по 15 секунд». Прогон снова заканчивался пустым регионом, только
			// теперь рядом лежал код, который выглядел как решение.
			if req.Context().Err() != nil {
				return nil, req.Context().Err()
			}
			lastErr = err
		} else {
			lastErr = errStatus(resp.StatusCode)
			// Тело неудачной попытки дочитываем и закрываем сами. Закрыть мало:
			// без вычитки соединение не возвращается в пул, и каждая повторная
			// попытка платит новым TCP-рукопожатием — как раз тогда, когда
			// источник и так отвечает еле-еле.
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
	}
	return nil, lastErr
}

// isRetryableStatus — код, при котором повтор имеет смысл: сервер перегружен
// или временно недоступен. 404 сюда не входит — объект снят с каталога, и это
// ответ, а не сбой.
func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

type statusError int

func (e statusError) Error() string {
	return "glamping_rf: источник ответил " + http.StatusText(int(e))
}

func errStatus(code int) error { return statusError(code) }
