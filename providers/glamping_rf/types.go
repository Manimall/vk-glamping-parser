package glamping_rf

// Структуры ответа внутреннего JSON-API глэмпинги.рф (OpenCart):
//   GET index.php?route=product/category/list&path=82&place=<id>&page=<N>
//   заголовок X-Requested-With: XMLHttpRequest
// Держим только поля, которые реально используем в маппинге на contract.Object.

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

type apiPlace struct {
	ID   int    `json:"id"`
	Name string `json:"name"` // «Ярославская область»
}

// apiCity — уточнение локации. Поля бывают null (json → пустая строка).
type apiCity struct {
	City    string `json:"city"`
	Highway string `json:"highway"`
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
