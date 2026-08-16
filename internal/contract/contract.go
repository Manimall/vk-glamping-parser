// Package contract — единый формат карточки объекта, который отдают ВСЕ провайдеры
// (VK, glamping_rf, …) и который потребляет фронтенд. Один контракт на все
// источники: добавление нового провайдера не требует правок на фронте.
//
// Раньше эти типы жили в пакете main (app) под именем GlampingData. Вынесены сюда,
// чтобы их могли использовать изолированные провайдеры (providers/*), не завязываясь
// на main. app держит на них псевдонимы (type GlampingData = contract.Object).
package contract

import (
	"strconv"
	"strings"

	"vk-parser/internal/extract"
)

// Coords — гео-координаты объекта. Указатель у владельца, чтобы omitempty мог их
// «выкинуть»: у структуры-значения нет понятия «пустая», а nil-указатель уберётся.
type Coords struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Cabin — ОДИН домик объекта (A-frame, купольный шатёр и т.п.): своя цена и своё
// структурированное описание (Property — что именно в нём есть).
type Cabin struct {
	Title       string            `json:"title"`
	Price       string            `json:"price,omitempty"`
	Description string            `json:"description,omitempty"`
	Property    *extract.Property `json:"property,omitempty"`
	// Variants — заголовки почти-одинаковых домиков, схлопнутых в этот (дедуп).
	Variants []string `json:"variants,omitempty"`
}

// Object — карточка объекта размещения: ОБЪЕКТ-уровень (название, локация,
// галерея) + список домиков. Единый JSON-контракт для фронта. omitempty убирает
// поля, которых нет.
type Object struct {
	Slug string `json:"slug,omitempty"` // URL-идентификатор (/api/v1/glampings/<slug>)
	// SourceID — id объекта у источника. Из слага id убран (issue #26), а связь
	// с источником обязана жить: стыковка объектов между прогонами, фильтры
	// «без новых»; слаги тёзок строятся из этого id.
	SourceID int    `json:"sourceId,omitempty"`
	Title    string `json:"title,omitempty"` // название объекта
	About    string `json:"about,omitempty"` // описание

	// Location — человекочитаемая строка ДЛЯ ПОКАЗА. Разбирать её обратно на
	// составляющие нельзя: для этого есть Region/Locality/NearCity ниже.
	// Искать по ней подстрокой можно — запрет только на разбор.
	//
	// Именно разбор этой строки и был багом: у glamping_rf она склеена как
	// «регион, опорный город», и потребители принимали второе за адрес объекта.
	Location string `json:"location,omitempty"`

	// Region — регион/направление верхнего уровня, как его называет источник.
	// НЕ гарантированно субъект РФ: у глэмпинги.рф встречаются «Архыз
	// (Карачаево-Черкесия)» и «Абхазия». Гарантия ровно одна, и её достаточно —
	// это никогда не населённый пункт.
	Region string `json:"region,omitempty"`

	// Locality — населённый пункт САМОГО объекта. Заполняется, только если
	// источник его действительно сообщил: ни регион, ни опорный город, ни шоссе
	// населённым пунктом не считаются. Пустое поле честнее чужого города —
	// потребитель тогда не покажет ничего вместо неверного.
	Locality string `json:"locality,omitempty"`

	// NearCity — опорный город направления, точка ОТКУДА ехать, а не адрес.
	// Почему это именно точка отсчёта и чем доказано — см. комментарий к
	// apiCity в providers/glamping_rf/types.go.
	NearCity string `json:"nearCity,omitempty"`

	// HouseTypes — форма жилья: «A-frame», «Купольный дом», «Барнхаус».
	// Не удобство, а КАТЕГОРИЯ: гость нишевого каталога выбирает форму дома
	// раньше, чем удобства внутри — «хочу пожить в куполе» это законченное
	// желание, а «хочу мангал» нет.
	//
	// Список, а не строка: у объекта бывает несколько корпусов разной формы.
	HouseTypes []string `json:"houseTypes,omitempty"`

	// Surroundings — что вокруг: лес, река, озеро, горнолыжный склон.
	// Приходит тегами от источника, а не разбором описания: в тексте «Лесное
	// озеро» может оказаться названием объекта, а не водоёмом рядом.
	Surroundings []string `json:"surroundings,omitempty"`

	// PetsAllowed — можно ли с питомцем, по данным источника. Указатель, чтобы
	// отличить «нельзя» (false) от «источник не сказал» (nil): гостю с собакой
	// это разные ответы, и молчание нельзя показывать как запрет.
	PetsAllowed *bool `json:"petsAllowed,omitempty"`

	// Rating — средняя оценка и число отзывов. Числом, а не строкой «4.8 · 129
	// отзывов»: по строке нельзя ни отсортировать каталог, ни отфильтровать.
	Rating       float64 `json:"rating,omitempty"`
	ReviewsCount int     `json:"reviewsCount,omitempty"`

	// DistanceKm — сколько километров ехать, DistanceFrom — ОТКУДА их считать,
	// Highway — по какому шоссе. Вместе отвечают на вопрос, с которого
	// начинается подмосковный выбор: «далеко ли и в какую сторону».
	//
	// Точка отсчёта обязательна рядом с числом: источник меряет то от МКАД, то
	// от города, и в Нижегородской области расстояния лежат в диапазоне 1–151 км
	// при том, что до Нижнего от Москвы четыреста. Одно поле «сколько км» без
	// второго складывает несравнимое, и фильтр «до 100 км» врал бы.
	DistanceKm   int    `json:"distanceKm,omitempty"`
	DistanceFrom string `json:"distanceFrom,omitempty"`
	Highway      string `json:"highway,omitempty"`

	// PriceValue — цена числом. Строка Cabins[0].Price («7 360 ₽») остаётся
	// для показа, но сортировать и фильтровать по ней нельзя.
	PriceValue int `json:"priceValue,omitempty"`

	// GuestsMax — сколько гостей помещается в самый вместительный домик.
	// Числом, а не строкой «до 6» внутри facts: «нас шестеро» — самый ходовой
	// фильтр каталога, и разбирать его регуляркой на стороне сайта значило бы
	// повторить ту же ошибку, из-за которой цена и рейтинг ехали строками.
	GuestsMax int `json:"guestsMax,omitempty"`

	Coords  *Coords         `json:"coords,omitempty"`  // координаты (если есть)
	MapURL  string          `json:"mapUrl,omitempty"`  // ссылка на карту
	Contact string          `json:"contact,omitempty"` // телефон/контакт
	Cover   string          `json:"cover,omitempty"`   // обложка-превью (карточка главной, OG)
	Photos  []string        `json:"photos"`            // галерея (URL кадров)
	Cabins  []Cabin         `json:"cabins"`            // домики с удобствами
	Extras  []extract.Extra `json:"extras,omitempty"`  // доп.услуги объекта
	Seo     *extract.SEO    `json:"seo,omitempty"`     // SEO/OG-тексты (без бренда фронта)
}

