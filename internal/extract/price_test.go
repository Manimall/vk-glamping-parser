package extract

import "testing"

// Формат цены — общий для провайдеров и совпадает с тем, что уже лежит в
// каталоге. Разделитель проверяется по коду символа: обычный пробел и
// неразрывный на глаз неотличимы, а в данных это разные строки.
func TestFormatPrice(t *testing.T) {
	cases := []struct {
		value int
		want  string
	}{
		{0, "0 ₽"},
		{100, "100 ₽"},
		{999, "999 ₽"},
		{1000, "1 000 ₽"},
		{6000, "6 000 ₽"},
		{7650, "7 650 ₽"},
		{999999, "999 999 ₽"},
		{1000000, "1 000 000 ₽"},
	}
	for _, c := range cases {
		if got := FormatPrice(c.value); got != c.want {
			t.Errorf("FormatPrice(%d) = %q, ожидалось %q", c.value, got, c.want)
		}
	}
}

func TestFormatPriceUsesPlainSpace(t *testing.T) {
	for _, r := range FormatPrice(7650) {
		if r == ' ' {
			t.Fatalf("разряды разделены неразрывным пробелом: %q — каталог собран с обычным", FormatPrice(7650))
		}
	}
}
