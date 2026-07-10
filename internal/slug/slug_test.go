package slug

import "testing"

func TestMake(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Деревня Ильино", "derevnya-ilino"},
		{"AFRAME", "aframe"},
		{"Ягодка-Малинка", "yagodka-malinka"},
		{"Время в Лесу", "vremya-v-lesu"},
		{"KENZA ", "kenza"},                // хвостовой пробел
		{"Ёжик & Ко", "ezhik-ko"},          // ё и спецсимволы
		{"aframe_iv", "aframe-iv"},         // подчёркивание → дефис
		{"  --  ", ""},                     // мусор → пусто
		{"Щёлково 2000", "schelkovo-2000"}, // щ/ё/цифры
		{"Baza --- Otdyha", "baza-otdyha"}, // схлопывание дефисов
	}
	for _, tt := range tests {
		if got := Make(tt.in); got != tt.want {
			t.Errorf("Make(%q) = %q, ожидал %q", tt.in, got, tt.want)
		}
	}
}
