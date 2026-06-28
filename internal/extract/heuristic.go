package extract

import (
	"context"
	"regexp"
	"strconv"
	"strings"
)

// Heuristic — БЕСПЛАТНАЯ реализация Extractor без LLM. Разбирает текст по
// словарю ключевых слов и собирает Property. Это не «понимание», а сопоставление:
// нашли «баня»/«wifi»/«мангал» — разложили по группам. Грубее LLM, но работает
// офлайн, без ключей и лимитов. Когда появится ключ — main подменит её на LLMClient.
type Heuristic struct{}

// NewHeuristic создаёт эвристический извлекатель. Состояния у него нет —
// поэтому пустая структура; метод можно было бы повесить и на значение, но
// держим единый стиль (указатель), как у LLMClient.
func NewHeuristic() *Heuristic { return &Heuristic{} }

// amenityRule — правило словаря: если в тексте встретилось любое из keywords,
// добавляем label в группу group. Слайс правил = наш «движок» сопоставления (DRY:
// одна структура данных вместо россыпи if-ов).
type amenityRule struct {
	keywords []string
	label    string
	group    string
}

const (
	groupInside    = "В домике"
	groupTerritory = "На территории"
	groupEntertain = "Развлечения"
)

// amenityRules — словарь удобств. Порядок задаёт порядок появления в карточке.
// Подстроки подобраны под реальные тексты (без «жадных» кусков вроде "печ",
// который ловит «микроволновая печь» как «отопление»).
var amenityRules = []amenityRule{
	{[]string{"кухн", "микроволнов", "холодильник", "чайник", "кофеварк", "тостер", "плита", "посуд"}, "Кухня (оборудованная)", groupInside},
	{[]string{"ванная", "полотенц", "халат"}, "Ванная, полотенца, халаты", groupInside},
	{[]string{"средства гигиен", "тапочк", "зубной"}, "Средства гигиены", groupInside},
	{[]string{"туалет", "санузел", "с/у"}, "Санузел", groupInside},
	{[]string{"кровать", "матрас", "диван", "спальн"}, "Спальные места", groupInside},
	{[]string{"постельное бель", "постельн"}, "Постельное бельё", groupInside},
	{[]string{"настольные игры", "настольн"}, "Настольные игры", groupInside},
	{[]string{"проектор", "телевизор"}, "ТВ / проектор", groupInside},
	{[]string{"станция", "алиса"}, "Умная колонка (Алиса)", groupInside},
	{[]string{"барная стойка", "стол со стул", "стол со стульями"}, "Стол / барная стойка", groupInside},
	{[]string{"wifi", "wi-fi", "вай-фай", "вайфай", "интернет"}, "Интернет", groupInside},
	{[]string{"отоплен", "обогрев", "тёплый пол", "теплый пол", "камин"}, "Отопление", groupInside},
	{[]string{"кондиционер"}, "Кондиционер", groupInside},
	{[]string{"водонагреват", "бойлер", "фен", "утюг"}, "Бытовая техника", groupInside},

	{[]string{"мангал", "шампур", "барбекю", "bbq", "гриль"}, "Мангальная зона", groupTerritory},
	{[]string{"костров", "костёр", "костер", "чаша"}, "Костровище", groupTerritory},
	{[]string{"уличный душ"}, "Уличный душ", groupTerritory},
	{[]string{"детская площадк", "детская площ"}, "Детская площадка", groupTerritory},
	{[]string{"сетка", "игры в мяч", "мяч"}, "Спортивная площадка", groupTerritory},
	{[]string{"парковк", "стоянк"}, "Парковка", groupTerritory},
	{[]string{"беседк"}, "Беседка", groupTerritory},
	{[]string{"терасс", "террас", "веранд"}, "Терраса", groupTerritory},
	{[]string{"причал", "пирс"}, "Причал", groupTerritory},

	{[]string{"фурако", "чан", "купель", "джакузи"}, "Фурако / банный чан", groupEntertain},
	{[]string{"баня", "сауна"}, "Баня / сауна", groupEntertain},
	{[]string{"бассейн"}, "Бассейн", groupEntertain},
	{[]string{"лошад", "катание на лошад"}, "Катание на лошадях", groupEntertain},
	{[]string{"квадроцикл"}, "Квадроциклы", groupEntertain},
	{[]string{"сап", "sup", "лодк", "каяк", "байдар"}, "Сап-доски / лодки", groupEntertain},
	{[]string{"велосипед", "велопрокат"}, "Велосипеды", groupEntertain},
	{[]string{"рыбалк"}, "Рыбалка", groupEntertain},
}

