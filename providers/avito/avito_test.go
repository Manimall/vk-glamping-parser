package avito

import (
	"context"
	"strings"
	"testing"
)

// Тесты идут по данным трёх НАСТОЯЩИХ объявлений, сохранённых владельцем из
// браузера в разные дни. Придумывать значения самому здесь было бы самообманом:
// проверять надо то, что реально отдаёт Авито, — подменённые буквы в описаниях,
// разный набор полей и разъезжающиеся координаты.
//
// Сами файлы testdata/*.html — не копии страниц, а вырезки из них
// (make_fixture.py рядом): полная страница весит мегабайт и несёт имя
// продавца, а репозиторий публичный. Значения полей в вырезке подлинные,
// обёртка пересобрана — поэтому крайние случаи формата (экранирование,
// минификация, обрыв файла) проверяются отдельно, в hydration_test.go.

const (
	idDlyaDvoih = 7862471298 // «Отдых для двоих», Иваново
	idFurako    = 7826460306 // «Отдых с Фурако в Приюте музыканта», д. Коляново
	idLesu      = 8190067783 // «Отдых в лесу с купелью», с. Тюрюково
)

func TestParseRealPages(t *testing.T) {
	objects, err := New("testdata").Parse(context.Background())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(objects) != 3 {
		t.Fatalf("объектов: %d, ожидалось 3", len(objects))
	}

	byID := map[int]int{}
	for i, o := range objects {
		byID[o.SourceID] = i
	}
	for _, id := range []int{idDlyaDvoih, idFurako, idLesu} {
		if _, ok := byID[id]; !ok {
			t.Fatalf("не разобрано объявление %d", id)
		}
	}

	// Страховка от расхождения генератора фикстур с types.go: ITEM_FIELDS в
	// make_fixture.py — второй, независимый список полей. Забудь его дополнить —
	// поле молча придёт нулевым, а тесты остались бы зелёными.
	t.Run("все читаемые поля есть в данных", func(t *testing.T) {
		for _, o := range objects {
			switch {
			case o.Title == "", o.Slug == "", o.SourceID == 0:
				t.Errorf("%d: пустое название/слаг/id", o.SourceID)
			case o.PriceValue == 0:
				t.Errorf("%d: нет цены", o.SourceID)
			case o.Location == "", o.NearCity == "":
				t.Errorf("%d: нет адреса или города", o.SourceID)
			case o.Coords == nil:
				t.Errorf("%d: нет координат", o.SourceID)
			case len(o.Photos) == 0, o.Cover == "":
				t.Errorf("%d: нет фото или обложки", o.SourceID)
			case o.About == "":
				t.Errorf("%d: нет описания", o.SourceID)
			case len(o.Extras) == 0:
				t.Errorf("%d: нет прайс-листа", o.SourceID)
			case o.Rating == 0 || o.ReviewsCount == 0:
				t.Errorf("%d: нет рейтинга", o.SourceID)
			case o.Seo == nil || o.Seo.Title == "":
				t.Errorf("%d: нет SEO-текстов", o.SourceID)
			}
		}
	})

	t.Run("цена, название, слаг", func(t *testing.T) {
		o := objects[byID[idFurako]]
		if o.Title != "Отдых с Фурако в Приюте музыканта" {
			t.Errorf("Title = %q", o.Title)
		}
		if o.PriceValue != 8000 {
			t.Errorf("PriceValue = %d, ожидалось 8000", o.PriceValue)
		}
		if o.Slug != "otdyh-s-furako-v-priyute-muzykanta" {
			t.Errorf("Slug = %q", o.Slug)
		}
	})

	// Координаты берутся из geo (точка объекта), а НЕ из location (город
	// выдачи). У этого объявления они расходятся на 12 км: geo указывает на
	// Тюрюково, location — на Ново-Талицы. Перепутать их значит поставить
	// метку на карте в чужом селе.
	t.Run("координаты — точка объекта, а не города", func(t *testing.T) {
		o := objects[byID[idLesu]]
		if o.Coords == nil {
			t.Fatal("Coords = nil")
		}
		if d := o.Coords.Lat - 57.030023; d > 0.001 || d < -0.001 {
			t.Errorf("Lat = %v, ожидалась точка объекта ~57.030 (Тюрюково)", o.Coords.Lat)
		}
		if d := o.Coords.Lon - 40.663372; d > 0.001 || d < -0.001 {
			t.Errorf("Lon = %v, ожидалась точка объекта ~40.663", o.Coords.Lon)
		}
	})

	t.Run("адрес для показа и опорный город", func(t *testing.T) {
		o := objects[byID[idFurako]]
		if !strings.Contains(o.Location, "Садовая ул., 32") {
			t.Errorf("Location = %q, ожидался адрес с домом", o.Location)
		}
		if o.NearCity != "Иваново" {
			t.Errorf("NearCity = %q", o.NearCity)
		}
	})

	t.Run("галерея в полном размере и обложка из неё", func(t *testing.T) {
		o := objects[byID[idDlyaDvoih]]
		if len(o.Photos) != 10 {
			t.Fatalf("фото: %d, ожидалось 10", len(o.Photos))
		}
		if o.Cover == "" {
			t.Fatal("Cover пустой")
		}
		var coverInGallery bool
		for _, p := range o.Photos {
			if p == o.Cover {
				coverInGallery = true
			}
			if !strings.HasPrefix(p, "https://") {
				t.Errorf("фото не абсолютной ссылкой: %q", p)
			}
		}
		if !coverInGallery {
			t.Error("обложка не из галереи объявления")
		}
	})

	// Рейтинг лежит уровнем выше объявления (buyerItem.rating): у item.rating
	// всегда null. Число отзывов берём из публичной строки «87 отзывов» — её
	// видит гость; внутренний activeReviewsCount там больше (92).
	t.Run("рейтинг продавца и публичное число отзывов", func(t *testing.T) {
		o := objects[byID[idDlyaDvoih]]
		if o.Rating != 4.8 {
			t.Errorf("Rating = %v, ожидалось 4.8", o.Rating)
		}
		if o.ReviewsCount != 87 {
			t.Errorf("ReviewsCount = %d, ожидалось 87 (публичная строка)", o.ReviewsCount)
		}
	})

	t.Run("прайс-лист становится допами", func(t *testing.T) {
		o := objects[byID[idFurako]]
		if len(o.Extras) != 4 {
			t.Fatalf("допов: %d, ожидалось 4", len(o.Extras))
		}
		want := map[string]string{
			"Будние дни/Воскресенье":      "8 000 ₽",
			"Пятница/Суббота":             "9 000 ₽",
			"Фурако":                      "5 000 ₽",
			"Украшение (опция Глинтвейн)": "2 000 ₽",
		}
		for _, e := range o.Extras {
			price, ok := want[e.Name]
			if !ok {
				t.Errorf("неожиданный доп %q", e.Name)
				continue
			}
			if e.Price != price {
				t.Errorf("доп %q: цена %q, ожидалась %q", e.Name, e.Price, price)
			}
		}
	})

	// Подменённые буквы обязаны быть вычищены: иначе поиск по каталогу не найдёт
	// «пространство», а адрес страницы соберётся из латиницы вперемешку.
	t.Run("описание очищено от подменённых букв", func(t *testing.T) {
		o := objects[byID[idFurako]]
		if o.About == "" {
			t.Fatal("About пустой")
		}
		if !strings.Contains(o.About, "Пространство для тишины") {
			t.Errorf("подмены не вычищены, About начинается с: %.120q", o.About)
		}
		if strings.Contains(o.About, "<") {
			t.Errorf("в About осталась разметка: %.120q", o.About)
		}
	})

	// Слепков окружения у сервисной категории Авито нет, а выдумывать их
	// разбором описания мы уже пробовали на glamping_rf и отказались по цифрам.
	t.Run("окружение и питомцы не выдуманы", func(t *testing.T) {
		for _, o := range objects {
			if len(o.Surroundings) != 0 {
				t.Errorf("%d: Surroundings = %v, ожидалось пусто", o.SourceID, o.Surroundings)
			}
			if len(o.HouseTypes) != 0 {
				t.Errorf("%d: HouseTypes = %v, ожидалось пусто", o.SourceID, o.HouseTypes)
			}
			if o.PetsAllowed != nil {
				t.Errorf("%d: PetsAllowed = %v, ожидалось nil", o.SourceID, *o.PetsAllowed)
			}
		}
	})

	// Телефона нет физически: Авито отдаёт только маску, а сам номер показывает
	// картинкой. Пустой контакт — правда, а не недоработка разбора.
	t.Run("контакт пустой", func(t *testing.T) {
		for _, o := range objects {
			if o.Contact != "" {
				t.Errorf("%d: Contact = %q, ожидалось пусто", o.SourceID, o.Contact)
			}
		}
	})
}
