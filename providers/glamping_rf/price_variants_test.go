package glamping_rf

import "testing"

// Реальный случай «Удачного места» (объект на 7 гостей, ночь 8 100 ₽):
// в одном описании баня на 20 человек и баня на 6, парсер брал первую.
func TestPriceFromDescPicksLowestOfVariants(t *testing.T) {
	desc := "Аренда Большая баня до 20 чел стоимость 50 000 руб. Таежная баня с чаном до 6 чел стоимость 12 000 руб"
	if got := priceFromDesc(desc); got != "от 12 000 ₽" {
		t.Errorf("ожидалось «от 12 000 ₽», получено %q", got)
	}
}

// Доплата — не самостоятельная цена: без её отсева аренда за 5 000 превратилась
// бы в «от 3 500 ₽», то есть в цену, по которой снять нельзя.
func TestPriceFromDescIgnoresSurcharge(t *testing.T) {
	cases := []struct{ desc, want string }{
		{"Базовая аренда: 5000 рублей чана. Каждый последующий день: доплата 3500 рублей", "5 000 ₽"},
		{"Аренда 7000 руб. Дополнительный час 1500 руб", "7 000 ₽"},
		{"Стоимость 4000 руб. За каждого гостя сверх четырёх доплата 500 руб", "4 000 ₽"},
	}
	for _, c := range cases {
		if got := priceFromDesc(c.desc); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.desc, got, c.want)
		}
	}
}

// Одна цена — прежнее поведение, без пометки «от».
func TestPriceFromDescSingleUnchanged(t *testing.T) {
	cases := []struct{ desc, want string }{
		{"Чан, купель стоимость 5000 руб на 3 часа", "5 000 ₽"},
		{"Доплата 1500р/питомец", "1 500 ₽"},
		{"", ""},
	}
	for _, c := range cases {
		if got := priceFromDesc(c.desc); got != c.want {
			t.Errorf("%q:\n got %q\nwant %q", c.desc, got, c.want)
		}
	}
}

// Почасовую и «за всё» сравнивать нельзя — это разные величины, и минимум из
// них не значит ничего. Такие описания идут прежним путём.
func TestPriceVariantsSkipsHourly(t *testing.T) {
	if got := priceVariants("Сауна 1500 ₽/час. Баня 12 000 руб за сутки"); got != nil {
		t.Errorf("почасовая попала в сравнение: %v", got)
	}
}

func TestPriceVariantsNeedsTwo(t *testing.T) {
	if got := priceVariants("Баня 12 000 руб"); got != nil {
		t.Errorf("одна цена не должна давать варианты: %v", got)
	}
	if got := priceVariants("Баня 12 000 руб. Полотенца включены"); got != nil {
		t.Errorf("вариант без цены не считается: %v", got)
	}
}

func TestLowestPrice(t *testing.T) {
	if got := lowestPrice([]int{50000, 12000, 30000}); got != 12000 {
		t.Errorf("ожидалось 12000, получено %d", got)
	}
}
