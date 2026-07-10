package glamping_rf

import (
	"strings"

	"vk-parser/internal/contract"
	"vk-parser/internal/extract"
)

// amenitiesGroupTitle — заголовок группы удобств в карточке (единый стиль контракта).
const amenitiesGroupTitle = "Удобства"

// toObject маппит объект API глэмпинги.рф в единый контракт contract.Object —
// ровно ту же форму, что отдаёт VK-провайдер (фронту не нужны изменения).
// Чистая функция: тестируется без сети.
func toObject(it apiItem) contract.Object {
	title := firstNonEmpty(it.NameNew, it.Name)
	location := buildLocation(it.Place.Name, it.City.City)
	photos := collectPhotos(it)

	obj := contract.Object{
		Title:    title,
		Location: location,
		Contact:  strings.TrimSpace(it.Telephone),
		MapURL:   strings.TrimSpace(it.Website),
		Photos:   photos,
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
	if it.Lat != 0 || it.Lng != 0 {
		obj.Coords = &contract.Coords{Lat: it.Lat, Lon: it.Lng}
	}
	seo := extract.BuildSEO(extract.SEOInput{Name: title, Location: location, About: obj.About})
	obj.Seo = &seo
	return obj
}

// buildLocation склеивает регион и город (если задан) в одну строку локации.
func buildLocation(place, city string) string {
	parts := make([]string, 0, 2)
	for _, p := range []string{strings.TrimSpace(place), strings.TrimSpace(city)} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
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
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}
