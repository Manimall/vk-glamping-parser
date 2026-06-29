package extract

import (
	"context"
	"strconv"
	"strings"
)

// Heuristic — БЕСПЛАТНАЯ реализация Extractor без LLM. Разбирает текст по
// словарю ключевых слов (см. dictionary.go) и собирает Property. Это не
// «понимание», а сопоставление: нашли «баня»/«wifi»/«мангал» — разложили по
// группам. Грубее LLM, но работает офлайн, без ключей и лимитов. Когда появится
// ключ — main подменит её на LLMClient.
type Heuristic struct{}

// NewHeuristic создаёт эвристический извлекатель. Состояния у него нет —
// поэтому пустая структура; метод можно было бы повесить и на значение, но
// держим единый стиль (указатель), как у LLMClient.
func NewHeuristic() *Heuristic { return &Heuristic{} }

// Extract собирает Property из сырья. Сигнатура совпадает с Extractor, поэтому
// *Heuristic подходит везде, где ждут Extractor. Ошибку не возвращаем (всегда
// что-то отдаём), но интерфейс требует error — отдаём nil.
func (h *Heuristic) Extract(_ context.Context, in Listing) (*Property, error) {
	// Весь текст для поиска — описание товара + описание сообщества, в нижнем
	// регистре (ToLower корректно работает с кириллицей).
	text := strings.ToLower(in.Description + " " + in.About)

	prop := &Property{
		Title:         firstNonEmpty(in.Title, "Объект"),
		Summary:       summarize(in.Description, in.About),
		Location:      in.Location,
		PriceFrom:     in.Price,
		Facts:         buildFacts(in),
		AmenityGroups: buildAmenityGroups(text),
		Extras:        buildExtras(text),
		Rules:         buildRules(in.Description + " " + in.About),
	}
	return prop, nil
}

// buildAmenityGroups прогоняет словарь по тексту и собирает группы. Сохраняем
// порядок групп (slice groupOrder) и внутри — порядок правил.
func buildAmenityGroups(text string) []AmenityGroup {
	groupOrder := []string{groupInside, groupTerritory, groupEntertain}
	found := map[string][]string{} // группа → найденные удобства

	for _, r := range amenityRules {
		if matchesAny(text, r.keywords) {
			found[r.group] = append(found[r.group], r.label)
		}
	}

	groups := make([]AmenityGroup, 0, len(groupOrder))
	for _, g := range groupOrder {
		if items := found[g]; len(items) > 0 {
			groups = append(groups, AmenityGroup{Title: g, Items: items})
		}
	}
	return groups
}

// buildExtras — список платных доп.услуг по словарю. Цену из текста надёжно не
// вытащить, поэтому price оставляем пустым (фронт покажет «по запросу»).
func buildExtras(text string) []Extra {
	extras := make([]Extra, 0)
	for _, r := range extraRules {
		if matchesAny(text, r.keywords) {
			extras = append(extras, Extra{Name: r.label, Price: ""})
		}
	}
	return extras
}

// buildFacts достаёт вместимость и площадь регэкспами, если они есть в тексте.
func buildFacts(in Listing) []Fact {
	text := in.Description + " " + in.About
	facts := make([]Fact, 0)

	// Вместимость: по убыванию надёжности формата. Один FindStringSubmatch на
	// шаблон (без отдельного MatchString — это был бы лишний проход по тексту).
	if m := reGuests.FindStringSubmatch(text); m != nil {
		facts = append(facts, Fact{Label: "Вместимость", Value: m[1] + " гостей"})
	} else if m := reSleeping.FindStringSubmatch(text); m != nil {
		facts = append(facts, Fact{Label: "Вместимость", Value: m[1] + " гостей"})
	} else if m := reCapacity.FindStringSubmatch(text); m != nil {
		facts = append(facts, Fact{Label: "Вместимость", Value: "до " + m[1] + " чел."})
	}
	if m := reArea.FindStringSubmatch(text); m != nil {
		facts = append(facts, Fact{Label: "Площадь", Value: m[1] + " м²"})
	}
	if in.PhotoCount > 0 {
		facts = append(facts, Fact{Label: "Фотографий", Value: strconv.Itoa(in.PhotoCount)})
	}
	return facts
}

// buildRules вытаскивает предложения, похожие на правила (заезд/выезд/животные…).
func buildRules(text string) []string {
	rules := make([]string, 0)
	for _, sentence := range splitSentences(text) {
		low := strings.ToLower(sentence)
		if matchesAny(low, ruleKeywords) {
			if s := strings.TrimSpace(sentence); s != "" {
				rules = append(rules, s)
			}
		}
	}
	return rules
}

// --- мелкие помощники --------------------------------------------------------

func matchesAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// summarize — первое содержательное предложение (до summaryMaxRunes символов) как
// краткое описание. Берём из описания товара, при пустоте — из описания сообщества.
func summarize(primary, fallback string) string {
	src := firstNonEmpty(primary, fallback)
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	if sentences := splitSentences(src); len(sentences) > 0 {
		src = strings.TrimSpace(sentences[0])
	}
	return truncateRunes(src, summaryMaxRunes)
}

// splitSentences — грубое деление на предложения по . ! ? и переводам строк.
func splitSentences(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?' || r == '\n'
	})
}

// truncateRunes режет по РУНАМ, а не байтам — иначе можно разрубить кириллический
// символ пополам (в UTF-8 он занимает 2 байта). Поэтому строку приводим к []rune.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}
