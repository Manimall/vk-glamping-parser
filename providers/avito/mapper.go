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

// photoSize — размер фото, который забираем в галерею. Максимум, который отдаёт
// состояние страницы; меньшие (640×480, 150×110, 75×55) — превью выдачи.
const photoSize = "1280x960"

// blockTagRe — теги, на месте которых в тексте должен остаться разрыв.
// Без этого «</p><p>» склеивает последнее слово абзаца с первым словом
// следующего: «…перезагрузкиМы создали это место…».
var blockTagRe = regexp.MustCompile(`(?i)<(br|/p|/li|/div|/h[1-6])\b[^>]*>`)

// tagRe — любой оставшийся тег.
var tagRe = regexp.MustCompile(`<[^>]*>`)

// reviewsCountRe — число из публичной строки рейтинга («87 отзывов»).
// Разряды у Авито разделены неразрывным пробелом, поэтому цифры могут идти
// группами: «1 234 отзыва».
var reviewsCountRe = regexp.MustCompile(`\d[\d \x{00a0}]*`)

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

	obj := contract.Object{
		Slug:       slug.Make(title),
		SourceID:   it.ID,
		Title:      title,
		About:      aboutText(it),
		Location:   CollapseSpaces(it.Address),
		NearCity:   CollapseSpaces(it.Location.Name),
		PriceValue: it.Price,
		Photos:     photos(it),
		Cover:      cover(it),
		Cabins:     []contract.Cabin{},
		Extras:     extras(it.PriceList),
	}

	if c := it.Geo.Coords; c.Lat != 0 || c.Lng != 0 {
		obj.Coords = &contract.Coords{Lat: c.Lat, Lon: c.Lng}
	}
	if bi.Rating != nil {
		obj.Rating = bi.Rating.ScoreFloat
		obj.ReviewsCount = reviewsCount(bi.Rating)
	}
	return obj
}

// aboutText — описание в виде плоского текста: HTML убран, сущности раскрыты,
// гомоглифы вычищены. DescriptionHtml приоритетнее: там сохранены списки,
// тогда как в Description те же пункты приходят схлопнутыми.
func aboutText(it avitoItem) string {
	src := it.DescriptionHtml
	if strings.TrimSpace(src) == "" {
		src = it.Description
	}
	withBreaks := blockTagRe.ReplaceAllString(src, " ")
	plain := html.UnescapeString(tagRe.ReplaceAllString(withBreaks, ""))
	return CollapseSpaces(NormalizeHomoglyphs(plain))
}

// photos — ссылки на полноразмерные фото в порядке объявления.
//
// Ссылки ведут на CDN Авито (*.img.avito.st) и НЕ скачиваются здесь: провайдер
// вообще не ходит в сеть. Скачивание — отдельный шаг с отдельным решением, и
// принимать его должен владелец, а не разбор сохранённого файла.
func photos(it avitoItem) []string {
	out := make([]string, 0, len(it.ImageUrls))
	for _, sizes := range it.ImageUrls {
		if u := sizes[photoSize]; u != "" {
			out = append(out, u)
		}
	}
	return out
}

// cover — обложка объявления. Авито отмечает её id в listingImage, а сами
// ImageUrls id не несут — сопоставляем по позиции в параллельном списке Images.
// Если списки разошлись (или обложка не отмечена), берём первое фото: пустая
// обложка в каталоге заметнее, чем не та.
func cover(it avitoItem) string {
	all := photos(it)
	if len(all) == 0 {
		return ""
	}
	for i, id := range it.Images {
		if id == it.ListingImage && i < len(all) {
			return all[i]
		}
	}
	return all[0]
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
	digits := reviewsCountRe.FindString(r.Summary)
	digits = strings.NewReplacer(" ", "", " ", "").Replace(digits)
	if n, err := strconv.Atoi(digits); err == nil && n > 0 {
		return n
	}
	return r.ActiveReviewsCount
}
