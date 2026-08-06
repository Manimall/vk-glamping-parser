package glamping_rf

import (
	"regexp"
	"strings"
	"unicode"
)

// Источник кладёт в одно описание услуги и её варианты, и прайс мелких
// расходников: «Аренда Большая баня до 20 чел стоимость 50 000 руб. Таежная
// баня с чаном до 6 чел стоимость 12 000 руб» — но рядом бывает и «веник
// дубовый 500 р», «шапочка для бани 250 руб», «соль для ванны 500 ₽».
//
// priceFromDesc берёт первое совпадение — и объект на 7 гостей получал ценник
// бани на двадцать человек. Брать вместо этого минимум по всему описанию
// нельзя: на выгрузке 309 объектов такой подход чинил 4 случая и ломал 30 —
// ценой услуги становился веник.
//
// Поэтому вариант засчитывается, только если в нём НАЗВАНА сама услуга.
// «Таежная баня … 12 000» проходит, «веник дубовый 500 р» — нет, потому что
// слова «баня» в нём нет. Из засчитанных берём нижнюю границу с пометкой «от»:
// она честна (такая цена есть) и не отпугивает, а точный вариант гость уточнит
// в заявке — у нас лид-модель, а не оплата на сайте.

// Варианты разделены концом предложения.
var sentenceSplitRe = regexp.MustCompile(`(?:\.\s+|\n+|;\s*)`)

// Доплаты и надбавки — прибавка к цене, а не сама цена. Ищем в окне ВОКРУГ
// числа, а не по всему предложению: «Стоимость за 2 часа 5 000 ₽, каждый
// последующий час 2 500 ₽» — здесь первая цена законна, и отбрасывать всё
// предложение целиком нельзя.
var surchargeRe = regexp.MustCompile(`(?i)доплат|последующ|дополнительн|сверх|каждый\s+след|за\s+каждого|продлен|повторн|растопка`)

// Окно вокруг числа, в котором ищем признак доплаты или почасовой цены.
const priceContextWindow = 32

// Расходники и мелочь из того же прайса. Имя услуги в них часто есть — «шапочка
// для бани», «веник для бани», — поэтому одной проверки на имя мало: без этого
// списка ценой бани становились 250 ₽ за шапочку.
var consumableRe = regexp.MustCompile(`(?i)веник|шапочк|тапочк|полотенц|простын|халат|соль|масл|запарк|наполнен|чай|кофе|уголь|розжиг|дров|мыл|шампун|гель|аромат`)

// Имя услуги в описании ищем по корню: «Баня» → «бан» найдёт и «баня»,
// и «бани», и «баню». Двух букв мало (даст ложные совпадения), поэтому корень
// не короче четырёх.
const minStemLen = 4

// serviceStem — корень названия услуги для поиска в тексте варианта.
func serviceStem(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	// Берём первое слово: «Горячий чан» → искать надо «чан», а не «горячий».
	fields := strings.Fields(name)
	if len(fields) == 0 {
		return ""
	}
	// Существительное обычно последнее: «Горячий чан» → «чан», а не «горячий».
	stem := []rune(fields[len(fields)-1])
	if len(stem) < minStemLen {
		return string(stem) // короткое имя вроде «чан» ищем целиком
	}
	return string(stem[:len(stem)-1]) // «баня» → «бан», ловит и «бани», и «баню»
}

// priceVariants — цены вариантов ИМЕННО этой услуги. Меньше двух — nil, тогда
// работает обычный разбор.
func priceVariants(desc, serviceName string) []int {
	stem := serviceStem(serviceName)
	if stem == "" {
		return nil
	}
	parts := sentenceSplitRe.Split(desc, -1)
	if len(parts) < 2 {
		return nil
	}
	seen := make(map[int]bool)
	var prices []int
	for _, part := range parts {
		lower := strings.ToLower(part)
		if !strings.Contains(lower, stem) {
			continue // вариант не про эту услугу — залог, чужой прайс, доплата
		}
		if consumableRe.MatchString(lower) {
			continue // «шапочка для бани» называет услугу, но ценой не является
		}
		n, idx := priceInPart(part)
		if n <= 0 {
			continue
		}
		if isSurchargeOrHourly(part, idx) || seen[n] {
			continue
		}
		seen[n] = true
		prices = append(prices, n)
	}
	if len(prices) < 2 {
		return nil
	}
	return prices
}

// isSurchargeOrHourly — доплата или почасовая цена рядом с числом.
//
// Маркер доплаты ищем СЛЕВА от числа, во всём префиксе: «Повторная топка чана
// без замены воды 3500 руб» — слово стоит в начале, а число в конце. Но именно
// слева, а не по всему предложению: в «стоимость за 2 часа 5 000 ₽, каждый
// последующий час 2 500 ₽» первая цена законна, и маркер второй её не касается.
//
// Почасовой суффикс — справа, в узком окне: «5000 рублей в час».
func isSurchargeOrHourly(part string, idx int) bool {
	if surchargeRe.MatchString(part[:idx]) {
		return true
	}
	to := min(len(part), idx+priceContextWindow)
	tail := part[idx:to]
	return hourlyRe.MatchString(tail) || perHourRe.MatchString(tail)
}

// priceInPart — цена варианта и позиция её начала.
func priceInPart(part string) (int, int) {
	if loc := priceRe.FindStringSubmatchIndex(part); loc != nil {
		return parseDigits(part[loc[2]:loc[3]]), loc[2]
	}
	if loc := costWordRe.FindStringSubmatchIndex(part); loc != nil {
		return parseDigits(part[loc[2]:loc[3]]), loc[2]
	}
	return 0, 0
}

// lowestPrice — нижняя граница вариантов.
func lowestPrice(prices []int) int {
	lowest := prices[0]
	for _, p := range prices[1:] {
		if p < lowest {
			lowest = p
		}
	}
	return lowest
}

// normalizeDigits — цифры числа без любых пробельных разделителей и точки
// между цифрами. Источник пишет «1 500» тонким пробелом U+2009 и «1.500»
// точкой; ASCII-класс \s их не ловит, и число разбиралось как 500.
func normalizeDigits(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		switch {
		case unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '.' && i > 0 && i+1 < len(runes) &&
			unicode.IsDigit(runes[i-1]) && unicode.IsDigit(runes[i+1]):
			// разделитель тысяч — пропускаем
		case unicode.IsSpace(r):
			// пробел любой ширины — пропускаем
		}
	}
	return b.String()
}
