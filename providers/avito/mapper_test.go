package avito

import "testing"

// Ветки маппинга, которых нет в трёх сохранённых страницах: у них рейтинг,
// координаты и прайс на месте, а галерея ровная. Именно поэтому такие случаи
// проверяются отдельно — иначе первое же нестандартное объявление сломало бы
// каталог молча, без единой ошибки в логах.

// Ссылки-заглушки: содержимое неважно, важна привязка к id кадра.
func urls(links ...string) []map[string]string {
	out := make([]map[string]string, 0, len(links))
	for _, l := range links {
		if l == "" {
			// Кадр, у которого CDN ещё не отдал крупный размер.
			out = append(out, map[string]string{"640x480": "small"})
			continue
		}
		out = append(out, map[string]string{photoSize: l})
	}
	return out
}

func TestCover(t *testing.T) {
	t.Run("обложка по отметке источника", func(t *testing.T) {
		it := avitoItem{
			ImageUrls:    urls("a", "b", "c"),
			Images:       []int64{10, 20, 30},
			ListingImage: 20,
		}
		if got := cover(it); got != "b" {
			t.Errorf("cover = %q, ожидалось b", got)
		}
	})

	// Тот самый рассинхрон: у первого кадра нет крупного размера, он выпадает
	// из галереи. Поиск по позиции взял бы соседа — обложкой стало бы не то
	// фото, что видно в выдаче Авито.
	t.Run("кадр без крупного размера не сдвигает обложку", func(t *testing.T) {
		it := avitoItem{
			ImageUrls:    urls("", "b", "c"),
			Images:       []int64{10, 20, 30},
			ListingImage: 30,
		}
		if got := cover(it); got != "c" {
			t.Errorf("cover = %q, ожидалось c", got)
		}
	})

	t.Run("отметки нет — первое фото", func(t *testing.T) {
		it := avitoItem{ImageUrls: urls("a", "b"), Images: []int64{10, 20}, ListingImage: 999}
		if got := cover(it); got != "a" {
			t.Errorf("cover = %q, ожидалось a", got)
		}
	})

	t.Run("список id короче галереи", func(t *testing.T) {
		it := avitoItem{ImageUrls: urls("a", "b"), Images: []int64{10}, ListingImage: 10}
		if got := cover(it); got != "a" {
			t.Errorf("cover = %q, ожидалось a", got)
		}
	})

	t.Run("фото нет вовсе", func(t *testing.T) {
		if got := cover(avitoItem{}); got != "" {
			t.Errorf("cover = %q, ожидалась пустая строка", got)
		}
	})
}

func TestReviewsCount(t *testing.T) {
	cases := []struct {
		name    string
		summary string
		active  int
		want    int
	}{
		{"публичная строка", "87 отзывов", 92, 87},
		{"разряды неразрывным пробелом", "1 234 отзыва", 9, 1234},
		{"один отзыв", "1 отзыв", 5, 1},

		// Страховка на будущее: стоит Авито добавить оценку в строку, и
		// непривязанная регулярка утащила бы в каталог 4 вместо 87 — молча.
		{"оценка перед числом", "4,8 · 87 отзывов", 92, 87},
		{"оценка словами", "Рейтинг 4.8, 87 отзывов", 92, 87},

		{"строки нет — внутренний счётчик", "", 92, 92},
		{"чисел нет — внутренний счётчик", "нет отзывов", 42, 42},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := reviewsCount(&rating{Summary: c.summary, ActiveReviewsCount: c.active})
			if got != c.want {
				t.Errorf("reviewsCount(%q, %d) = %d, ожидалось %d", c.summary, c.active, got, c.want)
			}
		})
	}
}

// Объявление без рейтинга, координат и прайса: все три блока у Авито
// необязательные, и ни один из них не должен ронять разбор.
func TestToObjectBareItem(t *testing.T) {
	obj := toObject(&buyerItem{Item: avitoItem{ID: 5, Title: "Дом", Price: 3000}})

	if obj.Rating != 0 || obj.ReviewsCount != 0 {
		t.Errorf("рейтинг без данных: %v / %d", obj.Rating, obj.ReviewsCount)
	}
	if obj.Coords != nil {
		t.Errorf("Coords = %+v, ожидался nil при нулевых координатах", obj.Coords)
	}
	if len(obj.Extras) != 0 {
		t.Errorf("Extras = %v, ожидалось пусто", obj.Extras)
	}
	if obj.Photos == nil || obj.Cabins == nil {
		t.Error("Photos и Cabins обязаны быть непустыми срезами: в JSON уйдёт null вместо []")
	}
	if obj.Slug != "dom" {
		t.Errorf("Slug = %q", obj.Slug)
	}
	// SEO собирается всегда: ссылкой на объект владелец делится в переписке.
	if obj.Seo == nil || obj.Seo.Title == "" {
		t.Errorf("Seo = %+v, ожидался заполненный", obj.Seo)
	}
}

// Описание с разметкой: списки не должны схлопываться в поток слов.
func TestAboutTextKeepsListItems(t *testing.T) {
	it := avitoItem{DescriptionHtml: "<ul><li>Баня</li><li>Купель</li><li>Мангал</li></ul>"}
	got := aboutText(it)
	want := "Баня · Купель · Мангал"
	if got != want {
		t.Errorf("aboutText = %q, ожидалось %q", got, want)
	}
}

// При пустом descriptionHtml берётся description — у двух из трёх реальных
// объявлений первое поле отсутствует вовсе.
func TestAboutTextFallsBackToPlainDescription(t *testing.T) {
	it := avitoItem{Description: "<p>Дoмик в лecу</p>"}
	if got := aboutText(it); got != "Домик в лесу" {
		t.Errorf("aboutText = %q", got)
	}
}

// Пустые абзацы и двойные <br> — обычная разметка объявления, и каждый такой
// тег оставляет по разделителю. В трёх сохранённых страницах их не оказалось
// случайно; уехали бы они и в About, и в OG-описание ссылки.
func TestAboutTextDropsSeparatorRuns(t *testing.T) {
	cases := []struct{ name, html, want string }{
		{"пустой абзац в конце", "<p>Текст</p><p><br></p>", "Текст"},
		{"два пустых абзаца", "<p>Текст</p><p></p><p></p>", "Текст"},
		{"разделители с обеих сторон", "<div><br/><br/>Текст<br/><br/></div>", "Текст"},
		{"пустой абзац в середине", "<p>Баня</p><p></p><p>Купель</p>", "Баня · Купель"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := aboutText(avitoItem{DescriptionHtml: c.html}); got != c.want {
				t.Errorf("aboutText = %q, ожидалось %q", got, c.want)
			}
		})
	}
}
