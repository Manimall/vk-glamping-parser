package avito

import "strings"

// Нормализация гомоглифов: продавцы Авито намеренно подменяют кириллические
// буквы визуально одинаковыми латинскими («Cаунa внутpи», «MЫ coздали»), чтобы
// обходить автофильтры площадки. Глазу разницы нет, машине — есть: поиск по
// каталогу такой объект не найдёт, а slug.Make даст мусор вроде
// "otdyh-s-furako-v-priyute-muzykanta" → "otdyh-s-furako-v-priyute-muzykanta"
// с латинскими буквами внутри транслита.
//
// Подтверждено на всех трёх сохранённых страницах — это норма источника, а не
// разовая случайность, поэтому чистка обязательна ДО slug.Make и записи в каталог.

// homoglyphs — латиница → визуально совпадающая кириллица. Только буквы, которые
// действительно неразличимы в шрифтах: «b»/«ь» и «n»/«п» сюда НЕ входят, они
// отличимы и подменяются продавцами гораздо реже.
var homoglyphs = map[rune]rune{
	'a': 'а', 'c': 'с', 'e': 'е', 'o': 'о', 'p': 'р', 'x': 'х', 'y': 'у',
	'A': 'А', 'B': 'В', 'C': 'С', 'E': 'Е', 'H': 'Н', 'K': 'К', 'M': 'М',
	'O': 'О', 'P': 'Р', 'T': 'Т', 'X': 'Х', 'Y': 'У',
}

// NormalizeHomoglyphs возвращает текст, в котором латинские подмены заменены на
// кириллицу — но ТОЛЬКО внутри слов, где кириллица уже есть.
//
// Условие «в слове есть кириллица» и делает функцию безопасной: настоящие
// латинские слова из описаний («A-frame», «SPA», «WhatsApp») кириллицы не
// содержат и остаются нетронутыми. Слепая замена по таблице превратила бы
// «A-frame» в «А-frame» — то есть сломала бы ровно тот термин, вокруг которого
// построен весь каталог.
func NormalizeHomoglyphs(s string) string {
	runes := []rune(s)
	out := make([]rune, len(runes))
	copy(out, runes)

	// Идём словами: слово — максимальная цепочка букв (латиница+кириллица).
	// Разделители (пробелы, пунктуация, цифры) слово обрывают.
	for start := 0; start < len(runes); {
		if !isLetter(runes[start]) {
			start++
			continue
		}
		end := start
		hasCyrillic := false
		for end < len(runes) && isLetter(runes[end]) {
			if isCyrillic(runes[end]) {
				hasCyrillic = true
			}
			end++
		}
		if hasCyrillic {
			for i := start; i < end; i++ {
				if cyr, ok := homoglyphs[runes[i]]; ok {
					out[i] = cyr
				}
			}
		}
		start = end
	}
	return string(out)
}

func isLetter(r rune) bool { return isCyrillic(r) || isLatin(r) }

func isLatin(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

// isCyrillic — основной блок кириллицы (без расширений для других языков):
// А-я по коду плюс отдельно стоящая Ё/ё.
func isCyrillic(r rune) bool {
	return r >= 'А' && r <= 'я' || r == 'Ё' || r == 'ё'
}

// CollapseSpaces приводит пробельные символы к одному пробелу и обрезает края.
// Описания Авито приходят из HTML: там неразрывные пробелы (  — им же
// разделены «1 ч» и «8 000 ₽»), переводы строк и двойные отбивки. Для полей
// контракта нужен ровный однострочный текст.
func CollapseSpaces(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, " ", " ")), " ")
}
