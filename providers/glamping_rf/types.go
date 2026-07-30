package glamping_rf

// Структуры ответа внутреннего JSON-API глэмпинги.рф (OpenCart):
//   GET index.php?route=product/category/list&path=82&place=<id>&page=<N>
//   заголовок X-Requested-With: XMLHttpRequest
// Держим только поля, которые реально используем в маппинге на contract.Object.
//
// [Go для изучения] Видимость решает РЕГИСТР первой буквы: apiItem со строчной —
// приватен для пакета (наружу не экспортируется), Object с заглавной в contract —
// публичен. Это единственный механизм public/private в Go, никаких ключевых слов.
// Бэктик-теги `json:"name_new"` говорят encoding/json, как маппить snake_case
// поля JSON на Go-имена — аналог описания формы ответа в TS-интерфейсе, только
// теги ещё и управляют (де)сериализацией в рантайме.

// apiResponse — страница выдачи каталога.
type apiResponse struct {
	Items   []apiItem `json:"items"`
	Total   int       `json:"total"`
	Page    int       `json:"page"`
	Limit   int       `json:"limit"`
	HasMore bool      `json:"has_more"`
}

// apiItem — один объект каталога (глэмпинг).
type apiItem struct {
	ID        int          `json:"id"`
	Name      string       `json:"name"`
	NameNew   string       `json:"name_new"` // «красивое» имя, если задано
	Href      string       `json:"href"`
	Images    []apiImage   `json:"images"`
	ThumbMain apiThumb     `json:"thumb_main"`
	Price     apiPrice     `json:"price"`
	Place     apiPlace     `json:"place"`
	City      apiCity      `json:"city"`
	Lat       float64      `json:"lat"`
	Lng       float64      `json:"lng"`
	Services  []apiService `json:"services"`
	Types     []apiType    `json:"types"`
	Website   string       `json:"website"`
	Telephone string       `json:"telephone"`

	// Ниже — поля, которые источник отдаёт структурно, а мы прежде добывали
	// разбором описания через LLM или не добывали вовсе. Структурный признак
	// надёжнее: он не галлюцинирует и покрывает почти весь каталог.

	// Badges — теги окружения: forest, river, lake, ski (плюс служебный bnovo).
	// Заполнены у всех объектов; «в лесу» стоит у двух третей, «река» и
	// «озеро» — у 41% каждый. Прежде эти признаки вытягивались из текста
	// описания, где «озеро» может оказаться названием, а не водоёмом.
	Badges []string `json:"badges"`

	// Animal — «yes» / «no»: можно ли с питомцем. Заполнено у 97% объектов
	// против 43%, которые давал разбор описания.
	Animal string `json:"animal"`

	// Rating — средняя оценка (4.845), ReviewsCount — сколько отзывов.
	// Заполнены у 89%. Число, а не строка «4.8 · 129 отзывов», которую
	// собирает обогащение detail-страницей: по строке нельзя ни отсортировать,
	// ни отфильтровать.
	Rating       float64 `json:"rating"`
	ReviewsCount int     `json:"reviews_count"`
}

// apiImage — кадр галереи: сайт уже отдаёт готовый webp.
type apiImage struct {
	Src  string `json:"src"`
	Webp string `json:"webp"`
}

type apiThumb struct {
	Src     string `json:"src"`
	SrcWebp string `json:"src_webp"`
}

type apiPrice struct {
	Value     int    `json:"value"`
	Formatted string `json:"formatted"` // «7 360 ₽»
	Per       string `json:"per"`       // «night»
}

// apiPlace — регион/направление верхнего уровня. Не всегда субъект РФ
// (встречаются «Архыз (Карачаево-Черкесия)» и «Абхазия»), но НИКОГДА не
// населённый пункт: на 60 значений источника нет ни одного города.
type apiPlace struct {
	ID   int    `json:"id"`
	Name string `json:"name"` // «Ярославская область»
}

// apiCity — ТОЧКА ОТСЧЁТА РАССТОЯНИЯ, а не место объекта. В ответе рядом с
// city лежат distance и distance_measurement_type: «154» + «км от МКАД»
// (встречаются также «км от КАД», «км от города», «км от аэропорта»).
//
// Читать City как адрес — исходный баг этого модуля: у 203 подмосковных
// объектов там «Москва», и склеенная строка уводила потребителей в неверный
// населённый пункт. Показательный случай — объект 1586: он в Ярославской
// области, а city=«Москва», highway=«Ярославское», distance=«154».
// Поэтому City маппится в NearCity, и никогда — в Locality.
//
// Поля бывают null (json → пустая строка). Если понадобится distance —
// объявлять ТОЛЬКО string: источник шлёт число строкой, и int уронит разбор
// всей страницы разом.
type apiCity struct {
	City    string `json:"city"`
	Highway string `json:"highway"`
	// Distance — расстояние от опорного города ЧИСЛОМ В СТРОКЕ («70»).
	// Именно строкой: объявить int значит уронить разбор всей страницы разом,
	// а с ним и весь регион. Unit поясняет, от чего меряли.
	Distance string `json:"distance"`
	Unit     string `json:"distance_measurement_type"` // «км от МКАД», «км от города»
}

type apiService struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type apiType struct {
	ID   int    `json:"id"`
	Name string `json:"name"` // «Эко-дом»
	Slug string `json:"slug"`
}
