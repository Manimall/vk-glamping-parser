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

// priceRe — цена из описания услуги: «5000 рублей», «1500р», «3 000 ₽».
// Требуем ≥3 цифр — иначе ловит «до 4х», «2 часа». Первое совпадение — цена.
var priceRe = regexp.MustCompile(`(\d[\d\s]{2,}|\d{3,})\s*(?:₽|руб|р\b|р/)`)

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

// priceFromDesc — цена доп.услуги из её описания: «N ₽» (как у списка) либо
// «N ₽/час» для почасовых («1200 руб. в час»). Честность прайса: почасовую
// нельзя выдавать за цену «за всё» — гость решит, что баня стоит 1 200 ₽,
// а реально «минимум 3 часа». Нет распознаваемой цены — пустая строка.
func priceFromDesc(desc string) string {
	loc := priceRe.FindStringSubmatchIndex(desc)
	if loc == nil {
		return ""
	}
	digits := strings.ReplaceAll(desc[loc[2]:loc[3]], " ", "")
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 {
		return ""
	}
	tail := desc[loc[1]:min(len(desc), loc[1]+hourlyTailWindow)]
	if hourlyRe.MatchString(tail) {
		return formatRub(n) + "/час"
	}
	return formatRub(n)
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
