package extract

import (
	"strings"
	"unicode"
)

// sentences режет текст на предложения. Сканер ручной: RE2 не умеет lookbehind,
// а границу задаёт контекст вокруг точки — «реки Нерль.Идеальное место» рвём,
// «р. Волга» нет, «Wi-Fi.» рвём (иначе фразы слипаются).
func sentences(text string) []string {
	runes := []rune(text)
	out := make([]string, 0, 8)
	start := 0

	flush := func(end int) {
		part := strings.TrimSpace(string(runes[start:end]))
		if part != "" {
			out = append(out, part)
		}
	}

	for i, r := range runes {
		// Начало уже перепрыгнуло вперёд (сканер съел пробелы после
		// терминатора) — эти руны разобраны, их пропускаем.
		if i < start {
			continue
		}
		if r == '\n' {
			flush(i)
			start = i + 1
			continue
		}
		if r != '.' && r != '!' && r != '?' && r != '…' {
			continue
		}
		if r == '.' && isAbbreviation(runes, i) {
			continue
		}
		// Границей считаем терминатор, за которым идёт заглавная, цифра или
		// кавычка — с пробелом или без.
		j := i + 1
		for j < len(runes) && (runes[j] == ' ' || runes[j] == '.' || runes[j] == '!' || runes[j] == '?') {
			j++
		}
		if j >= len(runes) {
			flush(i + 1)
			start = len(runes)
			break
		}
		next := runes[j]
		if unicode.IsUpper(next) || unicode.IsDigit(next) || next == '«' || next == '"' {
			flush(i + 1)
			start = j
		}
	}
	if start < len(runes) {
		flush(len(runes))
	}
	return out
}

// isAbbreviation — точка после одной-двух кириллических букв («г.», «кв.»),
// то есть сокращение, а не конец предложения. Для латиницы не применяем:
// «Wi-Fi.» — обычный конец фразы.
func isAbbreviation(runes []rune, dot int) bool {
	n := 0
	for i := dot - 1; i >= 0 && n < 3; i-- {
		r := runes[i]
		if !unicode.IsLetter(r) {
			break
		}
		if !unicode.Is(unicode.Cyrillic, r) {
			return false
		}
		n++
	}
	return n > 0 && n <= 2
}
