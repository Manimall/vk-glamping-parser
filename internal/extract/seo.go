package extract

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SEO-тексты объекта для превью в поиске и соцсетях. Генерятся из КОНТЕНТА
// (короткое имя + собственный «питч» объекта из описания сообщества) — БЕЗ бренда
// сайта: бренд/вёрстку добавляет фронт (разделение «контент (бэк) × презентация»).
//
// Описание презентует МЕСТО (что это и почему стоит поехать), а не перечисляет
// удобства (холодильник/микроволновка) — в стиле карточек Сабадури/ЁлкиДом/Scandi.

const (
	// seoCTA — призыв к брони в конце описания (единый стиль карточек сайта).
	seoCTA = "Бронь в три тапа."
	// seoFallbackPitch — нейтральный питч, когда у объекта нет своего описания.
	seoFallbackPitch = "уютный дом для отдыха на природе"
	// seoJoin — то, что описание добавляет вокруг питча: « — » и «. ».
	seoJoin = " — . "
)

// distanceRe выхватывает «живую» строку локации вида «18 км от Иваново» из
// описания (там она привлекательнее сухого адреса).
var distanceRe = regexp.MustCompile(`\d+\s*км\s+от\s+[А-ЯЁ][А-Яа-яЁё-]+`)

// emojiRe — эмодзи и модификаторы (чистим тексты, идущие в предложения).
var emojiRe = regexp.MustCompile(`[\x{1F000}-\x{1FAFF}\x{2600}-\x{27BF}\x{2190}-\x{21FF}\x{2B00}-\x{2BFF}\x{FE00}-\x{FE0F}\x{200D}]`)

// junkLineRe — строки описания, не относящиеся к презентации места (бронь,
// контакты, цена): их в SEO-описание не берём.
var junkLineRe = regexp.MustCompile(`(?i)(заброн|свободные дат|сообщени|telegram|вконтакт|http|@|☎|whatsapp|цена|стоимость|₽|\d[\d ,.]*\s*руб|доп\.?\s*плат|за отдельную плату|платн(ая|ую|ой|ым)|скидк|прайс)`)

// SEO — заголовок/описание для мета-тегов и короткая строка локации для OG.
type SEO struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Subtitle    string `json:"subtitle"` // короткая локация для OG-подзаголовка
}

// SEOInput — входные сигналы генерации (чистые данные, без сети/диска).
type SEOInput struct {
	Name     string // короткое имя объекта (обычно заголовок домика)
	Location string // структурный адрес (фоллбэк для Subtitle)
	About    string // описание сообщества — источник «живого» питча и локации
}

// BuildSEO собирает SEO/OG-тексты из контента объекта. Чистая функция —
// тестируется без сети. Пустое имя → пустой SEO (звать при наличии объекта).
func BuildSEO(in SEOInput) SEO {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return SEO{}
	}
	subtitle := locationHighlight(in.About, in.Location)

	title := name
	if subtitle != "" {
		title = fmt.Sprintf("%s — %s", name, subtitle)
	}

	return SEO{
		Title:       title,
		Description: buildDescription(name, subtitle, in.About),
		Subtitle:    subtitle,
	}
}

// buildDescription — «<Имя> — <питч>. Бронь в три тапа.».
//
// Бюджет считается от ВСЕЙ строки, а не от одного питча: имя и призыв
// прибавляются сверху, и лимит на питч не мешал описанию вырасти до 2881
// символа при объявленных 160.
//
// Питч берётся целым предложением с заглавной буквы — lowerFirst убран: он
// ломал имена собственные («Сабадури» → «сабадури», «Купель Фурако» →
// «купель»), а «Имя — Предложение» типографски корректно.
func buildDescription(name, subtitle, about string) string {
	// Руны, а не байты: len(" — . ") даёт 7 при пяти символах — тире занимает
	// три байта, и бюджет молча терял два символа у каждого объекта.
	budget := seoDescTotalRunes - runes(name) - runes(seoCTA) - runes(seoJoin)
	if pitch := pickPitch(about, name, budget); pitch != "" {
		return fmt.Sprintf("%s — %s. %s", name, pitch, seoCTA)
	}
	return fmt.Sprintf("%s — %s. %s", name, fallbackPitch(subtitle, budget), seoCTA)
}

// fallbackPitch — нейтральный шаблон с локацией, если своего текста нет.
// Локация приезжает из данных и бывает длинной («Ивановская обл., Ивановский
// р-н, д. Крюково, Славянская ул., 6»), поэтому её тоже держим в бюджете:
// иначе инвариант «описание не длиннее лимита» верен только для одной из
// двух веток.
func fallbackPitch(subtitle string, budget int) string {
	if subtitle == "" {
		return seoFallbackPitch
	}
	full := fmt.Sprintf("%s, %s", seoFallbackPitch, subtitle)
	if runes(full) <= budget {
		return full
	}
	return seoFallbackPitch
}

func runes(s string) int { return utf8.RuneCountInString(s) }

// locationHighlight — «живая» строка локации: если в описании есть «N км от
// Города» — берём её (точнее и привлекательнее адреса), иначе структурный адрес.
func locationHighlight(about, fallback string) string {
	if m := distanceRe.FindString(about); m != "" {
		return strings.Join(strings.Fields(m), " ") // нормализуем пробелы
	}
	return strings.TrimSpace(fallback)
}
