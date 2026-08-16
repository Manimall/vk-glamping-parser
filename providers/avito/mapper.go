package avito

import (
	"html"
	"regexp"
	"strconv"
	"strings"

	"vk-parser/internal/contract"
	"vk-parser/internal/extract"
	"vk-parser/internal/slug"
)

// Перекладывание объявления Авито в единый контракт каталога.

// blockTagRe — теги, на месте которых в тексте должен остаться разделитель.
// Без этого «</p><p>» склеивает последнее слово абзаца с первым словом
// следующего: «…перезагрузкиМы создали это место…».
var blockTagRe = regexp.MustCompile(`(?i)<(br|/p|/li|/div|/h[1-6])\b[^>]*>`)

// tagRe — любой оставшийся тег.
var tagRe = regexp.MustCompile(`<[^>]*>`)

// repeatedSeparatorRe — цепочка разделителей подряд: её оставляют пустые абзацы
// и двойные переводы строки, из каждого тега получается по «·».
var repeatedSeparatorRe = regexp.MustCompile(`(?:\s*·\s*){2,}`)

// reviewsCountRe — число отзывов из публичной строки рейтинга.
//
// Привязка к слову «отзыв» обязательна: без неё регулярка хватает ПЕРВОЕ число
// строки, и стоит Авито начать отдавать «4,8 · 87 отзывов» — в каталог тихо
// уедет 4 вместо 87. Это не падение, а незаметно неверная цифра, поэтому
// страхуемся заранее. Разряды у Авито разделены неразрывным пробелом
// («1 234 отзыва»), поэтому цифры могут идти группами.
var reviewsCountRe = regexp.MustCompile(`(\d[\d \x{00a0}]*)\s*отзыв`)

// regionSuffixes — окончания первого сегмента адреса, по которым видно, что это
// именно регион, а не сразу город. Адрес Авито — структурное поле источника, но
// первый сегмент у него бывает и «Ивановская обл.», и «Москва».
//
// Форм «Карельская респ.» здесь нет намеренно: республику Авито пишет
// префиксом, а признай мы такую строку регионом — развернуть её словарь бы не
// смог, и в каталог уехала бы вторая ось рядом с «Республика Карелия».
var regionSuffixes = []string{" обл.", " обл", " область", " край", " ао", " аобл"}

// regionSuffixExpansions — словарь сокращений Авито: «Ивановская обл.» и
// «Ивановская область» — одно и то же, и в каталог обязана уехать вторая форма.
//
// Region — ось группировки: каталог, фильтры и мастер подбора бота сводят
// объекты по СОВПАДЕНИЮ строки, а нормализация на сайте (resolveRegion) поле
// источника не трогает — доверяет ему. Отдай мы сокращение при том, что у
// остальных 319 объектов форма полная, — в фильтре появился бы регион-двойник,
// и объекты Авито прятались бы в нём: проверено сквозным прогоном синхронизации
// каталога, было 11 регионов вместо 10.
var regionSuffixExpansions = []struct{ short, full string }{
	{" обл.", " область"},
	{" обл", " область"},
	{" ао", " автономный округ"},
	{" аобл", " автономная область"},
}

// regionPrefixExpansions — сокращения, которые Авито ставит ПЕРЕД названием:
// республики пишутся «Респ. Карелия», а не «Карельская респ.».
var regionPrefixExpansions = []struct{ short, full string }{
	{"респ. ", "Республика "},
	{"респ ", "Республика "},
}

// toObject собирает карточку каталога из объявления.
//
// Чего здесь СОЗНАТЕЛЬНО нет: Surroundings, HouseTypes и PetsAllowed. Оба
// объявления сидят в сервисной категории Авито («Услуги → Праздники,
// мероприятия»), где структурных тегов окружения не существует в принципе —
// набор параметров там про исполнителя: «Опыт работы», «Предоплата»,
// «Аренда снастей». Добывать лес и питомцев разбором описания мы уже пробовали
// на glamping_rf и отказались по цифрам: «в лесу» находилось у 28 объектов
// против 213 по тегам, питомцы — у 43% против 97%. Пустое поле честнее
// выдуманного: владелец дополнит карточку сам, когда ответит.
func toObject(bi *buyerItem) contract.Object {
	it := bi.Item
	title := CollapseSpaces(NormalizeHomoglyphs(it.Title))
	address := CollapseSpaces(it.Address)
	about := aboutText(it)

	obj := contract.Object{
		Slug:       slug.Make(title),
		SourceID:   it.ID,
		Title:      title,
		About:      about,
		Location:   address,
		Region:     region(address),
		NearCity:   CollapseSpaces(it.Location.Name),
		PriceValue: it.Price,
		Photos:     photos(it),
		Cover:      cover(it),
		Cabins:     cabins(title, address, about, it.Price),
		Extras:     extras(it.PriceList),
	}

	if c := it.Geo.Coords; c.Lat != 0 || c.Lng != 0 {
		obj.Coords = &contract.Coords{Lat: c.Lat, Lon: c.Lng}
	}
	if bi.Rating != nil {
		obj.Rating = bi.Rating.ScoreFloat
		obj.ReviewsCount = reviewsCount(bi.Rating)
	}

	// SEO/OG-тексты собираются той же функцией, что у остальных источников:
	// без них ссылка на объект уезжает в мессенджер без описания, а именно
	// ссылками владелец и будет делиться при обходе.
	seo := extract.BuildSEO(extract.SEOInput{Name: title, Location: address, About: about})
	obj.Seo = &seo

	return obj
}

