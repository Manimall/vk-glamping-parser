// Package slug — URL-идентификаторы объектов: транслитерация кириллицы и
// приведение к kebab-case («Деревня Ильино» → "derevnya-ilino"). Слаг попадает
// в путь /api/v1/glampings/<slug> и должен быть стабильным и URL-безопасным.
package slug

import "strings"

// translit — таблица кириллица → латиница (ГОСТ-подобная, без диакритики).
// Ь/Ъ опускаются («Ильино» → "ilino").
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e",
	'ж': "zh", 'з': "z", 'и': "i", 'й': "y", 'к': "k", 'л': "l", 'м': "m",
	'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sch",
	'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

// Make превращает произвольную строку в слаг: транслит кириллицы, латиница/цифры
// как есть (в нижнем регистре), всё прочее — дефис; дефисы схлопываются и
// обрезаются по краям. Пустой вход → пустой слаг (решение за вызывающим).
func Make(s string) string {
	var b strings.Builder
	prevDash := true // подавляет дефис в начале
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if t, known := translit[r]; known {
			// Известная кириллическая буква. Пустой транслит (ь/ъ) просто
			// опускается — это НЕ разделитель («Ильино» → "ilino").
			if t != "" {
				b.WriteString(t)
				prevDash = false
			}
			continue
		}
		// Неизвестный символ (пробел, пунктуация…) — разделитель-дефис.
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}