// Preview — облегчённая карточка для списков (главная страница каталога):
// только то, что нужно отрисовать плитку — без галереи, удобств и правил.
type Preview struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Location string `json:"location,omitempty"`
	// Region — ось группировки: по нему фильтруется каталог и работает мастер
	// подбора бота. Locality/NearCity в плитку не кладём — они нужны только на
	// карточке объекта, а Preview намеренно облегчённый.
	Region string `json:"region,omitempty"`
	Cover  string `json:"cover,omitempty"`
	Price  string `json:"price,omitempty"` // цена первого домика («7 000 ₽»)

	// Поля фильтров. Каталог фильтруется на клиенте по загруженному списку, а
	// список — это Preview: чего здесь нет, по тому и не отфильтруешь. Отсюда
	// исключение из правила «Preview облегчённая»: эти поля не для показа в
	// плитке, а ради того, чтобы фильтр вообще стал возможен.
	//
	// Взято ровно то, что реально сужает выборку (охват на 309 объектах):
	// вместимость 305, окружение 294, цена 307, питомцы 301, рейтинг 275.
	// houseTypes сюда НЕ берём: фильтр по форме дома отложен, а охват 133 из 309
	// — больше половины каталога молчит. Сверить:
	//   jq '[.[]|select(.houseTypes)]|length' generated/glamping_rf/objects.json
	Surroundings []string `json:"surroundings,omitempty"`
	PetsAllowed  *bool    `json:"petsAllowed,omitempty"`
	GuestsMax    int      `json:"guestsMax,omitempty"`
	PriceValue   int      `json:"priceValue,omitempty"`
	Rating       float64  `json:"rating,omitempty"`

	Seo *extract.SEO `json:"seo,omitempty"` // OG-тексты для шаринга ссылки
}

// ToPreview собирает превью из полной карточки (цена — у первого домика).
//
// [Go для изучения] Ресивер (o Object) БЕЗ звёздочки — метод получает КОПИЮ
// структуры: читать можно, а мутировать оригинал — нет, что здесь и нужно
// (чистое преобразование). Указательный ресивер (o *Object) брали бы для
// мутаций или чтобы не копировать крупную структуру.
func (o Object) ToPreview() Preview {
	p := Preview{
		Slug: o.Slug, Title: o.Title, Location: o.Location, Region: o.Region,
		Cover: o.Cover, Seo: o.Seo,

		Surroundings: o.Surroundings,
		PetsAllowed:  o.PetsAllowed,
		GuestsMax:    o.GuestsMax,
		PriceValue:   o.PriceValue,
		Rating:       o.Rating,
	}
	if len(o.Cabins) > 0 {
		p.Price = o.Cabins[0].Price
	}
	// Источник без разбивки по домикам (Авито: объявление — это объект целиком)
	// оставил бы плитку каталога вовсе без цены, хотя PriceValue заполнен и
	// фильтры по нему работают. Собираем строку из числа.
	if p.Price == "" && o.PriceValue > 0 {
		p.Price = FormatPrice(o.PriceValue)
	}
	return p
}

// FormatPrice — цена для показа: «6000» → «6 000 ₽». Разряды разделены
// неразрывным пробелом, чтобы вёрстка не переносила строку между «6» и «000».
func FormatPrice(value int) string {
	digits := strconv.Itoa(value)
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteString(" ")
		}
		b.WriteRune(r)
	}
	b.WriteString(" ₽")
	return b.String()
}