// region — регион из первого сегмента адреса источника.
//
// Это НЕ обратный разбор contract.Location (он запрещён): читаем структурное
// поле address самого Авито, у которого регион всегда идёт первым. Проверяем
// форму записи, потому что у городов федерального значения первый сегмент —
// сразу город («Москва, ул. …»), и подставлять его как регион нельзя: Region в
// каталоге — ось группировки, и город в ней сломает фильтр.
func region(address string) string {
	first, _, found := strings.Cut(address, ",")
	if !found {
		return ""
	}
	first = strings.TrimSpace(first)
	lower := strings.ToLower(first)
	if !looksLikeRegion(lower) {
		return ""
	}
	return expandRegion(first, lower)
}

// looksLikeRegion — первый сегмент назван регионом, а не городом: у области и
// края тип идёт в конце, у республики и округа бывает и в начале.
func looksLikeRegion(lower string) bool {
	for _, suffix := range regionSuffixes {
		if strings.HasSuffix(lower, suffix) {
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
func expandRegion(region, lower string) string {
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

// aboutText — описание в виде плоского текста: HTML убран, сущности раскрыты,
// гомоглифы вычищены. DescriptionHtml приоритетнее: там сохранены списки,
// тогда как в Description те же пункты приходят схлопнутыми.
func aboutText(it avitoItem) string {
	src := it.DescriptionHtml
	if strings.TrimSpace(src) == "" {
		src = it.Description
	}
	// Разделитель «·», а не пробел: пункты списка иначе сливаются в поток слов
	// («Баня Купель Мангал»), и в SEO-описание уезжает каша.
	withBreaks := blockTagRe.ReplaceAllString(src, " · ")
	plain := html.UnescapeString(tagRe.ReplaceAllString(withBreaks, ""))
	return trimSeparators(CollapseSpaces(NormalizeHomoglyphs(plain)))
}

// trimSeparators убирает разделители, оставшиеся после замены тегов.
//
// Схлопывание повторов обязательно, а не для красоты: пустой абзац и двойной
// <br> — обычное дело в объявлениях, и каждый такой тег оставляет по «·».
// Обрезать края поодиночке недостаточно — между разделителями стоят пробелы,
// поэтому cutset берёт и то, и другое.
func trimSeparators(s string) string {
	return strings.Trim(repeatedSeparatorRe.ReplaceAllString(s, " · "), " ·")
}

// extras — платные допы из прайс-листа объявления («Фурако — 5 000 ₽»,
// «Украшение (опция Глинтвейн) — 2 000 ₽»).
//
// Позиции про сам объект («Будние дни/Воскресенье — 8 000 ₽») тоже попадают
// сюда: разделить их автоматически нельзя — у Авито и то и другое лежит одним
// списком без признака. Показать гостю полный прайс честнее, чем угадывать.
func extras(pl *priceList) []extract.Extra {
	if pl == nil {
		return nil
	}
	var out []extract.Extra
	for _, g := range pl.Groups {
		for _, v := range g.Values {
			name := CollapseSpaces(NormalizeHomoglyphs(v.Title))
			if name == "" {
				continue
			}
			out = append(out, extract.Extra{Name: name, Price: CollapseSpaces(v.Price)})
		}
	}
	return out
}

// reviewsCount — число отзывов из публичной строки; при неудаче — внутренний
// счётчик площадки, чтобы поле не осталось нулевым при явно живом рейтинге.
func reviewsCount(r *rating) int {
	m := reviewsCountRe.FindStringSubmatch(r.Summary)
	if len(m) == 2 {
		digits := strings.NewReplacer(" ", "", " ", "").Replace(m[1])
		if n, err := strconv.Atoi(digits); err == nil && n > 0 {
			return n
		}
	}
	return r.ActiveReviewsCount
}
