package avito

import "strings"

// Регион объекта из адреса объявления.
//
// Region — ось группировки: каталог, фильтры и мастер подбора бота сводят
// объекты по СОВПАДЕНИЮ строки, а нормализация на сайте (resolveRegion) поле
// источника не трогает — доверяет ему. Поэтому форма записи задаётся здесь и
// обязана совпадать с той, что уже лежит в каталоге: «Ивановская обл.» рядом с
// «Ивановская область» дала бы в фильтре регион-двойник, и объекты Авито
// прятались бы в нём (проверено сквозным прогоном синхронизации: было 11
// регионов вместо 10).

// regionFullSuffixes и regionFullPrefixes — формы, уже записанные полностью:
// разворачивать нечего, но узнать регион по ним обязаны. Держать их отдельно от
// словаря сокращений приходится потому, что «край» никто не сокращает.
var regionFullSuffixes = []string{" область", " край", " автономный округ", " автономная область", " республика"}

var regionFullPrefixes = []string{"республика "}

// regionSuffixExpansions — словарь сокращений Авито: «Ивановская обл.» и
// «Ивановская область» — одно и то же, и в каталог обязана уехать вторая форма.
//
// «Республика» с заглавной не по прихоти: у республик с прилагательным
// названием тип пишется вторым словом («Чувашская Республика»), и это часть
// имени субъекта, а не пояснение к нему.
var regionSuffixExpansions = []struct{ short, full string }{
	{" обл.", " область"},
	{" обл", " область"},
	{" респ.", " Республика"},
	{" респ", " Республика"},
	{" ао", " автономный округ"},
	{" аобл", " автономная область"},
}

// regionExceptions — субъекты, у которых общее правило даёт неверную форму.
//
// Единственная в стране автономная ОБЛАСТЬ сокращается и как «Аобл», и как
// «АО», а «АО» в остальных двенадцати случаях — округ. Общее правило выдало бы
// «Еврейская автономный округ»: безграмотный ярлык в оси группировки, да ещё и
// двойник к «Еврейской автономной области» при первом же объекте с «Аобл».
var regionExceptions = map[string]string{
	"еврейская ао": "Еврейская автономная область",
}

// regionPrefixExpansions — сокращения, которые Авито ставит ПЕРЕД названием:
// республики пишутся «Респ. Карелия», а не «Карельская респ.».
var regionPrefixExpansions = []struct{ short, full string }{
	{"респ. ", "Республика "},
	{"респ ", "Республика "},
}

// region — регион из первого сегмента адреса источника.
//
// Это НЕ обратный разбор contract.Location (он запрещён): читаем структурное
// поле address самого Авито, у которого регион всегда идёт первым. Проверяем
// форму записи, потому что у городов федерального значения первый сегмент —
// сразу город («Москва, ул. …»), и подставлять его как регион нельзя: город в
// оси группировки сломает фильтр.
// Запятая не обязательна: у сервисных объявлений адрес бывает и одним
// сегментом («Ивановская обл.»), а форму записи проверяет looksLikeRegion — ей
// всё равно, есть ли продолжение.
func region(address string) string {
	first, _, _ := strings.Cut(address, ",")
	first = strings.TrimSpace(first)
	lower := strings.ToLower(first)
	if !looksLikeRegion(lower) {
		return ""
	}
	return expandRegion(first, lower)
}

// canonAddress — адрес источника, в котором сокращение региона развёрнуто так
// же, как в поле Region.
//
// Без этого два поля одного объекта противоречат друг другу, и страдает от
// этого плитка каталога: displayLocation на сайте сокращает строку места до
// региона по совпадению префикса (`place.startsWith(area)`). «Ивановская обл.,
// …» с «Ивановская область» не совпадает, и в плитку вместо названия региона
// уезжает адрес целиком — до 93 символов с номером дома частного владельца
// рядом с короткими строками остальных 320 объектов.
func canonAddress(address string) string {
	first, rest, found := strings.Cut(address, ",")
	trimmed := strings.TrimSpace(first)
	lower := strings.ToLower(trimmed)
	if !looksLikeRegion(lower) {
		return address
	}
	expanded := expandRegion(trimmed, lower)
	if !found {
		return expanded
	}
	return expanded + "," + rest
}

// looksLikeRegion — первый сегмент назван регионом, а не городом.
//
// Списки сокращений участвуют в проверке наравне с полными формами, и это не
// удобство, а условие связности: заведи новое сокращение в словаре, но забудь
// продублировать его здесь — оно молча перестанет считаться регионом, и объект
// выпадет из фильтра без единой ошибки.
func looksLikeRegion(lower string) bool {
	for _, suffix := range regionFullSuffixes {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	for _, prefix := range regionFullPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	for _, e := range regionSuffixExpansions {
		if strings.HasSuffix(lower, e.short) {
			return true
		}
	}
	for _, e := range regionPrefixExpansions {
		if strings.HasPrefix(lower, e.short) {
			return true
		}
	}
	return false
}

// expandRegion разворачивает сокращение источника в полную форму каталога.
//
// Порядок перебора — от длинного к короткому, иначе « обл» съело бы хвост у
// « обл.» и в каталог уехала бы «Ивановская область.» с точкой на конце.
// Срез по длине в байтах безопасен: и кириллица, и латиница занимают в верхнем
// и нижнем регистре одинаковое число байт, поэтому lower совпадает с оригиналом
// по длине.
func expandRegion(region, lower string) string {
	if full, ok := regionExceptions[lower]; ok {
		return full
	}
	for _, e := range regionPrefixExpansions {
		if strings.HasPrefix(lower, e.short) {
			return e.full + region[len(e.short):]
		}
	}
	for _, e := range regionSuffixExpansions {
		if strings.HasSuffix(lower, e.short) {
			return region[:len(region)-len(e.short)] + e.full
		}
	}
	return region
}
