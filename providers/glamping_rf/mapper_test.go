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
		City:      apiCity{City: "Ильино"},
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
	if o.Location != "Тульская область, Ильино" {
		t.Fatalf("location=%q", o.Location)
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
	if got := buildLocation("  ", "Мышкин"); got != "Мышкин" {
		t.Fatalf("пустой регион: %q", got)
	}
}
