package glamping_rf

// Характеристики домиков из window.pv12RoomDetails: площадь, спальные места и
// вместимость.
//
// Прежде вместимость искалась регуляркой по вёрстке и не находилась НИ НА ОДНОЙ
// странице — у 620 объектов из 622 стояло дефолтное «до 4 гостей». Гость видел
// «до 4», ехал вчетвером, а его ждала одна двуспальная кровать.
//
// Причина оказалась мельче, чем думали: слово «Вместимость» на месте (14
// вхождений на странице), просто формат стал другим — «Вместимость: ДО 6
// гостей», а регулярка ждала число сразу за двоеточием.
//
// Данные лежат в том же JSON, который парсер уже разбирает ради платных услуг:
//
//	specs: ["📐 32 м²", "🛌 1 двуспальная"]
//	desc:  "Вместимость: до 6 гостей (…)"
//
// Заявление источника всегда важнее нашей арифметики по кроватям: оно
// учитывает и правила объекта, и то, что кровать бывает детской.

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"vk-parser/internal/rx"
)

// areaSpecRe — площадь домика: «📐 32 м²», «📐 32.5 м²», «📐 32,5 м²». Значок
// игнорируем, он может смениться; опираемся на число перед «м²».
//
// Дробная часть в шаблоне обязательна, хотя площадь мы округляем до целых:
// без неё регулярка совпадала с ХВОСТОМ числа — «32.5 м²» давало 5 м², то есть
// домик уменьшался в шесть раз ровно там, где источник был точнее обычного.
var areaSpecRe = regexp.MustCompile(`(\d+)(?:[.,](\d+))?\s*м²`)

// bedSpecRe — спальное место: «🛌 1 двуспальная», «🛏 2 односпальная»,
// «🛋 1 диван-кровать». Число впереди — сколько таких мест.
var bedSpecRe = regexp.MustCompile(`(?i)(\d+)\s*(двуспальн|односпальн|диван)`)

// Сколько гостей помещается на одно спальное место каждого вида.
//
// Диван-кровать — ОДИН гость, а не двое. Сверка с заявлениями источника на
// живых страницах: «3 двуспальные + диван» у него «до 6», а не 8; «1 дв. +
// 2 одн. + диван» — «до 5», а не 6. Оба расхождения дал именно диван, и оба в
// сторону завышения — то есть ровно та ошибка, от которой уходили: гость видит
// число, едет такой компанией, а места нет.
const (
	guestsPerDouble = 2
	guestsPerSingle = 1
	guestsPerSofa   = 1
)

// declaredCapacityRe — вместимость, названная источником прямо в описании
// комнаты: «Вместимость: до 6 гостей», «Вместимость: до 4-х гостей».
//
// Старая регулярка по вёрстке (capacityRe) не совпадала не потому, что слово
// исчезло — оно на месте, 14 вхождений на странице. Она не ждала «до».
var declaredCapacityRe = regexp.MustCompile(`(?i)вместимость[^0-9]{0,20}(\d{1,2})`)

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
	Desc  string   `json:"desc"`
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
// Основы ищем с границы слова (rx.AnyWordStart), а не подстрокой: иначе
// «Атмосфера» и «Биосфера» становятся куполом, а «Аутентика» — сафари-тентом,
// потому что внутри сидят «сфера» и «тент». Границу пишем через rx, потому что
// `\b` в Go RE2 опирается на латиницу и с кириллицей молча не срабатывает.
var houseTypeByName = []struct {
	pattern *regexp.Regexp
	label   string
}{
	{regexp.MustCompile(`(?i)на дереве|tree\s*house`), "Дом на дереве"},
	{rx.AnyWordStart("афрейм", "а-фрейм", "а фрейм", "aframe", "a-frame", "a frame"), "A-frame"},
	{rx.AnyWordStart("geocupol", "геокупол", "купольн", "сфера", "сфере", "dome"), "Купольный дом"},
	{rx.AnyWordStart("barn", "барн"), "Барнхаус"},
	{rx.AnyWordStart("модом", "модульн"), "Модульный дом"},
	{rx.AnyWordStart("сафари", "шатёр", "шатер", "тент"), "Сафари-тент"},
	{rx.AnyWordStart("кэмпер", "camper", "автодом"), "Кэмпер"},
}

// parseRoomsWithSpecs — домики объекта из встроенного JSON. Единая точка
// разбора: страница весит сотни килобайт, и повторять по ней поиск JSON ради
// каждого поля незачем.
func parseRoomsWithSpecs(page string) map[string]pv12RoomWithSpecs {
	raw := balancedJSON(page, roomDetailsMarker, '{', '}')
	if raw == "" {
		return nil
	}
	var rooms map[string]pv12RoomWithSpecs
	if err := json.Unmarshal([]byte(raw), &rooms); err != nil {
		return nil
	}
	return rooms
}

// houseTypesFromRooms — формы жилья по названиям домиков объекта.
//
// Возвращает все найденные: у объекта с куполами и барнхаусами гость должен
// находиться и в том разделе, и в другом. Порядок стабилен — по порядку правил,
// а не по порядку домиков в JSON (тот у map произвольный).
func houseTypesFromRooms(rooms map[string]pv12RoomWithSpecs) []string {
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

// parseRoom — характеристики ОДНОГО домика.
//
// Вместимость: сначала то, что источник назвал прямо в описании, и только при
// его молчании — арифметика по спальным местам. Заявление всегда точнее: оно
// учитывает и правила объекта, и то, что кровать бывает детской.
func parseRoom(room pv12RoomWithSpecs) roomSpecs {
	out := parseRoomSpecs(room.Specs)
	if m := declaredCapacityRe.FindStringSubmatch(room.Desc); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			out.Guests = n
		}
	}
	return out
}

// parseRoomSpecs — площадь и вместимость домика по строкам характеристик.
func parseRoomSpecs(specs []string) roomSpecs {
	var out roomSpecs
	for _, s := range specs {
		if m := areaSpecRe.FindStringSubmatch(s); m != nil {
			// Дробную часть отбрасываем: в карточке площадь показывается целыми
			// метрами, и «32.5» от «32» гостя ни к чему не обязывает.
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				// Складываем: у домика площадь одна строкой, но если источник
				// перечислил комнаты по отдельности, гостю важна общая.
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
func detailRoomSpecs(rooms map[string]pv12RoomWithSpecs) roomSpecs {
	var best roomSpecs
	for _, room := range rooms {
		s := parseRoom(room)
		if s.Guests > best.Guests {
			best.Guests = s.Guests
		}
		if s.AreaM2 > best.AreaM2 {
			best.AreaM2 = s.AreaM2
		}
	}
	return best
}
