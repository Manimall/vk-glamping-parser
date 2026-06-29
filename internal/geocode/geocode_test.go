package geocode

import (
	"strings"
	"testing"
)

// TestCandidates проверяет «огрубление» адреса: раскрытие сокращений и запрос
// «населённый пункт + область» для сельских адресов (сетевых вызовов нет).
func TestCandidates(t *testing.T) {
	got := candidates("Ивановская обл., Ивановский р-н, д. Крюково, Славянская ул., 6")

	if len(got) < 2 {
		t.Fatalf("ожидал минимум 2 варианта (полный + НП+область), получил %v", got)
	}

	// Первый — нормализованный полный адрес (сокращения раскрыты).
	if got[0] != "Ивановская область, Ивановский район, деревня Крюково, Славянская улица, 6" {
		t.Errorf("нормализация неверна: %q", got[0])
	}

	// Должен появиться компактный вариант «деревня Крюково Ивановская область».
	wantCoarse := "деревня Крюково Ивановская область"
	found := false
	for _, q := range got {
		if q == wantCoarse {
			found = true
		}
	}
	if !found {
		t.Errorf("ожидал вариант %q среди %v", wantCoarse, got)
	}
}

// TestCandidatesCity: для города без запятых — один вариант (НП+область не собрать).
func TestCandidatesCity(t *testing.T) {
	got := candidates("Иваново")
	if len(got) != 1 || got[0] != "Иваново" {
		t.Errorf("для города ожидал [Иваново], получил %v", got)
	}
}

// TestCandidatesHouseNumber: «д. 5» (номер дома) НЕ должен стать «деревня 5»,
// а «д. Светлое» (населённый пункт) — должен раскрыться.
func TestCandidatesHouseNumber(t *testing.T) {
	got := candidates("Ивановская обл., д. Светлое, д. 5")
	for _, q := range got {
		if strings.Contains(q, "деревня 5") {
			t.Errorf("номер дома ошибочно стал деревней: %q", q)
		}
	}
	if !strings.Contains(got[0], "деревня Светлое") {
		t.Errorf("населённый пункт не раскрылся: %q", got[0])
	}
}
