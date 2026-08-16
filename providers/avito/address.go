package avito

import "strings"

// Адрес показа: что из адреса объявления можно публиковать.

// streetPrefixes и streetWords — признаки сегмента с улицей: дальше него в
// адресе идёт частный дом, а не населённый пункт.
//
// Двумя списками, потому что тип пишут с обеих сторон названия: «ул. Тверская»
// и «Садовая ул.». Проверять одним вхождением нельзя — сокращение без границы
// найдётся внутри имени населённого пункта («Шарья» содержит «ш.»? нет, но
// «Прохоровка» содержит «пр.»), а имя посёлка терять нельзя.
var streetPrefixes = []string{"ул.", "ул ", "пр-т", "пр-кт", "пр.", "пер.", "наб.", "б-р", "ш.", "мкр", "кв-л"}

var streetWords = []string{"улица", "проспект", "переулок", "шоссе", "набережная", "бульвар", "проезд", "тракт", "тупик", "аллея", "квартал"}

// publicAddress — адрес, который не стыдно опубликовать: регион и населённый
// пункт, без улицы и номера дома.
//
// Авито отдаёт точный адрес объекта вплоть до дома, и он уезжал в каталог
// целиком: в плитку выдачи, в подпись на странице, а главное — в <title>
// пререндеренной страницы и в og:image:alt, то есть прямиком в поисковый
// индекс. У «Отдыха с Фурако» это 132 символа, оканчивающиеся «Садовая ул., 32»
// — домашний адрес человека, который согласия на публикацию не давал: каталог
// собран из открытых объявлений, а не по договорённости с владельцами.
//
// Точку на карте это не ломает: координаты приезжают отдельным полем и берутся
// из geo самого объявления.
func publicAddress(address string) string {
	parts := strings.Split(address, ",")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" || isStreet(trimmed) || isHouseNumber(trimmed) {
			break
		}
		kept = append(kept, trimmed)
	}
	if len(kept) == 0 {
		return ""
	}
	// Регион и последний населённый пункт: промежуточные районы и поселения
	// строку удлиняют, а гостю не говорят ничего сверх того, что уже видно.
	if len(kept) > 2 {
		kept = []string{kept[0], kept[len(kept)-1]}
	}
	return strings.Join(kept, ", ")
}

func isStreet(segment string) bool {
	lower := strings.ToLower(segment)
	for _, p := range streetPrefixes {
		if strings.HasPrefix(lower, p) || strings.HasSuffix(lower, " "+strings.TrimSpace(p)) {
			return true
		}
	}
	for _, w := range streetWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// isHouseNumber — сегмент из одного числа (возможно с литерой или корпусом):
// «32», «12А», «5 к2».
func isHouseNumber(segment string) bool {
	for _, r := range segment {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}
