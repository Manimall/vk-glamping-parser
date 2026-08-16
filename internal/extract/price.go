package extract

import (
	"strconv"
	"strings"
)

// FormatPrice — цена для показа: «6000» → «6 000 ₽».
//
// Формат задан не здесь: 319 объектов каталога уже лежат с таким разделителем
// (обычный пробел), и цена собирается в строку в двух провайдерах сразу.
// Разойдись они — одно и то же число печаталось бы на карточках по-разному.
func FormatPrice(value int) string {
	digits := strconv.Itoa(value)
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	b.WriteString(" ₽")
	return b.String()
}
