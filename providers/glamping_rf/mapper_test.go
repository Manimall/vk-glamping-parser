package glamping_rf

import "testing"

func sampleItem() apiItem {
	return apiItem{
		ID:      959,
		Name:    "Ильино",
		NameNew: "Деревня Ильино",
		Images: []apiImage{
			{Src: "https://s/img1.jpg", Webp: "https://s/img1.webp"},
			{Src: "https://s/img2.jpg", Webp: ""}, // без webp → берём src
		},
		ThumbMain: apiThumb{SrcWebp: "https://s/cover.webp"},
		Price:     apiPrice{Value: 7360, Formatted: "7 360 ₽", Per: "night"},
		Place:     apiPlace{ID: 70, Name: "Тульская область"},
		// Живой API отдаёт для id=959 city:null. «Ильино» здесь стояло по ошибке
		// и создавало ложное впечатление, будто city — населённый пункт объекта:
		// по такой фикстуре любой ревьюер сделал бы неверный вывод. Деревня
		// Ильино существует, но приходит из разметки detail-страницы, а не отсюда.
		City:      apiCity{},
		Lat:       54.355987,
		Lng:       37.362846,
		Services:  []apiService{{ID: 11, Name: "Можно с питомцем"}, {ID: 61, Name: "Баня"}},
		Telephone: " +79038410313 ",
		Website:   "https://glamping-ilyino.ru",
	}
}

func TestToObject_Mapping(t *testing.T) {
	o := toObject(sampleItem())

	if o.Title != "Деревня Ильино" { // name_new приоритетнее name
		t.Fatalf("title=%q, ожидал name_new", o.Title)
	}
	// Запирает композицию слага и no-op декода на чистых названиях.
	if o.Slug != "derevnya-ilino" {
		t.Fatalf("slug=%q, ожидал derevnya-ilino (без id, issue #26)", o.Slug)
	}
	if o.SourceID != 959 {
		t.Fatalf("sourceId=%d, ожидал 959 — id переехал из слага в данные", o.SourceID)
	}
	if o.Location != "Тульская область" {
		t.Fatalf("location=%q", o.Location)
	}
	if o.Region != "Тульская область" {
		t.Fatalf("region=%q", o.Region)
	}
	if o.NearCity != "" {
		t.Fatalf("nearCity=%q, источник города не дал", o.NearCity)
	}
	if o.Locality != "" {
		t.Fatalf("locality=%q, населённый пункт из списочного API не берётся", o.Locality)
	}
	if o.Coords == nil || o.Coords.Lat != 54.355987 || o.Coords.Lon != 37.362846 {
		t.Fatalf("coords=%+v (Lng должен маппиться в Lon)", o.Coords)
	}
	if o.Contact != "+79038410313" { // телефон обрезан
		t.Fatalf("contact=%q", o.Contact)
	}
	// Фото: webp где есть, иначе src; обложка не нужна (галерея не пуста).
	if len(o.Photos) != 2 || o.Photos[0] != "https://s/img1.webp" || o.Photos[1] != "https://s/img2.jpg" {
		t.Fatalf("photos=%v", o.Photos)
	}
	if len(o.Cabins) != 1 || o.Cabins[0].Price != "7 360 ₽" {
		t.Fatalf("cabin=%+v", o.Cabins)
	}
	ag := o.Cabins[0].Property.AmenityGroups
	if len(ag) != 1 || len(ag[0].Items) != 2 || ag[0].Items[0] != "Можно с питомцем" {
		t.Fatalf("amenityGroups=%+v", ag)
	}
	if o.Seo == nil || o.Seo.Title == "" {
		t.Fatalf("seo не сгенерирован: %+v", o.Seo)
	}
}

// Гард на исходный баг: опорный город направления НЕ должен попадать в
// населённый пункт объекта. Данные подлинные — объект 1586 стоит в Ярославской
// области, а источник помечает его городом «Москва» (это точка отсчёта: рядом
// в ответе highway «Ярославское» и 154 км от МКАД). Именно такие карточки
// уводили поисковик в неверный город.
//
// Тест обязан покраснеть в ту секунду, когда кто-то решит «город же есть,
// положим его в Locality».
func TestToObject_HubCityIsNotLocality(t *testing.T) {
	it := sampleItem()
	it.ID = 1586
	it.Place = apiPlace{ID: 75, Name: "Ярославская область"}
	it.City = apiCity{City: "Москва", Highway: "Ярославское"}

	o := toObject(it)

	if o.Locality != "" {
		t.Fatalf("locality=%q — опорный город выдан за адрес объекта", o.Locality)
	}
	if o.Region != "Ярославская область" {
		t.Fatalf("region=%q", o.Region)
	}
	if o.NearCity != "Москва" {
		t.Fatalf("nearCity=%q", o.NearCity)
	}
	// Гард совместимости: строка показа менять формат не должна, пока сайт и
	// бот не перешли на раздельные поля. На ней держатся SEO-тексты и поиск
	// бота по названию города.
	if o.Location != "Ярославская область, Москва" {
		t.Fatalf("location=%q — формат строки показа изменился", o.Location)
	}
}

