// Package vision — опциональный выбор кадра-обложки через ЛОКАЛЬНУЮ vision-модель
// (Ollama, напр. moondream). Бесплатно, без подписок: парсер ходит по HTTP в
// локальный Ollama. Если он недоступен — вызывающий откатывается на эвристику.
//
// Почему describe + скоринг по словам, а не «ответь yes/no»: маленькие vision-
// модели (moondream) на строгих форматах часто отдают пустой ответ, зато
// стабильно описывают кадр одним предложением. Наличие «cabin/house/porch» в
// описании — надёжный сигнал «виден фасад домика».
package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	defaultURL   = "http://localhost:11434"
	defaultModel = "moondream"
	// describePrompt — открытый промпт (moondream надёжно отвечает описанием).
	describePrompt = "Describe this image in one short sentence."
	// maxCandidates — сколько кадров максимум прогоняем через модель (бюджет
	// времени: ~1–3 c на кадр). Кандидатов пред-фильтрует эвристика.
	maxCandidates = 8
)

// positiveKW — слова «виден фасад/домик снаружи» (плюс к скору).
var positiveKW = []string{
	"cabin", "house", "cottage", "chalet", "a-frame", "aframe",
	"lodge", "hut", "building", "porch", "deck", "facade",
}

// negativeKW — люди / интерьер / еда / текст (минус: не годится в обложку).
var negativeKW = []string{
	"person", "people", "man", "woman", "girl", "boy", "child",
	"someone", "selfie", "posing",
	"room", "bedroom", "kitchen", "sofa", "couch", "bed ", "interior", "indoor",
	"plate", "food", "meal", "table setting", "text", "sign",
}

// Client — тонкий HTTP-клиент к Ollama.
type Client struct {
	url   string
	model string
	hc    *http.Client
}

// New создаёт клиент. Пустые url/model → значения по умолчанию.
func New(url, model string) *Client {
	if url == "" {
		url = defaultURL
	}
	if model == "" {
		model = defaultModel
	}
	return &Client{url: url, model: model, hc: &http.Client{Timeout: 90 * time.Second}}
}

// Available — быстрый чек, что Ollama жив и нужная модель установлена.
func (c *Client) Available(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url+"/api/tags", nil)
	if err != nil {
		return false
	}
	quick := &http.Client{Timeout: 3 * time.Second}
	resp, err := quick.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if json.NewDecoder(resp.Body).Decode(&out) != nil {
		return false
	}
	for _, m := range out.Models {
		if strings.HasPrefix(m.Name, c.model) {
			return true
		}
	}
	return false
}

// PickCover возвращает индекс лучшего кадра-обложки (виден домик, без людей/
// интерьера) по описаниям модели. (0,false) — если модель недоступна/упала:
// вызывающий откатывается на эвристику. Обрабатывает не более maxCandidates.
func (c *Client) PickCover(ctx context.Context, candidates [][]byte) (int, bool) {
	n := len(candidates)
	if n == 0 {
		return 0, false
	}
	if n > maxCandidates {
		n = maxCandidates
	}
	best, bestScore := -1, 0
	for i := 0; i < n; i++ {
		desc, err := c.describe(ctx, candidates[i])
		if err != nil {
			return 0, false // Ollama недоступен — общий фолбэк
		}
		if s := coverTextScore(desc); best < 0 || s > bestScore {
			best, bestScore = i, s
		}
	}
	if best < 0 {
		return 0, false
	}
	return best, true
}

// describe просит модель описать кадр одним предложением.
func (c *Client) describe(ctx context.Context, img []byte) (string, error) {
	payload, _ := json.Marshal(map[string]any{
		"model":  c.model,
		"prompt": describePrompt,
		"images": []string{base64.StdEncoding.EncodeToString(img)},
		"stream": false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/api/generate", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vision: ollama status %d", resp.StatusCode)
	}
	var out struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Response, nil
}

// coverTextScore оценивает описание кадра как обложки: +за домик снаружи, −за
// людей/интерьер/еду/текст. Логика вынесена из клиента — тестируется без сети.
func coverTextScore(desc string) int {
	d := strings.ToLower(desc)
	score := 0
	for _, kw := range positiveKW {
		if strings.Contains(d, kw) {
			score += 2
		}
	}
	for _, kw := range negativeKW {
		if strings.Contains(d, kw) {
			score -= 3
		}
	}
	return score
}
