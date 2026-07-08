// Package objects читает пер-объектные конфиги data/<domain>.json — ручные
// данные, которых нет в VK API: координаты, ссылка на карту, id товаров-домиков
// и «ручные» домики (например, описание с Avito, который ботами не парсится).
package objects

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Cabin — «сырой» домик из конфига: заголовок, цена, описание текстом. Структуру
// удобств из описания делает уже извлекатель (extract), здесь только сырьё.
type Cabin struct {
	Title       string `json:"title"`
	Price       string `json:"price"`
	Description string `json:"description"`
}

// Object — конфиг одного глэмпинга. Все поля необязательные: чего нет — то пусто.
type Object struct {
	Address string   `json:"address"` // адрес (VK у юзеров не отдаёт)
	Coords  string   `json:"coords"`  // "lat,lon"
	Map     string   `json:"map"`     // ссылка на карту
	Items   []string `json:"items"`   // товары-домики VK (URL/id)
	Extras  []string `json:"extras"`  // товары-услуги VK (фурако, наполнение…): доп.услуги, не домики
	Cabins  []Cabin  `json:"cabins"`  // ручные домики (не из VK)
}

// Load читает data/<domain>.json. Если файла нет — это НЕ ошибка: возвращаем
// (nil, nil), и вызывающий просто работает без ручных данных. Так конфиг —
// дополнение, а не обязательное условие.
func Load(dir, domain string) (*Object, error) {
	path := filepath.Join(dir, domain+".json")

	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("objects: read %s: %w", path, err)
	}

	var obj Object
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("objects: parse %s: %w", path, err)
	}
	return &obj, nil
}
