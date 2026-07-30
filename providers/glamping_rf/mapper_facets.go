package glamping_rf

// Признаки для фильтров каталога: что вокруг, можно ли с питомцем, далеко ли
// ехать, сколько стоит числом.
//
// Источник отдаёт всё это структурно, а раньше половина добывалась разбором
// описания через LLM — и добывалась хуже: «в лесу» находилось у 28 объектов
// против 213 по тегам, питомцы у 43% против 97%. Структурный признак не
// галлюцинирует и не зависит от того, как владелец написал текст.

import (
	"strconv"
	"strings"
)

// surroundingLabels — теги окружения источника в человекочитаемый вид.
// bnovo — служебная метка системы бронирования, к окружению отношения не имеет
// и в выдачу не идёт.
var surroundingLabels = map[string]string{
	"forest": "Лес",
	"river":  "Река",
	"lake":   "Озеро",
	"ski":    "Горнолыжный склон",
}

// surroundings — что вокруг объекта. Неизвестные теги пропускаем молча:
// источник вправе завести новый, и падать из-за этого незачем.
func surroundings(badges []string) []string {
	out := make([]string, 0, len(badges))
	for _, b := range badges {
		if label, ok := surroundingLabels[strings.ToLower(strings.TrimSpace(b))]; ok {
			out = append(out, label)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// canonicalHouseType — единое написание формы жилья.
//
// Источник и наш словарь называют одно и то же по-разному: «БарнХаус» против
// «Барнхаус», «Шатер» против «Сафари-тент». Без канонизации в каталоге
// появляются два раздела на одну форму, и объекты делятся между ними — гость,
// выбравший «Барнхаус», не увидит половину.
//
// Ключ сравнения — нижний регистр без пробелов и дефисов: так «A-frame»,
// «а фрейм» и «Афрейм» сходятся в одно.
var canonicalHouseType = map[string]string{
	"барнхаус":     "Барнхаус",
	"barnhouse":    "Барнхаус",
	"barn":         "Барнхаус",
	"aframe":       "A-frame",
	"афрейм":       "A-frame",
	"купольныйдом": "Купольный дом",
	"геокупол":     "Купольный дом",
	"сфера":        "Купольный дом",
	"модульныйдом": "Модульный дом",
	"модом":        "Модульный дом",
	"сафаритент":   "Сафари-тент",
	"шатер":        "Сафари-тент",
	"шатёр":        "Сафари-тент",
	"типи":         "Сафари-тент",
	"домнадереве":  "Дом на дереве",
	"домнаводе":    "Дом на воде",
	"кэмпер":       "Кэмпер",
	"номер":        "Номер",
}

// canonHouseType приводит название к единому виду. Незнакомое возвращаем как
// есть: источник вправе завести новую форму, и терять её незачем.
func canonHouseType(name string) string {
	key := strings.ToLower(strings.NewReplacer(" ", "", "-", "", "ё", "е").Replace(strings.TrimSpace(name)))
	if canon, ok := canonicalHouseType[key]; ok {
		return canon
	}
	return strings.TrimSpace(name)
}

// mergeHouseTypes — объединить формы жилья из двух источников без дублей.
//
// Теги списка дают форму у 40% объектов (остальным источник ставит «Эко-дом»,
// который мы отбрасываем как свалку), названия домиков с detail-страницы — у
// большинства остальных. Вместе они покрывают почти весь каталог, и ни один из
// двух источников не полон сам по себе.
//
// Порядок сохраняем: сначала то, что сказал список, потом найденное в именах.
func mergeHouseTypes(fromList, fromRooms []string) []string {
	if len(fromList) == 0 && len(fromRooms) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(fromList)+len(fromRooms))
	out := make([]string, 0, len(fromList)+len(fromRooms))
	for _, group := range [][]string{fromList, fromRooms} {
		for _, raw := range group {
			t := canonHouseType(raw)
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// petsAllowed — «yes»/«no» источника в трёхзначное поле контракта.
// nil означает «источник не сказал»: показывать молчание как запрет нельзя,
// гость с собакой просто не поедет туда, где на самом деле можно.
func petsAllowed(animal string) *bool {
	switch strings.ToLower(strings.TrimSpace(animal)) {
	case "yes":
		t := true
		return &t
	case "no":
		f := false
		return &f
	default:
		return nil
	}
}

// distanceKm — расстояние до опорного города в километрах.
//
// Источник шлёт число строкой («70») и отдельно единицу измерения — «км от
// МКАД», «км от города», «км от аэропорта». Все варианты в километрах, но
// точка отсчёта разная, поэтому она хранится рядом (NearCity) и без неё
// цифра смысла не имеет.
//
// 0 означает «неизвестно»: расстояние в ноль километров от Москвы у
// загородного дома всё равно означало бы ошибку данных.
func distanceKm(raw string) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return 0
	}
	return n
}
