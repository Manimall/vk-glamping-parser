package glamping_rf

import (
	"reflect"
	"testing"
)

// Гость видит этот список в карточке объекта. Рубрика «Дети» среди бани и
// рыбалки читается как удобство под названием «Дети» — и обесценивает весь
// блок: раз тут мусор, значит и остальному верить нельзя.
func TestCleanAmenitiesDropsRubrics(t *testing.T) {
	got := cleanAmenities([]string{
		"Мангальная зона", "Территория", "Рыбалка", "Развлечения",
		"Дети", "Баня", "Общие", "Общее", "Прочее",
	})
	want := []string{"Мангальная зона", "Рыбалка", "Баня"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("рубрики не отсеяны:\n got %v\nwant %v", got, want)
	}
}

// «Нельзя» — значение рубрики «можно ли с питомцем». Без вопроса оно
// превращается в удобство с названием «Нельзя»; сам ответ уже разобран
// в поле petsAllowed.
func TestCleanAmenitiesDropsOrphanValues(t *testing.T) {
	got := cleanAmenities([]string{"Домашние животные", "Нельзя", "Парковка"})
	want := []string{"Парковка"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("осиротевшие значения не отсеяны:\n got %v\nwant %v", got, want)
	}
}

// «Можно только с согласованием» — не сирота, а единственный носитель смысла:
// у обоих объектов с этой строкой petsAllowed=null. Убрав её, гость вместо
// «можно по согласованию» получил бы молчание.
func TestCleanAmenitiesKeepsConditionalPets(t *testing.T) {
	got := cleanAmenities([]string{"Домашние животные", "Можно только с согласованием"})
	want := []string{"Можно только с согласованием"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("условие про питомцев потеряно:\n got %v\nwant %v", got, want)
	}
}

// Рубрика мероприятий приходит в хвосте, рядом с её же конкретным значением.
func TestCleanAmenitiesDropsEventsRubric(t *testing.T) {
	got := cleanAmenities([]string{"Подходит для проведения мероприятий", "Праздничные и корпоративные мероприятия"})
	want := []string{"Подходит для проведения мероприятий"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("рубрика мероприятий не отсеяна:\n got %v\nwant %v", got, want)
	}
}

// Рубрика «Интернет» стоит рядом с конкретным «WI-FI» у 289 объектов из 300 —
// в списке это выглядит как два разных удобства.
func TestCleanAmenitiesCollapsesInternet(t *testing.T) {
	got := cleanAmenities([]string{"Интернет", "WI-FI", "Мобильный интернет"})
	want := []string{"Wi-Fi", "Мобильный интернет"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("интернет не сведён:\n got %v\nwant %v", got, want)
	}
}

func TestCleanAmenitiesCanonicalizesSpelling(t *testing.T) {
	got := cleanAmenities([]string{"WI-FI", "Wi-Fi", "wifi", "Микроволновая печь"})
	want := []string{"Wi-Fi", "Микроволновая печь"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("написания не сведены:\n got %v\nwant %v", got, want)
	}
}

// Похожие, но разные вещи склеивать нельзя: баня и сауна топятся по-разному,
// общая кухня на территории — не то же, что своя в домике.
func TestCleanAmenitiesKeepsDistinctThings(t *testing.T) {
	in := []string{"Баня", "Сауна", "Горячий чан", "Общая кухня", "Собственная кухня", "Бассейн", "Мини бассейн"}
	got := cleanAmenities(in)
	if !reflect.DeepEqual(got, in) {
		t.Errorf("склеено лишнее:\n got %v\nwant %v", got, in)
	}
}

func TestCleanAmenitiesKeepsOrderAndTrims(t *testing.T) {
	got := cleanAmenities([]string{"  Баня  ", "", "   ", "Рыбалка", "Баня"})
	want := []string{"Баня", "Рыбалка"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("порядок или обрезка сломаны:\n got %v\nwant %v", got, want)
	}
}

func TestCleanAmenitiesEmptyWhenOnlyRubrics(t *testing.T) {
	if got := cleanAmenities([]string{"Территория", "Дети", "Общие"}); got != nil {
		t.Errorf("ожидался nil, получено %v", got)
	}
}