// extraRules — ПЛАТНЫЕ доп.услуги (отдельно от удобств, входящих в стоимость).
var extraRules = []amenityRule{
	{[]string{"фурако", "растопка чан", "растопка"}, "Растопка Фурако / чана", ""},
	{[]string{"сауна"}, "Сауна", ""},
	{[]string{"сервировк"}, "Праздничная сервировка", ""},
	{[]string{"завтрак"}, "Завтраки", ""},
	{[]string{"доп место", "дополнительное место", "доп. место", "доп. гость", "доп.гость"}, "Дополнительный гость / место", ""},
	{[]string{"животн", "питом", "собак"}, "Размещение с животными", ""},
	{[]string{"доставк"}, "Доставка еды", ""},
	{[]string{"трансфер"}, "Трансфер", ""},
}

// Регэкспы для фактов. (?i) — без учёта регистра. Вместимость пишут по-разному,
// поэтому несколько шаблонов в порядке надёжности:
//   - reGuests   — «Количество гостей: 8» (число ПОСЛЕ слова — самый явный формат,
//     не ловит «на 6 гостей» из строки про сервировку);
//   - reSleeping — «8 спальных мест»;
//   - reCapacity — «до 4 чел», «максимум 4 человека» (без «гост», чтобы снова не
//     цеплять «на 6 гостей»).
//
// Площадь — «84 м²».
var (
	reGuests   = regexp.MustCompile(`(?i)гостей\D{0,4}(\d+)`)
	reSleeping = regexp.MustCompile(`(?i)(\d+)\s*спальных\s+мест`)
	reCapacity = regexp.MustCompile(`(?i)(?:до|на|максимум|для)\s+(\d+)\s*(?:чел|человек|персон)`)
	reArea     = regexp.MustCompile(`(?i)(\d+)\s*(?:м²|м2|кв\.?\s*м)`)
)

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

	// Вместимость: по убыванию надёжности формата.
	switch {
	case reGuests.MatchString(text):
		facts = append(facts, Fact{Label: "Вместимость", Value: reGuests.FindStringSubmatch(text)[1] + " гостей"})
	case reSleeping.MatchString(text):
		facts = append(facts, Fact{Label: "Вместимость", Value: reSleeping.FindStringSubmatch(text)[1] + " гостей"})
	case reCapacity.MatchString(text):
		facts = append(facts, Fact{Label: "Вместимость", Value: "до " + reCapacity.FindStringSubmatch(text)[1] + " чел."})
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
	keywords := []string{"заезд", "выезд", "заселен", "правил", "животн", "питом", "курен", "залог", "депозит", "тишин", "вечеринк", "дет", "договор", "обув"}
	rules := make([]string, 0)
	for _, sentence := range splitSentences(text) {
		low := strings.ToLower(sentence)
		if matchesAny(low, keywords) {
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

// summarize — первое содержательное предложение (до ~200 символов) как краткое
// описание. Берём из описания товара, при пустоте — из описания сообщества.
func summarize(primary, fallback string) string {
	src := firstNonEmpty(primary, fallback)
	src = strings.TrimSpace(src)
	if src == "" {
		return ""
	}
	if sentences := splitSentences(src); len(sentences) > 0 {
		src = strings.TrimSpace(sentences[0])
	}
	return truncateRunes(src, 200)
}

// (для «число → строка» используем strconv.Itoa из stdlib — см. buildFacts.)

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
