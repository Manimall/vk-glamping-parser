package extract

import (
	"strings"
	"testing"
)

func TestSentences_SplitsSolidParagraph(t *testing.T) {
	// Так приходят описания с сайта-агрегатора: один абзац без переносов.
	got := sentences("Дом стоит в лесу. Рядом озеро! Есть баня?")
	if len(got) != 3 {
		t.Fatalf("ожидал 3 предложения, получил %d: %q", len(got), got)
	}
	if got[0] != "Дом стоит в лесу." {
		t.Errorf("терминатор должен остаться при предложении: %q", got[0])
	}
}

func TestSentences_Abbreviations(t *testing.T) {
	// «р. Волга» — сокращение, не конец предложения: две кириллические буквы
	// перед точкой. А «Wi-Fi.» — конец, иначе следующая фраза слипается.
	got := sentences("Дом на берегу р. Волга в тишине. Есть Wi-Fi. Рядом лес.")
	if len(got) != 3 {
		t.Fatalf("ожидал 3 предложения, получил %d: %q", len(got), got)
	}
	if !strings.Contains(got[0], "р. Волга") {
		t.Errorf("сокращение не должно рвать предложение: %q", got[0])
	}
}

func TestSentences_GlueWithoutSpace(t *testing.T) {
	// Склейка блоков источником даёт точку без пробела.
	got := sentences("Дом на берегу реки Нерль.Идеальное место для отдыха.")
	if len(got) != 2 {
		t.Fatalf("ожидал 2 предложения, получил %d: %q", len(got), got)
	}
}

func TestSentences_KeepsNewlineAsBoundary(t *testing.T) {
	// У описаний из ВК точек нет вовсе — единственная граница это перенос.
	got := sentences("Уютный домик в лесу\nВековое озеро рядом\nКупель под небом")
	if len(got) != 3 {
		t.Fatalf("перенос строки — граница предложения, получил %d: %q", len(got), got)
	}
}

func TestSuitable_RejectsOwnerVoice(t *testing.T) {
	cases := map[string]string{
		"местоимение":      "Мы построили пять уютных домов в сосновом лесу у озера",
		"глагол 1 лица":    "Приглашаем вас отдохнуть в уютном доме на берегу лесного озера",
		"императив":        "Живите в гармонии с природой в доме у соснового леса и озера",
		"регламент":        "Заезд с 14:00, выезд до 12:00, залог за дом на берегу озера",
		"единицы":          "Дом 85 кв.м у лесного озера с баней и просторной террасой",
		"рубрика":          "Расположение и окружение лесного озера рядом с домом и баней",
		"перечисление":     "На территории дома: купель, мангальная зона, парковка, лес",
		"обрубок в хвосте": "Дом стоит в лесу рядом с озером, где Вы",
	}
	for name, s := range cases {
		if suitable(s) {
			t.Errorf("%s: не должно проходить в сниппет: %q", name, s)
		}
	}
}

func TestSuitable_AcceptsPlaceDescription(t *testing.T) {
	good := []string{
		"Уютная лесная дача в местечке Берёзовая роща рядом с озером",
		"Комплекс загородных домов в аренду рядом с Переславль-Залесским",
		"Дом с панорамными окнами на берегу лесного озера в сосновом бору",
	}
	for _, s := range good {
		if !suitable(s) {
			t.Errorf("нормальное описание места должно проходить: %q", s)
		}
	}
}

func TestSuitable_RejectsWithoutPlaceWord(t *testing.T) {
	// Позитивный фильтр: без слова про место фраза формально чиста, но
	// в сниппете бесполезна.
	if suitable("Оплата производится в день заселения любым удобным способом") {
		t.Error("фраза без слова про место не должна проходить")
	}
}

func TestDropName_RemovesLeadingName(t *testing.T) {
	// Иначе «Сабадури — сабадури — А-фрейм в сосновом лесу».
	got := dropName([]string{"Сабадури — А-фрейм в сосновом лесу Тейковского района"}, "Сабадури")
	if strings.HasPrefix(strings.ToLower(got[0]), "сабадури") {
		t.Errorf("ведущее имя должно сниматься: %q", got[0])
	}
}

func TestDropName_KeepsWhenRestTooShort(t *testing.T) {
	// Снимать имя, если остаётся огрызок, нельзя — лучше оставить как есть.
	items := []string{"Лесной дом у озера"}
	got := dropName(items, "Лесной дом")
	if got[0] != items[0] {
		t.Errorf("короткий остаток — имя не снимаем: %q", got[0])
	}
}

func TestPickPitch_RespectsBudget(t *testing.T) {
	// Первое предложение в бюджет не влезает — берём следующее годное.
	about := "Очень длинное первое предложение про дом в сосновом лесу у тихого озера с баней и просторной террасой на самом берегу. " +
		"Дом стоит в сосновом лесу на берегу озера. Ещё предложение."
	got := pickPitch(about, "Тест", 60)
	if got == "" {
		t.Fatal("подходящее по длине предложение должно найтись за длинным")
	}
	if len([]rune(got)) > 60 {
		t.Errorf("питч не влез в бюджет: %d рун, %q", len([]rune(got)), got)
	}
}

func TestPickPitch_NoTruncationMidSentence(t *testing.T) {
	// Никакой обрезки по словам: обрыв дорисует поисковик, своё многоточие
	// читается как поломка генератора.
	about := "Просторный дом с панорамными окнами в сосновом лесу на берегу тихого лесного озера недалеко от города"
	got := pickPitch(about, "Тест", 50)
	if got != "" {
		t.Errorf("предложение длиннее бюджета берём целиком или не берём вовсе: %q", got)
	}
}

func TestBuildSEO_LimitCoversWholeString(t *testing.T) {
	// Лимит на весь description, а не на питч: имя и призыв прибавляются
	// сверху, и раньше описание разрасталось до 2881 символа при лимите 160.
	long := strings.Repeat("Дом в лесу у озера с баней. ", 40)
	seo := BuildSEO(SEOInput{Name: "Очень Длинное Название Объекта", Location: "Тверская область", About: long})
	if n := len([]rune(seo.Description)); n > seoDescTotalRunes {
		t.Errorf("описание длиннее лимита: %d > %d — %q", n, seoDescTotalRunes, seo.Description)
	}
}

func TestBuildSEO_FallbackKeepsLocation(t *testing.T) {
	// Годного питча нет — шаблон с локацией, а не пустота.
	seo := BuildSEO(SEOInput{
		Name:     "Домик",
		Location: "д. Крюково, Ивановская обл.",
		About:    "Забронировать: сообщения в Telegram",
	})
	if !strings.Contains(seo.Description, seoFallbackPitch) ||
		!strings.Contains(seo.Description, "д. Крюково") {
		t.Errorf("ожидал шаблон с локацией: %q", seo.Description)
	}
}