func TestCollectPhotos_FallbackToThumb(t *testing.T) {
	it := apiItem{ThumbMain: apiThumb{SrcWebp: "https://s/cover.webp"}} // images пусты
	got := collectPhotos(it)
	if len(got) != 1 || got[0] != "https://s/cover.webp" {
		t.Fatalf("ожидал обложку-фоллбэк, получил %v", got)
	}
}

func TestBuildLocation(t *testing.T) {
	if got := buildLocation("Тверская область", ""); got != "Тверская область" {
		t.Fatalf("без города: %q", got)
	}
	if got := buildLocation("", "Мышкин"); got != "Мышкин" {
		t.Fatalf("пустой регион: %q", got)
	}
}

// Трим делает toObject, и делает его один раз — иначе поле контракта Region
// и строка показа Location разъедутся на данных с лишними пробелами.
func TestToObject_TrimsAddressOnce(t *testing.T) {
	it := sampleItem()
	it.Place = apiPlace{ID: 49, Name: "  Московская область "}
	it.City = apiCity{City: " Москва  "}

	o := toObject(it)

	if o.Region != "Московская область" {
		t.Errorf("region=%q", o.Region)
	}
	if o.NearCity != "Москва" {
		t.Errorf("nearCity=%q", o.NearCity)
	}
	if o.Location != "Московская область, Москва" {
		t.Errorf("location=%q", o.Location)
	}
}

// Тип дома — верхний уровень навигации каталога: гость нишевого сервиса
// выбирает форму жилья раньше удобств. Источник отдаёт её структурно, но
// парсер поле игнорировал, и на сайте типов не было вовсе.
func TestHouseTypes(t *testing.T) {
	t.Run("берём формы жилья", func(t *testing.T) {
		got := houseTypes([]apiType{{Name: "A-frame"}, {Name: "Купольный дом"}})
		if len(got) != 2 || got[0] != "A-frame" || got[1] != "Купольный дом" {
			t.Fatalf("houseTypes = %v", got)
		}
	})

	// «Эко-дом» стоит у 69% каталога и о форме дома не говорит ничего.
	// Раздел с двумя третями объектов не сужает — значит это не тип.
	t.Run("категорию-свалку отбрасываем", func(t *testing.T) {
		if got := houseTypes([]apiType{{Name: "Эко-дом"}}); got != nil {
			t.Errorf("«Эко-дом» попал в типы: %v", got)
		}
		got := houseTypes([]apiType{{Name: "Эко-дом"}, {Name: "A-frame"}})
		if len(got) != 1 || got[0] != "A-frame" {
			t.Errorf("отбросили лишнее: %v", got)
		}
	})

	// Пустой список должен исчезнуть из JSON целиком (omitempty), а не
	// приехать как []: потребитель отличает «нет типа» от «типов ноль».
	t.Run("нет типов — nil, а не пустой срез", func(t *testing.T) {
		if got := houseTypes(nil); got != nil {
			t.Errorf("ожидали nil, got %#v", got)
		}
		if got := houseTypes([]apiType{{Name: "  "}}); got != nil {
			t.Errorf("пробельное имя не отброшено: %#v", got)
		}
	})

	t.Run("доезжает до контракта", func(t *testing.T) {
		it := sampleItem()
		it.Types = []apiType{{Name: "A-frame"}, {Name: "Эко-дом"}}
		o := toObject(it)
		if len(o.HouseTypes) != 1 || o.HouseTypes[0] != "A-frame" {
			t.Fatalf("HouseTypes = %v", o.HouseTypes)
		}
	})
}

// «Эко-дом» и «Номер» формой жилья не являются: первое — категория-свалка
// источника (под ней две трети каталога), второе — тип размещения, комната в
// корпусе. У 13 объектов из 20 «Номер» был ЕДИНСТВЕННЫМ типом, то есть подменял
// собой отсутствие данных.
func TestHouseTypes_DropsNonForms(t *testing.T) {
	cases := []struct {
		name  string
		types []apiType
		want  []string
	}{
		{"свалка отбрасывается", []apiType{{Name: "Эко-дом"}}, nil},
		{"другое написание тоже", []apiType{{Name: "Эко дом"}, {Name: "экодом"}}, nil},
		{"тип размещения отбрасывается", []apiType{{Name: "Номер"}}, nil},
		{"настоящая форма остаётся", []apiType{{Name: "Эко-дом"}, {Name: "Барнхаус"}}, []string{"Барнхаус"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := houseTypes(c.types)
			if len(got) != len(c.want) {
				t.Fatalf("houseTypes = %v, ожидали %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("houseTypes = %v, ожидали %v", got, c.want)
				}
			}
		})
	}
}
