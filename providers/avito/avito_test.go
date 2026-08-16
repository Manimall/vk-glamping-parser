package avito

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Тесты идут по трём НАСТОЯЩИМ страницам, сохранённым владельцем из браузера в
// разные дни (testdata/*.html). Синтетические фикстуры здесь были бы
// самообманом: проверять надо ровно то, что реально отдаёт Авито, — включая
// подменённые буквы в описаниях и разный набор полей у разных объявлений.

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

func TestParseErrors(t *testing.T) {
	t.Run("каталог не задан", func(t *testing.T) {
		if _, err := New("").Parse(context.Background()); err == nil {
			t.Fatal("ожидал ошибку про --input")
		}
	})

	t.Run("каталога нет на диске", func(t *testing.T) {
		if _, err := New(filepath.Join(t.TempDir(), "нет-такого")).Parse(context.Background()); err == nil {
			t.Fatal("ожидал ошибку открытия каталога")
		}
	})

	t.Run("каталог без html", func(t *testing.T) {
		if _, err := New(t.TempDir()).Parse(context.Background()); err == nil {
			t.Fatal("ожидал ошибку про отсутствие .html")
		}
	})

	// Чужая страница не должна ронять сбор целиком — но если разобрать нечего
	// вовсе, это ошибка, а не «успешный» пустой objects.json.
	t.Run("не та страница", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "left.html"), []byte("<html>привет</html>"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := New(dir).Parse(context.Background()); err == nil {
			t.Fatal("ожидал ошибку: ни одна страница не разобрана")
		}
	})

	// Одна кривая копия рядом с рабочими страницами лишь пропускается.
	t.Run("кривой файл пропускается, остальные собираются", func(t *testing.T) {
		dir := t.TempDir()
		src, err := os.ReadFile(filepath.Join("testdata", "furako-muzykant-7826460306.html"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ok.html"), src, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "broken.html"), []byte("<html>мусор</html>"), 0o644); err != nil {
			t.Fatal(err)
		}
		objects, err := New(dir).Parse(context.Background())
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(objects) != 1 || objects[0].SourceID != idFurako {
			t.Fatalf("объекты = %+v", objects)
		}
	})

	t.Run("отмена контекста", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := New("testdata").Parse(ctx); err == nil {
			t.Fatal("ожидал ошибку отмены")
		}
	})
}

// Слаг обязан быть уникальным: по нему строится адрес страницы каталога, а
// «Отдых для двоих» на Авито — название сразу у десятка объявлений.
func TestDedupeSlugs(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile(filepath.Join("testdata", "dlya-dvoih-7862471298.html"))
	if err != nil {
		t.Fatal(err)
	}
	// Копия того же объявления под другим id: подменяем идентификатор по всему
	// файлу, чтобы получить настоящего тёзку (то же название, другой объект),
	// а не дубль, который провайдер отсеет раньше проверки слагов.
	twin := strings.ReplaceAll(string(src), "7862471298", "7999999999")
	for name, data := range map[string]string{"a.html": string(src), "b.html": twin} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := New(dir).Parse(context.Background())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("объектов: %d, ожидалось 2 (тёзки, а не дубль)", len(got))
	}
	if got[0].Slug == got[1].Slug {
		t.Fatalf("слаги совпали: %q", got[0].Slug)
	}
	if !strings.HasPrefix(got[1].Slug, got[0].Slug) {
		t.Errorf("второй слаг %q не производный от первого %q", got[1].Slug, got[0].Slug)
	}
}
