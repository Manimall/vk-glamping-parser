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

func TestSuitable_AcceptsLeadingQuote(t *testing.T) {
	// sentences считает кавычку законным началом предложения, и suitable не
	// должен из-за неё отбрасывать живое описание курируемого объекта.
	s := `«Баня место силы» и дома с чаном, для отдыха с атмосферой уюта и эстетики`
	if !suitable(s) {
		t.Errorf("кандидат с ведущей кавычкой должен проходить: %q", s)
	}
}

func TestSuitable_RejectsAnaphora(t *testing.T) {
	// Предложение, вырванное из абзаца: без соседей не читается.
	for _, s := range []string{
		"Он предлагает уютную атмосферу благодаря собственной территории у леса",
		"Однако это не единственное место релакса в доме на берегу озера",
	} {
		if suitable(s) {
			t.Errorf("связка без антецедента не должна проходить: %q", s)
		}
	}
}

func TestSuitable_RejectsPrice(t *testing.T) {
	if suitable("Банный чан на отдельном участке в лесу — 2500 рублей в час") {
		t.Error("прайс-лист не должен попадать в сниппет")
	}
}

func TestClauseHead_CutsAtComma(t *testing.T) {
	s := "Уютное место для тех, кто ищет природу и настоящий отдых, вдали от городской суеты"
	got := clauseHead(s, 60)
	if got != "Уютное место для тех, кто ищет природу и настоящий отдых" {
		t.Errorf("рез по границе клаузы: %q", got)
	}
}

func TestClauseHead_NoBoundary(t *testing.T) {
	// Границы нет — лучше отдать шаблон, чем обрубок посреди слова.
	if got := clauseHead("Просторный дом с панорамными окнами в сосновом лесу", 30); got != "" {
		t.Errorf("без запятой резать нечего: %q", got)
	}
}

func TestFallbackPitch_FitsBudget(t *testing.T) {
	// Длинная локация не должна выносить описание за лимит: инвариант обязан
	// держаться на обеих ветках, а не только там, где нашёлся питч.
	long := "Ивановская обл., Ивановский р-н, д. Крюково, Славянская ул., 6"
	seo := BuildSEO(SEOInput{Name: "Scandi Villa (А-фрейм)", Location: long, About: "Забронировать: телеграм"})
	if n := runes(seo.Description); n > seoDescTotalRunes {
		t.Errorf("фоллбэк вышел за лимит: %d > %d — %q", n, seoDescTotalRunes, seo.Description)
	}
}

func TestNormalizeAbout_UnicodeSpaces(t *testing.T) {
	// \s в RE2 — только ASCII, поэтому U+2003 доживал до сниппета.
	got := normalizeAbout("Отдых в бору")
	if strings.Contains(got, " ") || strings.Contains(got, " ") {
		t.Errorf("юникод-пробелы должны схлопываться: %q", got)
	}
}
