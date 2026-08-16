package avito

// Формат состояния Авито — только те поля, которые реально используются.
//
// Состояние страницы весит около мегабайта; описывать его целиком незачем и
// вредно: чем больше полей, тем чаще разбор ломается на изменениях площадки.
// encoding/json молча игнорирует лишние ключи входа, поэтому узкая структура
// переживает релизы Авито, пока живы нужные нам поля.

// buyerItem — ветка объявления. Рейтинг лежит ЗДЕСЬ, а не внутри item: он
// принадлежит продавцу, а не объявлению (у item.rating всегда null — проверено
// на всех трёх страницах).
type buyerItem struct {
	Item   avitoItem `json:"item"`
	Rating *rating   `json:"rating"`
}

type rating struct {
	ScoreFloat float64 `json:"scoreFloat"`
	// Summary — публичная строка «87 отзывов». Именно это число видит гость на
	// странице; activeReviewsCount бывает больше (92) — там своя внутренняя
	// арифметика площадки, и подставлять её в каталог значит расходиться с
	// первоисточником у всех на глазах.
	Summary            string `json:"summary"`
	ActiveReviewsCount int    `json:"activeReviewsCount"`
}

type avitoItem struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Price int    `json:"price"`
	// Address — адрес, как его показывает Авито. Точность плавает от объявления
	// к объявлению: «д. Коляново, Садовая ул., 32» против «Иваново, пр-т Ленина»
	// без дома. Годится для показа, не годится для разбора на составляющие.
	Address string `json:"address"`

	// Description — описание в HTML. Description vs DescriptionHtml: у одной из
	// трёх страниц заполнено второе поле с нормальной разметкой (<ul>/<li>), у
	// двух других его нет вовсе. Берём то, что есть, приоритет — DescriptionHtml.
	Description     string `json:"description"`
	DescriptionHtml string `json:"descriptionHtml"`

	// Geo — точка САМОГО объекта, Location — город выдачи. Их нельзя путать:
	// у «Отдыха в лесу с купелью» geo указывает на Тюрюково (57.030, 40.663), а
	// location — на Ново-Талицы (57.007, 40.855), это разные места в 12 км.
	Geo      geoBlock      `json:"geo"`
	Location locationBlock `json:"location"`

	// ImageUrls — по объекту на фото, в каждом четыре размера. ListingImage —
	// id обложки; сопоставляется с Images по позиции (сами ImageUrls id не несут).
	ImageUrls    []map[string]string `json:"imageUrls"`
	Images       []int64             `json:"images"`
	ListingImage int64               `json:"listingImage"`

	PriceList *priceList `json:"priceList"`

	// Phone приходит всегда, но публичного номера в нём нет: publicPhone и
	// realPhone — null, есть только маска «8 909 XXX-XX-XX». Это не ограничение
	// разбора: сам Авито показывает номер картинкой и не даёт скопировать его
	// даже вручную. Контакт в каталоге заполняется после переписки с владельцем.
	Phone *phoneBlock `json:"phone"`

	Seo seoBlock `json:"seo"`
}

type geoBlock struct {
	Coords coords `json:"coords"`
}

type locationBlock struct {
	Coords coords `json:"coords"`
	Name   string `json:"name"`
}

// coords — у Авито долгота названа lng (в контракте каталога — Lon).
type coords struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type phoneBlock struct {
	Mask string `json:"mask"`
}

type seoBlock struct {
	CanonicalURL string `json:"canonicalUrl"`
}

// priceList — «Прайс-лист» объявления: у одного объекта одна позиция («Купель —
// 4 000 ₽»), у другого четыре, включая цену по дням недели. Из-за этого поле
// price объявления — это «от», а не единственная цена.
type priceList struct {
	Groups []priceGroup `json:"groups"`
}

type priceGroup struct {
	Values []priceValue `json:"values"`
}

type priceValue struct {
	Title string `json:"title"`
	Price string `json:"price"`
}
