package avito

import (
	"html"
	"log/slog"
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
	// Сокращение региона разворачивается и в адресе показа: Region и Location
	// обязаны начинаться одинаково, иначе плитка каталога перестаёт сокращать
	// строку места и печатает адрес целиком (см. canonAddress).
	// Адрес чистится от подменённых букв наравне с заголовком: латинская «a» в
	// «Ивaновская обл.» дала бы регион-двойник — ровно то, от чего защищает
	// словарь ниже.
	address := publicAddress(canonAddress(CollapseSpaces(NormalizeHomoglyphs(it.Address))))
	about := aboutText(it)

	obj := contract.Object{
		Slug:       slug.Make(title),
		SourceID:   it.ID,
		Title:      title,
		About:      about,
		Location:   address,
		Region:     objectRegion(it.ID, address),
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

// objectRegion — регион объекта с жалобой в лог, если форма записи незнакома.
//
// Молчать тут нельзя: пустой регион синк не пропускает — он подставляет догадку
// по адресу, и в ось группировки уезжает либо чужой субъект, либо строка
// целиком. Ни то ни другое проверкой «регионов не стало больше» не ловится.
// В лог идёт только первый сегмент: остальное — адрес человека.
func objectRegion(sourceID int, address string) string {
	r := region(address)
	if r == "" {
		first, _, _ := strings.Cut(address, ",")
		slog.Warn("avito: регион не распознан, каталог подставит догадку по адресу",
			"sourceId", sourceID, "первыйСегмент", strings.TrimSpace(first))
	}
	return r
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
