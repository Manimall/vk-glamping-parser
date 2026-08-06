package glamping_rf

import (
	"regexp"
	"strings"
)

// Источник кладёт в одно описание услуги несколько вариантов с разными ценами:
// «Аренда Большая баня до 20 чел стоимость 50 000 руб. Таежная баня с чаном до
// 6 чел стоимость 12 000 руб». priceFromDesc берёт первое совпадение — и объект
// на 7 гостей получал ценник бани на двадцать человек. Гость видит 50 000 при
// цене ночи 8 100 и уходит, а подходящий вариант до него не доезжает.
//
// Показываем нижнюю границу с пометкой «от»: она честна (такая цена
// действительно есть) и не отпугивает. Точный вариант гость уточнит в заявке —
// у нас лид-модель, а не оплата на сайте.

// Варианты разделены концом предложения. Точка внутри числа («50 000.» в конце)
// не мешает: разделителем считается точка с пробелом или переводом строки.
var sentenceSplitRe = regexp.MustCompile(`(?:\.\s+|\n+|;\s*)`)

// Доплаты и надбавки — не самостоятельная цена услуги, а прибавка к ней.
// Без этого «Базовая аренда 5000. Каждый последующий день: доплата 3500»
// схлопнулось бы в «от 3 500 ₽», хотя аренда стоит 5 000.
var surchargeRe = regexp.MustCompile(`(?i)доплат|последующ|дополнительн|сверх|каждый\s+след|за\s+каждого`)

// priceVariants разбирает описание на самостоятельные варианты и возвращает их
// цены в рублях. Меньше двух вариантов — nil: тогда работает обычный разбор.
func priceVariants(desc string) []int {
	parts := sentenceSplitRe.Split(desc, -1)
	if len(parts) < 2 {
		return nil
	}
	var prices []int
	for _, part := range parts {
		if strings.TrimSpace(part) == "" || surchargeRe.MatchString(part) {
			continue
		}
		// Почасовые и сеансовые цены в сравнение не берём: «5 000 ₽ за 3 ч»
		// и «12 000 ₽ за всё» — разные величины, минимум из них бессмыслен.
		if perHourRe.MatchString(part) || hourlyRe.MatchString(part) {
			continue
		}
		if n := priceInPart(part); n > 0 {
			prices = append(prices, n)
		}
	}
	if len(prices) < 2 {
		return nil
	}
	return prices
}

// priceInPart — цена одного варианта, теми же правилами, что и priceFromDesc.
func priceInPart(part string) int {
	if m := priceRe.FindStringSubmatch(part); m != nil {
		return parseDigits(m[1])
	}
	if m := costWordRe.FindStringSubmatch(part); m != nil {
		return parseDigits(m[1])
	}
	return 0
}

// lowestPrice — минимум из вариантов.
func lowestPrice(prices []int) int {
	lowest := prices[0]
	for _, p := range prices[1:] {
		if p < lowest {
			lowest = p
		}
	}
	return lowest
}
