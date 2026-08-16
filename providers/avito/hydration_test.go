package avito

import (
	"errors"
	"strings"
	"testing"
)

// Извлечение состояния — самая хрупкая часть провайдера: она держится на
// формате чужой страницы, который меняется без предупреждения. Фикстуры
// проверяют только happy path, поэтому граничные случаи собраны здесь вручную.

// page заворачивает готовый JS-литерал в страницу, как это делает Авито.
func page(literal string) string {
	return `<html><body><script>window.__staticRouterHydrationData = JSON.parse(` +
		literal + `);</script></body></html>`
}

// state собирает литерал состояния с объявлением внутри: JSON, закодированный
// внутри JSON-строки, — ровно та двойная упаковка, что лежит на странице.
func state(itemJSON string) string {
	inner := `{"loaderData":{"catalog-or-main-or-item":{"buyerItem":{"item":` + itemJSON + `}}}}`
	return quoteJS(inner)
}

// quoteJS кодирует строку как JS/JSON-литерал.
func quoteJS(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return `"` + r.Replace(s) + `"`
}

func TestExtractBuyerItem(t *testing.T) {
	t.Run("объявление достаётся", func(t *testing.T) {
		bi, err := extractBuyerItem(page(state(`{"id":123,"title":"Дом"}`)))
		if err != nil {
			t.Fatalf("extractBuyerItem: %v", err)
		}
		if bi.Item.ID != 123 || bi.Item.Title != "Дом" {
			t.Fatalf("item = %+v", bi.Item)
		}
	})

	// Кавычки и обратные слэши внутри описания — обычное дело, и именно на них
	// ломается наивный поиск закрывающей кавычки регуляркой.
	t.Run("экранированные кавычки в тексте", func(t *testing.T) {
		bi, err := extractBuyerItem(page(state(`{"id":1,"title":"Дом \"У озера\""}`)))
		if err != nil {
			t.Fatalf("extractBuyerItem: %v", err)
		}
		if bi.Item.Title != `Дом "У озера"` {
			t.Fatalf("Title = %q", bi.Item.Title)
		}
	})

	t.Run("обратный слэш перед закрывающей кавычкой", func(t *testing.T) {
		bi, err := extractBuyerItem(page(state(`{"id":1,"title":"Путь C:\\"}`)))
		if err != nil {
			t.Fatalf("extractBuyerItem: %v", err)
		}
		if bi.Item.Title != `Путь C:\` {
			t.Fatalf("Title = %q", bi.Item.Title)
		}
	})

	// Описания содержат и `);` — на нём обрывался бы нежадный шаблон.
	t.Run("скобка со точкой с запятой внутри текста", func(t *testing.T) {
		bi, err := extractBuyerItem(page(state(`{"id":1,"title":"Баня (2 этажа); купель"}`)))
		if err != nil {
			t.Fatalf("extractBuyerItem: %v", err)
		}
		if bi.Item.Title != "Баня (2 этажа); купель" {
			t.Fatalf("Title = %q", bi.Item.Title)
		}
	})

	// Форматирование скрипта — не наше дело: ищем имя переменной, а дальше
	// ближайший вызов. Иначе минификация на стороне Авито обнулила бы разбор
	// всех страниц разом, уже после того, как человек их сохранил.
	t.Run("пробелы вокруг присваивания не важны", func(t *testing.T) {
		for name, script := range map[string]string{
			"минифицировано": `window.__staticRouterHydrationData=JSON.parse(` + state(`{"id":7}`) + `);`,
			"лишние пробелы": `window.__staticRouterHydrationData   =   JSON.parse( ` + state(`{"id":7}`) + ` );`,
			"перенос строки": "window.__staticRouterHydrationData =\n  JSON.parse(" + state(`{"id":7}`) + ");",
		} {
			t.Run(name, func(t *testing.T) {
				bi, err := extractBuyerItem("<script>" + script + "</script>")
				if err != nil {
					t.Fatalf("extractBuyerItem: %v", err)
				}
				if bi.Item.ID != 7 {
					t.Fatalf("id = %d", bi.Item.ID)
				}
			})
		}
	})
}

func TestExtractBuyerItemErrors(t *testing.T) {
	t.Run("не страница объявления", func(t *testing.T) {
		_, err := extractBuyerItem("<html>привет</html>")
		if !errors.Is(err, errNoHydration) {
			t.Fatalf("ожидал errNoHydration, получил %v", err)
		}
	})

	// Копия, оборванная на середине сохранения: разбор обязан сказать это
	// внятно, иначе человек будет искать причину в самом объявлении.
	t.Run("файл обрезан", func(t *testing.T) {
		// Режем так, чтобы маркер остался, а закрывающая кавычка литерала —
		// нет: именно так выглядит копия, оборванная на середине сохранения.
		full := page(state(`{"id":1,"title":"Дом"}`))
		_, err := extractBuyerItem(full[:len(full)-30])
		if err == nil || errors.Is(err, errNoHydration) {
			t.Fatalf("ожидал ошибку про незакрытую строку, получил %v", err)
		}
	})

	t.Run("состояние без маршрута объявления", func(t *testing.T) {
		_, err := extractBuyerItem(page(quoteJS(`{"loaderData":{"other-route":{}}}`)))
		if err == nil || !strings.Contains(err.Error(), "не страница объявления") {
			t.Fatalf("ожидал ошибку про маршрут, получил %v", err)
		}
	})

	t.Run("маршрут без объявления", func(t *testing.T) {
		_, err := extractBuyerItem(page(quoteJS(`{"loaderData":{"catalog-or-main-or-item":{}}}`)))
		if err == nil || !strings.Contains(err.Error(), "buyerItem") {
			t.Fatalf("ожидал ошибку про buyerItem, получил %v", err)
		}
	})

	// id — единственное, что связывает карточку с источником; объявление без
	// него дальше по конвейеру сломает и дедуп слагов, и стыковку прогонов.
	t.Run("объявление без id", func(t *testing.T) {
		_, err := extractBuyerItem(page(state(`{"title":"Дом без id"}`)))
		if err == nil {
			t.Fatal("ожидал ошибку: объявление без id")
		}
	})

	t.Run("битый JSON внутри состояния", func(t *testing.T) {
		_, err := extractBuyerItem(page(quoteJS(`{"loaderData": не json}`)))
		if err == nil || !strings.Contains(err.Error(), "разобрать состояние") {
			t.Fatalf("ожидал ошибку разбора, получил %v", err)
		}
	})
}
