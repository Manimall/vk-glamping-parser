package glamping_rf

import (
	"strings"
	"testing"
)

// fixtureHTML — минимальный слепок реальной карточки /glampings/2000: два
// ld+json блока, кадры галереи (с дублем) и текстовые маркеры вместимости.
const fixtureHTML = `<html><head>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"LodgingBusiness",
"name":"Кемпинг Тест","description":"Отдых в лесу у озера.",
"checkinTime":"14:00","checkoutTime":"12:00",
"aggregateRating":{"@type":"AggregateRating","ratingValue":"5.0000","reviewCount":"41"},
"amenityFeature":[{"@type":"LocationFeatureSpecification","name":"Интернет"},{"name":"Парковка"}]}</script>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"FAQPage","mainEntity":[
{"@type":"Question","name":"Какие правила отмены бронирования?",
 "acceptedAnswer":{"@type":"Answer","text":"<p>Бесплатная отмена за 7 дней. При более поздней отмене удерживается предоплата.</p>"}},
{"@type":"Question","name":"Можно ли с домашними животными?",
 "acceptedAnswer":{"@type":"Answer","text":"Да, размещение с питомцами разрешено по согласованию."}},
{"@type":"Question","name":"Сколько стоит проживание?",
 "acceptedAnswer":{"@type":"Answer","text":"От 12000 рублей — это НЕ правило, в rules попасть не должно."}}]}</script>
</head><body>
<img src="https://x.ru/image/cachewebp/catalog/2000/a.webp">
<img src="https://x.ru/image/cachewebp/catalog/2000/a.webp">
<img src="https://x.ru/image/cachewebp/catalog/2000/b.webp">
<img src="https://x.ru/image/cachewebp/catalog/999/other.webp">
<div>Вместимость: 4 + 2 гостя.Площадь: 80 м².Количество спален: 2</div>
</body></html>`

func TestParseDetailHTML(t *testing.T) {
	d := parseDetailHTML(fixtureHTML, 2000)

	if d.Description != "Отдых в лесу у озера." {
		t.Errorf("description: %q", d.Description)
	}
	if d.CheckIn != "14:00" || d.CheckOut != "12:00" {
		t.Errorf("заезд/выезд: %q/%q", d.CheckIn, d.CheckOut)
	}
	if d.Rating != "5.0" || d.Reviews != 41 {
		t.Errorf("рейтинг: %q · %d", d.Rating, d.Reviews)
	}
	// Фото: дедуп (a — один раз) и только СВОЕГО объекта (999 отфильтрован).
	if len(d.Photos) != 2 || !strings.HasSuffix(d.Photos[0], "/a.webp") {
		t.Errorf("photos: %v", d.Photos)
	}
	if len(d.Amenities) != 2 || d.Amenities[0] != "Интернет" {
		t.Errorf("amenities: %v", d.Amenities)
	}
	// Правила: 2 подходящих вопроса (отмена, животные), «сколько стоит» отсеян;
	// HTML-теги вычищены.
	if len(d.Rules) != 2 {
		t.Fatalf("rules: %v", d.Rules)
	}
	if strings.Contains(d.Rules[0], "<p>") || !strings.HasPrefix(d.Rules[0], "Бесплатная отмена") {
		t.Errorf("rule[0] не очищен: %q", d.Rules[0])
	}
	// Вместимость: 4 базовых + 2 доп. = 6; площадь с «узким» пробелом источника.
	if d.Guests != 6 {
		t.Errorf("guests = %d, ожидал 6", d.Guests)
	}
	if d.Area != "80 м²" {
		t.Errorf("area = %q", d.Area)
	}
}

// TestParseLdJSON_RawNewlineInsideString воспроизводит реальный баг сайта:
// буквальный перенос строки ВНУТРИ значения JSON-строки (невалидно по спеке,
// encoding/json Go иначе падает с «invalid control character»). Нашли на
// объекте id=1579 (глэмпинги.рф) — без фикса FAQPage не парсился вообще, и
// объект получал только дефолтные правила вместо реальных с сайта.
func TestParseLdJSON_RawNewlineInsideString(t *testing.T) {
	page := "<script type=\"application/ld+json\">{\"@context\":\"https://schema.org\",\"@type\":\"FAQPage\",\"mainEntity\":[" +
		"{\"@type\":\"Question\",\"name\":\"Какие правила отмены бронирования?\"," +
		"\"acceptedAnswer\":{\"@type\":\"Answer\",\"text\":\"Едем по шоссе.\nдо поста ДПС.\"}}]}</script>"

	d := &detailData{}
	parseLdJSON(page, d)

	if len(d.Rules) != 1 {
		t.Fatalf("FAQ с сырым переносом строки внутри значения должен распарситься, получил rules=%v", d.Rules)
	}
	if strings.Contains(d.Rules[0], "\n") {
		t.Errorf("перенос строки должен схлопнуться в пробел: %q", d.Rules[0])
	}
}

func TestCleanRule_CutsAtSentence(t *testing.T) {
	long := strings.Repeat("Первое предложение. ", 20) // > maxRuleRunes
	got := cleanRule(long)
	if len([]rune(got)) > maxRuleRunes || !strings.HasSuffix(got, ".") {
		t.Errorf("обрезка по предложению не сработала: %d рун, %q…", len([]rune(got)), got[:40])
	}
}
