package extract

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
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
	// seoDescMaxRunes — мягкий предел длины «питча» (без имени и CTA).
	seoDescMaxRunes = 160
	// seoMinRichRunes — ниже этого «питч» считаем бедным → берём шаблон-фоллбэк.
	seoMinRichRunes = 30
	// seoFallbackPitch — нейтральный питч, когда у объекта нет своего описания.
	seoFallbackPitch = "уютный дом для отдыха на природе"
)

// distanceRe выхватывает «живую» строку локации вида «18 км от Иваново» из
// описания (там она привлекательнее сухого адреса).
var distanceRe = regexp.MustCompile(`\d+\s*км\s+от\s+[А-ЯЁ][А-Яа-яЁё-]+`)

// emojiRe — эмодзи и модификаторы (чистим тексты, идущие в предложения).
var emojiRe = regexp.MustCompile(`[\x{1F000}-\x{1FAFF}\x{2600}-\x{27BF}\x{2190}-\x{21FF}\x{2B00}-\x{2BFF}\x{FE00}-\x{FE0F}\x{200D}]`)

// junkLineRe — строки описания, не относящиеся к презентации места (бронь,
// контакты, цена): их в SEO-описание не берём.
var junkLineRe = regexp.MustCompile(`(?i)(заброн|свободные дат|сообщени|telegram|вконтакт|http|@|☎|whatsapp|цена|стоимость|₽)`)

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

// buildDescription — «<Имя> — <питч>. Бронь в три тапа.». Питч — собственный
// текст объекта (что за место, почему стоит), иначе нейтральный фоллбэк с локацией.
func buildDescription(name, subtitle, about string) string {
	pitch := aboutPitch(about)
	if pitch == "" {
		pitch = seoFallbackPitch
		if subtitle != "" {
			pitch = fmt.Sprintf("%s, %s", seoFallbackPitch, subtitle)
		}
	} else {
		pitch = lowerFirst(pitch) // после «Имя — » питч идёт с маленькой буквы
	}
	return fmt.Sprintf("%s — %s. %s", name, pitch, seoCTA)
}

// aboutPitch собирает «питч» из описания сообщества: осмысленные строки (без
// эмодзи и без строк про бронь/контакты/цену), склеенные в предложения, с мягким
// лимитом длины. Пусто, если своего описания мало (тогда buildDescription берёт
// фоллбэк).
func aboutPitch(about string) string {
	kept := make([]string, 0)
	total := 0
	for _, ln := range strings.Split(about, "\n") {
		ln = strings.TrimSpace(emojiRe.ReplaceAllString(ln, ""))
		ln = strings.Trim(ln, " .·—-")
		// Строку-дистанцию («N км от Города») в питч не берём — она уже в Subtitle,
		// иначе описание рубленое и длиннее (мета-описание режется на ~160).
		if ln == "" || !hasLetter(ln) || junkLineRe.MatchString(ln) || distanceRe.MatchString(ln) {
			continue
		}
		r := len([]rune(ln))
		if len(kept) > 0 && total+r > seoDescMaxRunes {
			break
		}
		kept = append(kept, ln)
		total += r
	}
	if total < seoMinRichRunes {
		return ""
	}
	return strings.Join(kept, ". ")
}

// locationHighlight — «живая» строка локации: если в описании есть «N км от
// Города» — берём её (точнее и привлекательнее адреса), иначе структурный адрес.
func locationHighlight(about, fallback string) string {
	if m := distanceRe.FindString(about); m != "" {
		return strings.Join(strings.Fields(m), " ") // нормализуем пробелы
	}
	return strings.TrimSpace(fallback)
}

// hasLetter — есть ли в строке хоть одна буква (отсекаем строки из одних символов/
// невидимых, напр. «⠀» или «—»).
func hasLetter(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}

// lowerFirst делает первую руну строчной (питч после «Имя — »).
func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	r[0] = unicode.ToLower(r[0])
	return string(r)
}
