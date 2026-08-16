package avito

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Извлечение данных объявления из сохранённой человеком страницы Авито.
//
// Ключевой факт разведки: вёрстку разбирать не нужно. Авито — React-приложение,
// и в конце <body> оно печатает ПОЛНОЕ состояние страницы:
//
//	window.__staticRouterHydrationData = JSON.parse("{…}");
//
// Внутри — те же поля, что видит браузер: цена, адрес, координаты, все ссылки на
// фото в 1280×960, прайс-лист. Разбор регулярками по классам вёрстки (они у
// Авито генерируемые, вида "_8d1c213637ecd63a") сломался бы на первом же
// релизе площадки; JSON-состояние пережило три разные страницы, сохранённые в
// разные дни, без единого расхождения в структуре.

// hydrationMarker — начало блока состояния. Ищем именно присваивание, а не
// просто "JSON.parse(": на странице есть и другие вызовы parse.
const hydrationMarker = `window.__staticRouterHydrationData = JSON.parse(`

// itemRouteKey — ключ маршрута страницы объявления в loaderData. Назван так у
// самого Авито (один компонент обслуживает каталог, главную и карточку).
const itemRouteKey = "catalog-or-main-or-item"

// errNoHydration — в файле нет блока состояния. Отдельная ошибка, чтобы
// вызывающий мог отличить «не та страница» от «формат сломался».
var errNoHydration = errors.New("не найден window.__staticRouterHydrationData")

// extractBuyerItem достаёт объявление из HTML сохранённой страницы.
//
// Два разбора JSON подряд — не избыточность: в HTML лежит JS-литерал строки
// ("{\"loaderData\"…}"), то есть JSON, ЗАКОДИРОВАННЫЙ внутри JSON-строки.
// Первый Unmarshal снимает экранирование, второй читает сам объект.
func extractBuyerItem(html string) (*buyerItem, error) {
	literal, err := findJSONStringLiteral(html)
	if err != nil {
		return nil, err
	}

	var inner string
	if err := json.Unmarshal([]byte(literal), &inner); err != nil {
		return nil, fmt.Errorf("avito: раскодировать строку состояния: %w", err)
	}

	// Промежуточная карта, а не типизированная структура: ключ маршрута
	// (itemRouteKey) содержит дефисы и не годится для имени поля Go, а json-тег
	// вложить внутрь анонимной структуры на два уровня — нечитаемо.
	var root struct {
		LoaderData map[string]struct {
			BuyerItem *buyerItem `json:"buyerItem"`
		} `json:"loaderData"`
	}
	if err := json.Unmarshal([]byte(inner), &root); err != nil {
		return nil, fmt.Errorf("avito: разобрать состояние: %w", err)
	}

	route, ok := root.LoaderData[itemRouteKey]
	if !ok {
		return nil, fmt.Errorf("avito: в состоянии нет маршрута %q — это не страница объявления", itemRouteKey)
	}
	if route.BuyerItem == nil || route.BuyerItem.Item.ID == 0 {
		return nil, errors.New("avito: в состоянии нет объявления (buyerItem.item)")
	}
	return route.BuyerItem, nil
}

// findJSONStringLiteral возвращает JS-литерал строки после JSON.parse( — вместе
// с обрамляющими кавычками, готовый к json.Unmarshal в string.
//
// Почему сканирование вручную, а не регулярка: описания объявлений содержат и
// `);`, и экранированные кавычки. Нежадный шаблон вида `"(.*?)"\)` обрезал бы
// строку на первом же \" внутри текста, а жадный — захватил бы полстраницы.
// Здесь мы идём по символам и считаем экранирование, как это делает парсер JS.
func findJSONStringLiteral(html string) (string, error) {
	at := strings.Index(html, hydrationMarker)
	if at < 0 {
		return "", errNoHydration
	}
	rest := html[at+len(hydrationMarker):]

	open := strings.IndexByte(rest, '"')
	if open < 0 {
		return "", errNoHydration
	}

	// Идём от символа после открывающей кавычки. Обратный слэш экранирует
	// следующий символ (в том числе самого себя), поэтому его пропускаем парой.
	for i := open + 1; i < len(rest); i++ {
		switch rest[i] {
		case '\\':
			i++ // экранированный символ закрыть строку не может
		case '"':
			return rest[open : i+1], nil
		}
	}
	return "", errors.New("avito: строка состояния не закрыта — файл обрезан?")
}
