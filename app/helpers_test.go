package main

import "testing"

func TestIsValidDomain(t *testing.T) {
	valid := []string{"elkidom37", "scandi.villa", "a_b.c123"}
	invalid := []string{"../etc", "a/b", "", "a b", "..%2f"}
	for _, d := range valid {
		if !isValidDomain(d) {
			t.Errorf("домен %q должен быть валиден", d)
		}
	}
	for _, d := range invalid {
		if isValidDomain(d) {
			t.Errorf("домен %q должен быть отвергнут", d)
		}
	}
}
