package extract

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

// Отбор «питча» для meta-description из описания объекта.
//
// Описания приходят с сайта-агрегатора одним сплошным абзацем (медиана 1687
// символов у 313 объектов из 319), поэтому разбор идёт по предложениям, а не по
// строкам: разбиение по \n на таких данных не срабатывает вовсе.
//
// Два правила, ради которых всё написано. Первое: в сниппет не должен попадать
// голос владельца объекта — «Мы рады предложить вам», «Наш глэмпинг». В выдаче
// такой текст подписан нашим доменом, а с владельцами 313 объектов у нас нет
// никаких отношений. Второе: описание обязано укладываться в лимит целиком,
// вместе с именем и призывом, иначе поисковик обрывает его многоточием.

const (
	// Предельная длина ВСЕГО description. Поисковик показывает ~150-160
	// символов; лимит на один лишь питч (как было) не спасает — имя и призыв
	// прибавляются сверху.
	seoDescTotalRunes = 150
	// Ниже этого питч не берём: обрывок вида «Тихий уголок» ничего не говорит.
	seoMinPitchRunes = 40
)

var nbspReplacer = strings.NewReplacer(" ", " ", " ", " ", " ", " ")

// Схлопываем пробелы, но НЕ переводы строк: у описаний из ВК перенос — это
// единственная граница предложения (точек в них нет вовсе), и схлопывание
// склеивало весь текст в одну строку с «Забронировать» внутри.
// \s в RE2 — только ASCII, поэтому U+2003/U+2009/U+200B доживали до сниппета
// и давали в выдаче тройные пробелы. Класс перечислен явно.
var spacesRe = regexp.MustCompile(`[^\S\n\p{Zs}]|[\p{Zs}\x{200b}-\x{200f}\x{feff}]+`)
var newlinesRe = regexp.MustCompile(`\n+`)
var spaceBeforePunctRe = regexp.MustCompile(`[^\S\n]+([,.!?;:])`)

// normalizeAbout приводит описание к виду, пригодному для разбора.
func normalizeAbout(about string) string {
	s := html.UnescapeString(about)
	s = emojiRe.ReplaceAllString(s, " ")
	s = nbspReplacer.Replace(s)
	s = spacesRe.ReplaceAllString(s, " ")
	s = newlinesRe.ReplaceAllString(s, "\n")
	s = spaceBeforePunctRe.ReplaceAllString(s, "$1")
	return strings.TrimSpace(s)
}

// candidates даёт варианты питча из одного предложения, от лучшего к худшему.
func candidates(sentence, name string) []string {
	base := headingRe.ReplaceAllString(sentence, "")
	base = strings.TrimSpace(base)
	if base == "" {
		return nil
	}

	out := make([]string, 0, 3)
	add := func(s string) {
		s = strings.TrimSpace(strings.Trim(strings.TrimSpace(s), " .!?…,;:—-"))
		if s != "" {
			out = append(out, s)
		}
	}

	// Склейка заголовка с телом: обе половины — самостоятельные кандидаты,
	// целое отбрасываем (в нём заведомо сидит чужой голос). Половинку,
	// оборванную на прилагательном, не берём — разрез пришёлся не туда.
	if loc := glueRe.FindStringSubmatchIndex(base); loc != nil {
		cut := loc[2] - 1
		if cut > 0 {
			for _, half := range []string{base[:cut], base[cut:]} {
				if !danglingAdjRe.MatchString(strings.Trim(strings.TrimSpace(half), " .!?…,;:—-")) {
					add(half)
				}
			}
			return dropName(out, name)
		}
	}

	// Голова до двоеточия — законченная мысль, а перечисление за ним в питч
	// не годится никогда.
	if i := strings.Index(base, ":"); i > 0 {
		add(base[:i])
	}
	add(base)
	return dropName(out, name)
}

// dropName снимает ведущее имя объекта: «Сабадури — А-фрейм в лесу» после
// «Сабадури — » читалось бы как «Сабадури — сабадури — А-фрейм в лесу».
func dropName(items []string, name string) []string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" {
		return items
	}
	out := make([]string, 0, len(items))
	for _, s := range items {
		if strings.HasPrefix(strings.ToLower(s), n) {
			rest := strings.TrimLeft(s[len(n):], " —-:,.")
			if len([]rune(rest)) >= seoMinPitchRunes {
				out = append(out, rest)
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// startsUpper — с заглавной ли буквы кандидат. Ведущие кавычки пропускаем:
// без этого отбрасывалось бы «„Баня место силы“ и дома с чаном…» — живое
// описание курируемого объекта.
func startsUpper(r []rune) bool {
	for _, c := range r {
		if unicode.IsLetter(c) {
			return unicode.IsUpper(c)
		}
		if !strings.ContainsRune(`«»"'‹›„“(`, c) {
			return false
		}
	}
	return false
}

func suitable(s string) bool {
	r := []rune(s)
	if len(r) < seoMinPitchRunes || !startsUpper(r) {
		return false
	}
	if !placeWordRe.MatchString(s) {
		return false
	}
	if serviceTailRe.MatchString(s) {
		return false
	}
	for _, re := range []*regexp.Regexp{
		firstPersonRe, firstPersonVerbRe, invitationRe,
		houseRulesRe, unitsRe, junkLineRe, headingRe, colonRe, inventoryRe,
		anaphoraRe,
	} {
		if re.MatchString(s) {
			return false
		}
	}
	return true
}

// pickPitch — первый годный фрагмент, влезающий в бюджет. Ровно один: добор
// второго замерялся и не менял результат ни на одном объекте.
func pickPitch(about, name string, budget int) string {
	if budget < seoMinPitchRunes {
		return ""
	}
	for _, sentence := range sentences(normalizeAbout(about)) {
		for _, c := range candidates(sentence, name) {
			if len([]rune(c)) <= budget {
				if suitable(c) {
					return c
				}
				continue
			}
			if short := clauseHead(c, budget); short != "" && suitable(short) {
				return short
			}
		}
	}
	return ""
}

// clauseHead — начало кандидата до последней запятой или тире в пределах
// бюджета. Не обрезка по словам: режем там, где мысль и так кончается, поэтому
// многоточие не нужно. Без этого каждое третье описание терялось из-за
// перебора в десяток символов.
func clauseHead(s string, budget int) string {
	r := []rune(s)
	if len(r) <= budget {
		return s
	}
	head := string(r[:budget])
	cut := strings.LastIndexAny(head, ",—–")
	if cut <= 0 {
		return ""
	}
	out := strings.TrimSpace(strings.Trim(strings.TrimSpace(head[:cut]), " ,;:—–-"))
	if len([]rune(out)) < seoMinPitchRunes {
		return ""
	}
	return out
}
