package glamping_rf

// Характеристики домиков из window.pv12RoomDetails: площадь и спальные места.
//
// Прежде вместимость искалась регуляркой по вёрстке — и не находилась НИ НА
// ОДНОЙ странице: слова «Вместимость» в разметке источника больше нет. Из-за
// этого у 620 объектов из 622 стояло дефолтное «до 4 гостей». Гость видел
// «до 4», ехал вчетвером, а его ждала одна двуспальная кровать.
//
// Настоящие данные лежат в том же JSON, который парсер уже разбирает ради
// платных услуг, — просто в соседнем поле:
//
//	specs: ["📐 32 м²", "🛌 1 двуспальная"]

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// areaSpecRe — площадь домика: «📐 32 м²». Значок игнорируем, он может
// смениться; опираемся на число перед «м²».
var areaSpecRe = regexp.MustCompile(`(\d+)\s*м²`)

// bedSpecRe — спальное место: «🛌 1 двуспальная», «🛏 2 односпальная»,
// «🛋 1 диван-кровать». Число впереди — сколько таких мест.
var bedSpecRe = regexp.MustCompile(`(?i)(\d+)\s*(двуспальн|односпальн|диван)`)

// Сколько гостей помещается на одно спальное место каждого вида.
//
// Диван-кровать считаем за двоих: раскладывается именно на двоих, и занижение
// здесь опаснее завышения — гость, приехавший вдвоём на «одно место», уедет,
// а не потерпит.
const (
	guestsPerDouble = 2
	guestsPerSingle = 1
	guestsPerSofa   = 2
)

// roomSpecs — характеристики домика: площадь в м² и вместимость по спальным
// местам. Нули означают «источник не сказал»: подставлять дефолт нельзя,
// именно так и появилось выдуманное «до 4».
type roomSpecs struct {
	AreaM2 int
	Guests int
}

// pv12RoomWithSpecs — домик с характеристиками. Отдельно от pv12Room:
// тот описывает удобства и меняется по своей причине.
type pv12RoomWithSpecs struct {
	Name  string   `json:"name"`
	Specs []string `json:"specs"`
}

// houseTypeByName — форма жилья по НАЗВАНИЮ домика.
//
// Тег types у источника отвечает на этот вопрос плохо: у двух третей каталога
// там «Эко-дом». Причина видна на живых данных — у объекта бывает девять
// домиков разных форм («Geocupol Deluxe», «Barn Family», «Smart Lodge»), и
// одним тегом их не описать, поэтому источник ставит общий.
//
// А в названиях форма есть почти всегда: владелец называет домик так, как его
// продаёт. Порядок правил важен — сначала узкие, потом широкие.
var houseTypeByName = []struct {
	pattern *regexp.Regexp
	label   string
}{
	{regexp.MustCompile(`(?i)на дереве|tree\s*house`), "Дом на дереве"},
	{regexp.MustCompile(`(?i)а[\s-]?фрейм|a[\s-]?frame`), "A-frame"},
	{regexp.MustCompile(`(?i)geocupol|геокупол|купольн|сфера|dome`), "Купольный дом"},
	// Граница слова здесь без `\b`: в Go RE2 она опирается на латиницу и с
	// кириллицей молча не срабатывает — «Дом „Барн“» и «Фридом Барн с диваном»
	// так и не находились. Вместо неё — «после „барн“ не кириллическая буква».
	{regexp.MustCompile(`(?i)barn|барнхаус|барн(?:[^а-яё]|$)`), "Барнхаус"},
	{regexp.MustCompile(`(?i)модом|модульн`), "Модульный дом"},
	{regexp.MustCompile(`(?i)сафари|шатёр|шатер|тент`), "Сафари-тент"},
	{regexp.MustCompile(`(?i)кэмпер|camper|автодом`), "Кэмпер"},
}

// houseTypesFromRooms — формы жилья по названиям домиков объекта.
//
// Возвращает все найденные: у объекта с куполами и барнхаусами гость должен
// находиться и в том разделе, и в другом. Порядок стабилен — по порядку правил,
// а не по порядку домиков в JSON (тот у map произвольный).
func houseTypesFromRooms(page string) []string {
	raw := balancedJSON(page, roomDetailsMarker, '{', '}')
	if raw == "" {
		return nil
	}
	var rooms map[string]pv12RoomWithSpecs
	if err := json.Unmarshal([]byte(raw), &rooms); err != nil {
		return nil
	}

	found := make(map[string]bool)
	for _, room := range rooms {
		for _, rule := range houseTypeByName {
			if rule.pattern.MatchString(room.Name) {
				found[rule.label] = true
			}
		}
	}
	if len(found) == 0 {
		return nil
	}

	out := make([]string, 0, len(found))
	for _, rule := range houseTypeByName {
		if found[rule.label] {
			out = append(out, rule.label)
		}
	}
	return out
}

// parseRoomSpecs — площадь и вместимость ОДНОГО домика по его specs.
func parseRoomSpecs(specs []string) roomSpecs {
	var out roomSpecs
	for _, s := range specs {
		if m := areaSpecRe.FindStringSubmatch(s); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				// Площадь берём наибольшую: у домика она одна, но если источник
				// перечислил комнаты, гостю важна общая.
				out.AreaM2 += n
			}
			continue
		}
		if m := bedSpecRe.FindStringSubmatch(s); m != nil {
			n, err := strconv.Atoi(m[1])
			if err != nil || n <= 0 {
				continue
			}
			switch strings.ToLower(m[2]) {
			case "двуспальн":
				out.Guests += n * guestsPerDouble
			case "односпальн":
				out.Guests += n * guestsPerSingle
			case "диван":
				out.Guests += n * guestsPerSofa
			}
		}
	}
	return out
}

// detailRoomSpecs — характеристики САМОГО ВМЕСТИТЕЛЬНОГО домика объекта.
//
// У объекта бывает несколько домиков разного размера. Гостю в карточке важно
// «сколько нас поместится» — то есть максимум, а не сумма: суммой мы обещали
// бы компанию из 12 человек там, где есть три домика по четверо, и каждый
// бронируется отдельно.
func detailRoomSpecs(page string) roomSpecs {
	raw := balancedJSON(page, roomDetailsMarker, '{', '}')
	if raw == "" {
		return roomSpecs{}
	}
	var rooms map[string]pv12RoomWithSpecs
	if err := json.Unmarshal([]byte(raw), &rooms); err != nil {
		return roomSpecs{}
	}

	var best roomSpecs
	for _, room := range rooms {
		s := parseRoomSpecs(room.Specs)
		if s.Guests > best.Guests {
			best.Guests = s.Guests
		}
		if s.AreaM2 > best.AreaM2 {
			best.AreaM2 = s.AreaM2
		}
	}
	return best
}
