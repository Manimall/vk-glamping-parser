package glamping_rf

import (
	"fmt"
	"html"
	"strings"

	"vk-parser/internal/contract"
	"vk-parser/internal/extract"
	"vk-parser/internal/slug"
)

// amenitiesGroupTitle — заголовок группы удобств в карточке (единый стиль контракта).
const amenitiesGroupTitle = "Удобства"

// toObject маппит объект API глэмпинги.рф в единый контракт contract.Object —
// ровно ту же форму, что отдаёт VK-провайдер (фронту не нужны изменения).
// Чистая функция: тестируется без сети.
func toObject(it apiItem) contract.Object {
	// Листинг отдаёт название с HTML-сущностями («&quot;Веденье&quot;») —
	// detail-страницы декодируются, а этот путь не декодировался, и сущность
	// уезжала в Title и в slug (baza-otdyha-quot-vedene-quot-838, issue #23).
	title := html.UnescapeString(firstNonEmpty(it.NameNew, it.Name))
	region := strings.TrimSpace(it.Place.Name)
	nearCity := strings.TrimSpace(it.City.City)
	location := buildLocation(region, nearCity)
	photos := collectPhotos(it)

	obj := contract.Object{
		// Слаг: транслит имени + id источника — уникален даже при одинаковых
		// названиях («Деревня Ильино» id=959 → "derevnya-ilino-959").
		Slug:     slug.Make(fmt.Sprintf("%s %d", title, it.ID)),
		Title:    title,
		Location: location,
		Region:   region,
		NearCity: nearCity,
		// Locality не заполняем: в списочном ответе источника адресных полей нет
		// вовсе, а City — точка отсчёта расстояния (см. apiCity в types.go).
		// Настоящий населённый пункт лежит только в разметке detail-страницы,
		// свободным текстом и противоречиво — это отдельная задача со своей
		// политикой разбора, а не догадка на ходу.

		HouseTypes:   houseTypes(it.Types),
		Surroundings: surroundings(it.Badges),
		PetsAllowed:  petsAllowed(it.Animal),
		Rating:       it.Rating,
		ReviewsCount: it.ReviewsCount,
		DistanceKm:   distanceKm(it.City.Distance),
		DistanceFrom: strings.TrimSpace(it.City.Unit),
		Highway:      canonHighway(it.City.Highway),
		PriceValue:   it.Price.Value,

		Contact: strings.TrimSpace(it.Telephone),
		MapURL:  strings.TrimSpace(it.Website),
		Cover:   firstNonEmpty(it.ThumbMain.SrcWebp, it.ThumbMain.Src),
		Photos:  photos,
		Cabins: []contract.Cabin{{
			Title: title,
			Price: it.Price.Formatted,
			Property: &extract.Property{
				Title:         title,
				Location:      location,
				PriceFrom:     it.Price.Formatted,
				AmenityGroups: amenityGroups(it.Services),
			},
		}},
	}
	// [Go для изучения] &Тип{...} создаёт значение и сразу берёт указатель на
	// него. Coords в контракте — указатель ради omitempty: nil-указатель JSON
	// опускает поле, а «нулевую» структуру {0,0} опустить бы не смог.
	if it.Lat != 0 || it.Lng != 0 {
		obj.Coords = &contract.Coords{Lat: it.Lat, Lon: it.Lng}
	}
	seo := extract.BuildSEO(extract.SEOInput{Name: title, Location: location, About: obj.About})
	obj.Seo = &seo
	return obj
}

// buildLocation склеивает регион и опорный город в одну строку ДЛЯ ПОКАЗА.
// Обе части ждёт уже отримленными — это делает вызывающий, чтобы поля контракта
// и строка показа гарантированно получились из одних и тех же значений.
//
// Формат намеренно не меняется: на этой строке висят SEO-тексты объектов и
// поиск бота (гость пишет «Москва» и ждёт подмосковные варианты). Структура
// теперь доступна отдельными полями — Region/NearCity, — а укорачивание
// строки до региона возможно только после того, как потребители на них
// перейдут.
func buildLocation(region, nearCity string) string {
	parts := make([]string, 0, 2)
	for _, p := range []string{region, nearCity} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// Слова, которые формой жилья НЕ являются и в типы не берутся.
//
// «Эко-дом» — категория-свалка источника: под ней больше двух третей каталога,
// и раздел из неё собрал бы 69% объектов, перестав что-либо значить.
//
// «Номер» — тип размещения, а не форма дома: комната в корпусе. В нишевом
// каталоге A-frame такой раздел не сужает выбор ничем, а у 13 объектов из 20 он
// оказывался ЕДИНСТВЕННЫМ типом — то есть подменял собой отсутствие данных.
var notHouseTypes = map[string]bool{
	"Эко-дом": true,
	"Номер":   true,
}

// houseTypes — формы жилья объекта. Пустой список означает «источник не
// сказал»: гадать по названию нельзя, «Дом у озера» может быть чем угодно.
//
// Названия канонизируются ЗДЕСЬ, а не только при слиянии с данными
// detail-страницы: та отдаётся не всегда, и при сбое объект уезжал с сырым
// написанием источника. «БарнХаус» рядом с «Барнхаус» — это два раздела на
// одну форму, между которыми делятся объекты.
func houseTypes(types []apiType) []string {
	out := make([]string, 0, len(types))
	seen := make(map[string]bool, len(types))
	for _, t := range types {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			continue
		}
		// Сравниваем ПОСЛЕ канонизации: «Эко дом» и «экодом» пишутся по-разному,
		// а означают одно и то же.
		canon := canonHouseType(name)
		if notHouseTypes[canon] || seen[canon] {
			continue
		}
		seen[canon] = true
		out = append(out, canon)
	}
	if len(out) == 0 {
		return nil // omitempty уберёт поле целиком
	}
	return out
}

// collectPhotos берёт готовые webp-кадры сайта (fallback на src/обложку).
func collectPhotos(it apiItem) []string {
	photos := make([]string, 0, len(it.Images))
	for _, img := range it.Images {
		if u := firstNonEmpty(img.Webp, img.Src); u != "" {
			photos = append(photos, u)
		}
	}
	if len(photos) == 0 {
		if u := firstNonEmpty(it.ThumbMain.SrcWebp, it.ThumbMain.Src); u != "" {
			photos = append(photos, u)
		}
	}
	return photos
}

// amenityGroups превращает услуги объекта в одну группу удобств контракта.
func amenityGroups(services []apiService) []extract.AmenityGroup {
	if len(services) == 0 {
		return nil
	}
	items := make([]string, 0, len(services))
	for _, s := range services {
		if n := strings.TrimSpace(s.Name); n != "" {
			items = append(items, n)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return []extract.AmenityGroup{{Title: amenitiesGroupTitle, Items: items}}
}

// firstNonEmpty — первая непустая (после трима) строка из переданных.
//
// [Go для изучения] vals ...string — вариадик (rest-параметр ...args из JS):
// внутри функции vals — обычный слайс. Вызов с обратной операцией-«spread» —
// f(slice...) — разворачивает слайс в аргументы.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}
