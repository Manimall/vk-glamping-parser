// Package extract превращает «сырьё» из VK (текст описания + инфо группы) в
// структуру, как на странице Сабадури: факты, группы удобств, доп.услуги,
// правила. Делает это одним вызовом Claude со строгой JSON-схемой (strict
// tool-use) — модель ОБЯЗАНА вернуть валидный JSON ровно нужной формы.
package extract

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// --- Целевая структура (контракт для фронтенда) ------------------------------
// Эти типы повторяют форму mockData в iv-iframes. Поля экспортируемые и с
// json-тегами — иначе encoding/json их не увидит и фронт получит пустоту.

// Fact — короткий «ключ-значение» для блока фактов (площадь, вместимость…).
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// AmenityGroup — удобства, сгруппированные по теме («В домике», «На территории»).
type AmenityGroup struct {
	Title string   `json:"title"`
	Items []string `json:"items"`
}

// Extra — платная доп.услуга. Price — строкой (может быть пустой, если не указана).
type Extra struct {
	Name  string `json:"name"`
	Price string `json:"price"`
}

// Property — итоговая карточка объекта в форме «как в Сабадури».
type Property struct {
	Title         string         `json:"title"`
	Summary       string         `json:"summary"`
	Location      string         `json:"location"`
	PriceFrom     string         `json:"priceFrom"`
	Facts         []Fact         `json:"facts"`
	AmenityGroups []AmenityGroup `json:"amenityGroups"`
	Extras        []Extra        `json:"extras"`
	Rules         []string       `json:"rules"`
}

// --- Вход (сырьё) ------------------------------------------------------------

// Listing — «сырые» поля, которые мы уже достали из VK. Это вход для извлечения.
// Пакет extract сам себе хозяин: не импортирует main, поэтому входной тип
// объявлен здесь, а main маппит свою GlampingData в этот Listing.
type Listing struct {
	Title       string
	Description string
	About       string
	Location    string
	Price       string
	PhotoCount  int
}

// Extractor — абстракция «превратить сырьё в Property». Главная идея (Dependency
// Inversion): main зависит от ЭТОГО интерфейса, а не от конкретной реализации.
// Поэтому движков может быть несколько (бесплатная эвристика / платный LLM), а
// переключение между ними — это просто другой объект, реализующий интерфейс.
type Extractor interface {
	Extract(ctx context.Context, in Listing) (*Property, error)
}

// --- Клиент ------------------------------------------------------------------

// модель извлечения. Opus 4.8 — самый способный; для удешевления можно
// заменить на anthropic.ModelClaudeSonnet4_6 (это решение пользователя, не наше).
const model = anthropic.ModelClaudeOpus4_8

// toolName — имя «инструмента»-схемы. Мы не выполняем никакого кода: tool-use
// здесь — лишь способ заставить модель вернуть строго типизированный JSON.
const toolName = "save_property"

// LLMClient — реализация Extractor через Claude. Платная (нужен ключ), но самая
// «умная». Создаётся один раз (как и vk.Client) и переиспользуется.
type LLMClient struct {
	api anthropic.Client
}

// NewLLM собирает LLM-извлекатель с явным API-ключом.
func NewLLM(apiKey string) *LLMClient {
	return &LLMClient{
		api: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}
}

// propertySchema — JSON-схема нашего Property для strict tool-use.
// strict требует, чтобы КАЖДОЕ свойство было в required и additionalProperties
// был false — тогда модель не сможет ни выдумать лишних полей, ни пропустить
// нужные (неизвестное заполнит пустым "" или []).
func propertySchema() anthropic.ToolInputSchemaParam {
	str := map[string]any{"type": "string"}
	strArray := map[string]any{"type": "array", "items": map[string]any{"type": "string"}}

	// Вспомогательная фабрика «объект со строго заданными полями».
	object := func(props map[string]any, required ...string) map[string]any {
		return map[string]any{
			"type":                 "object",
			"properties":           props,
			"required":             required,
			"additionalProperties": false,
		}
	}

	return anthropic.ToolInputSchemaParam{
		Properties: map[string]any{
			"title":     str,
			"summary":   str,
			"location":  str,
			"priceFrom": str,
			"facts": map[string]any{
				"type":  "array",
				"items": object(map[string]any{"label": str, "value": str}, "label", "value"),
			},
			"amenityGroups": map[string]any{
				"type":  "array",
				"items": object(map[string]any{"title": str, "items": strArray}, "title", "items"),
			},
			"extras": map[string]any{
				"type":  "array",
				"items": object(map[string]any{"name": str, "price": str}, "name", "price"),
			},
			"rules": strArray,
		},
		Required: []string{
			"title", "summary", "location", "priceFrom",
			"facts", "amenityGroups", "extras", "rules",
		},
		// additionalProperties:false для верхнего объекта кладём в ExtraFields:
		// у ToolInputSchemaParam нет отдельного поля под него.
		ExtraFields: map[string]any{
			"additionalProperties": false,
		},
	}
}

const systemPrompt = `Ты — ассистент, который структурирует описания глэмпингов и баз отдыха для карточки на сайте.
Тебе дают «сырьё»: название, описание товара из VK, описание сообщества, локацию, цену.
Извлеки и аккуратно сгруппируй информацию. Пиши по-русски, кратко и по делу.

Правила:
- summary — 1–2 предложения, суть места.
- facts — ключевые факты парами (например {"label":"Вместимость","value":"до 4 чел."}). Только то, что реально есть в тексте.
- amenityGroups — удобства по темам ("В домике", "На территории", "Развлечения"). Не выдумывай.
- extras — платные доп.услуги (баня, чан, мангал). price оставь пустым "", если цена не указана.
- rules — правила проживания, если упомянуты.
Если данных для поля нет — верни пустую строку "" или пустой массив. Ничего не придумывай сверх текста.`

// Extract отправляет сырьё в Claude и возвращает структурированную карточку.
func (c *LLMClient) Extract(ctx context.Context, in Listing) (*Property, error) {
	// Собираем «сырьё» в один текстовый блок для модели.
	userText := fmt.Sprintf(
		"Название: %s\nЛокация: %s\nЦена: %s\nКоличество фото: %d\n\nОписание товара:\n%s\n\nОписание сообщества:\n%s",
		in.Title, in.Location, in.Price, in.PhotoCount, in.Description, in.About,
	)

	tool := anthropic.ToolParam{
		Name:        toolName,
		Description: anthropic.String("Сохранить структурированную карточку объекта."),
		InputSchema: propertySchema(),
		Strict:      anthropic.Bool(true), // гарантия: input строго по схеме
	}

	resp, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: 4096,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &tool}},
		// Форсим вызов нашего инструмента: модель ОБЯЗАНА вернуть JSON по схеме,
		// а не свободный текст. (Thinking не включаем — с форс-tool он несовместим,
		// да и для извлечения не нужен.)
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: toolName},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userText)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("extract: call claude: %w", err)
	}

	// Ищем блок tool_use и домаршаливаем его input в Property.
	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			var prop Property
			if err := json.Unmarshal([]byte(tu.JSON.Input.Raw()), &prop); err != nil {
				return nil, fmt.Errorf("extract: unmarshal tool input: %w", err)
			}
			return &prop, nil
		}
	}
	return nil, fmt.Errorf("extract: no tool_use block in response (stop_reason=%s)", resp.StopReason)
}
