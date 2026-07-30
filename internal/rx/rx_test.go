package rx

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestWord(t *testing.T) {
	km := Word("км")
	for _, ok := range []string{"2 км", "в 5 км от МКАД", "км", "70км.", "расстояние — 12 км"} {
		if !km.MatchString(ok) {
			t.Errorf("Word(«км») не нашла в %q", ok)
		}
	}
	for _, no := range []string{"километров", "мкм", "экм"} {
		if km.MatchString(no) {
			t.Errorf("Word(«км») ложно сработала на %q", no)
		}
	}
}

func TestWordStart(t *testing.T) {
	banya := WordStart("бан")
	for _, ok := range []string{"баня", "в бане", "банный чан", "Баня-бочка"} {
		if !banya.MatchString(ok) {
			t.Errorf("WordStart(«бан») не нашла в %q", ok)
		}
	}
	// Основа внутри другого слова — не совпадение: иначе «кабан» станет баней.
	for _, no := range []string{"кабан", "ванна", "барабан"} {
		if banya.MatchString(no) {
			t.Errorf("WordStart(«бан») ложно сработала на %q", no)
		}
	}
}

func TestAnyWordStart(t *testing.T) {
	re := AnyWordStart("чан", "купел", "фурако")
	for _, ok := range []string{"горячий чан", "с купелью", "Фурако под небом"} {
		if !re.MatchString(ok) {
			t.Errorf("не нашла в %q", ok)
		}
	}
	if re.MatchString("качание") {
		t.Error("ложное срабатывание на «качание»")
	}
	if AnyWordStart().MatchString("что угодно") {
		t.Error("пустой набор основ не должен совпадать ни с чем")
	}
}

// Страж: `\b` рядом с кириллицей в регулярке — молчаливая ошибка. Совпадений
// не будет, но и ошибки тоже: проверка просто перестаёт работать, и это
// замечают месяцы спустя по кривым данным.
//
// Проверяем ИСХОДНИКИ всего репозитория. Нашлось — используйте rx.Word или
// rx.WordStart вместо `\b`.
func TestNoCyrillicWordBoundaryInSources(t *testing.T) {
	// `\b` в той же строке, что и кириллица.
	suspicious := regexp.MustCompile(`\\b[^"'` + "`" + `\n]*[а-яА-ЯёЁ]|[а-яА-ЯёЁ][^"'` + "`" + `\n]*\\b`)

	root := "../.."
	var found []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // недоступный путь пропускаем, это не предмет теста
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "vendor", "node_modules", "generated", "export":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		// Сам этот файл содержит `\b` в объяснении — он и есть исключение.
		if strings.HasSuffix(path, "rx_test.go") || strings.HasSuffix(path, "rx.go") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			// Комментарии не проверяем: там `\b` обсуждают, а не используют.
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if !strings.Contains(line, `\b`) {
				continue
			}
			if suspicious.MatchString(line) {
				found = append(found, filepath.Base(path)+":"+itoa(i+1)+"  "+trimmed)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("обход исходников: %v", err)
	}

	if len(found) > 0 {
		t.Errorf("`\\b` рядом с кириллицей — совпадений не будет, ошибки тоже.\n"+
			"Используйте rx.Word / rx.WordStart:\n  %s", strings.Join(found, "\n  "))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// «$^» выглядит как «ничего не совпадёт», но с ПУСТОЙ строкой совпадает: конец
// и начало там в одной точке. Правило без основ должно молчать всегда, иначе
// объект без названия получал бы форму жилья из ниоткуда.
func TestEmptySetMatchesNothing(t *testing.T) {
	for _, re := range []*regexp.Regexp{AnyWordStart(), AnyWord()} {
		for _, s := range []string{"", "что угодно", "дом"} {
			if re.MatchString(s) {
				t.Errorf("пустой набор совпал с %q", s)
			}
		}
	}
}

// Основы мало там, где слово продолжается другим: «на дереве» как начало
// совпадает и с «на деревенской улице», а это другое место.
func TestAnyWordNeedsWholeWord(t *testing.T) {
	re := AnyWord("на дереве")
	if !re.MatchString("Домик на дереве у озера") {
		t.Error("целое совпадение не найдено")
	}
	if re.MatchString("Домик на деревенской улице") {
		t.Error("«на деревенской» принято за «на дереве»")
	}
}
