package glamping_rf

// Встроенный JSON window.pv12RoomDetails detail-страницы: домики с удобствами,
// у каждого удобства пометка платности и описание с ценой. Платные удобства —
// источник доп.услуг (баня/чан/питомец), которые гость заказывает → растят чек.

import (
	"encoding/json"
	"html"
	"regexp"
	"strconv"
	"strings"

	"vk-parser/internal/extract"
)

// roomDetailsMarker — начало встроенного JSON с домиками и их удобствами.
const roomDetailsMarker = "window.pv12RoomDetails ="

// pv12Room — домик из window.pv12RoomDetails.
type pv12Room struct {
	Amenities []pv12Amenity `json:"amenities"`
}

type pv12Amenity struct {
	Name string `json:"name"`
	Paid bool   `json:"paid"`
	Desc string `json:"desc"` // текст с ценой: «Доплата 1500р/питомец»
}

// priceRe — цена с валютой: «5000 рублей», «1500р.», «3 000 ₽», «1000р/питомец».
// Требуем ≥3 цифр — иначе ловит «до 4х», «2 часа». После одиночной «р» — НЕ
// буква/цифра либо конец строки: \b в Go ASCII-only и кириллицу не понимает
// (та же грабля, что с «от» в isRangePrice фронта) — из-за неё «6000р.»
// раньше оставался без цены.
var priceRe = regexp.MustCompile(`(\d[\d\s]{2,}|\d{3,})\s*(?:₽|руб|р(?:[^а-яё0-9]|$))`)

// perHourRe — цена без валюты в формате «2500/час» или «15000/3 часа»
// (сеанс из N часов). Группа 2 — длительность сеанса, если указана.
var perHourRe = regexp.MustCompile(`(\d[\d\s]{2,}|\d{3,})\s*/\s*(\d+)?\s*час`)

// costWordRe — «Стоимость сеанса 10000»: голая сумма в пределах 20 символов
// после слова «стоимость» (валюту на сайте часто опускают).
var costWordRe = regexp.MustCompile(`(?i)стоимость[^0-9]{0,20}(\d[\d\s]{2,}|\d{3,})`)

// bareNumberRe — описание целиком из одного числа («350» у халатов) — это цена.
var bareNumberRe = regexp.MustCompile(`^\s*(\d[\d\s]{1,8})\s*$`)

// hourlyTailWindow — сколько символов после суммы смотрим в поисках «час»
// («1200 руб. в час», «5000 рублей в час»): дальше по тексту слово «час» уже
// не относится к цене.
const hourlyTailWindow = 20

// hourlyRe — признак почасовой цены в хвосте сразу после суммы.
var hourlyRe = regexp.MustCompile(`(?i)[в/]\s*час`)

// detailPaidExtras — платные услуги объекта из window.pv12RoomDetails:
// собираем помеченные paid (дедуп по имени), цена — из описания.
func detailPaidExtras(page string) []extract.Extra {
	raw := balancedJSON(page, roomDetailsMarker, '{', '}')
	if raw == "" {
		return nil
	}
	var rooms map[string]pv12Room
	if err := json.Unmarshal([]byte(raw), &rooms); err != nil {
		return nil
	}
	seen := make(map[string]bool)
	var extras []extract.Extra
	for _, room := range rooms {
		for _, a := range room.Amenities {
			name := html.UnescapeString(strings.TrimSpace(a.Name))
			if !a.Paid || name == "" || seen[name] {
				continue
			}
			seen[name] = true
			extras = append(extras, extract.Extra{Name: name, Price: priceFromDesc(a.Desc)})
		}
	}
	return extras
}

// priceFromDesc — цена доп.услуги из её описания. Форматы по убыванию
// уверенности: сумма с валютой («1500р.», «3 000 ₽»); без валюты «2500/час» и
// сеанс «15000/3 часа»; «Стоимость сеанса 10000»; описание из одного числа.
// Честность прайса: почасовую/сеансовую нельзя выдавать за цену «за всё» —
// суффикс «/час» или «за N ч». Нет распознаваемой цены — пустая строка.
func priceFromDesc(desc string) string {
	if loc := priceRe.FindStringSubmatchIndex(desc); loc != nil {
		n := parseDigits(desc[loc[2]:loc[3]])
		if n <= 0 {
			return ""
		}
		tail := desc[loc[1]:min(len(desc), loc[1]+hourlyTailWindow)]
		if hourlyRe.MatchString(tail) {
			return formatRub(n) + "/час"
		}
		return formatRub(n)
	}
	if m := perHourRe.FindStringSubmatch(desc); m != nil {
		if n := parseDigits(m[1]); n > 0 {
			if m[2] != "" {
				return formatRub(n) + " за " + m[2] + " ч" // сеанс: «15 000 ₽ за 3 ч»
			}
			return formatRub(n) + "/час"
		}
	}
	if m := costWordRe.FindStringSubmatch(desc); m != nil {
		if n := parseDigits(m[1]); n > 0 {
			return formatRub(n)
		}
	}
	if m := bareNumberRe.FindStringSubmatch(desc); m != nil {
		if n := parseDigits(m[1]); n > 0 {
			return formatRub(n)
		}
	}
	return ""
}

// parseDigits — число из строки с пробелами-разделителями («15 000» → 15000).
func parseDigits(s string) int {
	n, err := strconv.Atoi(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		return 0
	}
	return n
}

// formatRub — цена в стиле сайта: «5 000 ₽» (пробел-разделитель тысяч).
func formatRub(n int) string {
	s := strconv.Itoa(n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(c)
	}
	b.WriteString(" ₽")
	return b.String()
}

// balancedJSON вырезает первый сбалансированный литерал (объект/массив) после
// marker, честно пропуская скобки внутри строк и экранирование. Пусто — если
// marker не найден или скобки не сошлись.
func balancedJSON(page, marker string, open, close byte) string {
	i := strings.Index(page, marker)
	if i < 0 {
		return ""
	}
	start := strings.IndexByte(page[i:], open)
	if start < 0 {
		return ""
	}
	start += i
	depth, inStr, esc := 0, false, false
	for k := start; k < len(page); k++ {
		c := page[k]
		switch {
		case inStr:
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
		case c == '"':
			inStr = true
		case c == open:
			depth++
		case c == close:
			depth--
			if depth == 0 {
				return page[start : k+1]
			}
		}
	}
	return ""
}
