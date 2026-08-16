// Package avito — провайдер объявлений Авито из страниц, сохранённых человеком.
//
// Почему из файлов, а не из сети. Пользовательское соглашение Авито запрещает
// автоматический сбор, и бан прилетает аккаунту и сети, откуда шли запросы, —
// то есть ровно тому аккаунту, через который идёт переписка с владельцами
// домиков. Провайдер не делает НИ ОДНОГО обращения к Авито: человек открывает
// объявление в браузере, сохраняет страницу («Сохранить как» → «Веб-страница
// полностью») и кладёт .html в папку. Дальше — только чтение с диска.
//
// Это и есть тот случай, про который в README сказано «Avito, который ботами не
// парсится»: механизм «ручных» домиков там существует давно, здесь он просто
// перестал быть ручным набором текста.
package avito

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"vk-parser/internal/contract"
)

// Provider читает сохранённые страницы из каталога dir.
type Provider struct {
	dir string
}

// New собирает провайдера над каталогом с сохранёнными страницами.
func New(dir string) *Provider { return &Provider{dir: dir} }

// Name — имя источника (каталог вывода generated/avito).
func (p *Provider) Name() string { return "avito" }

// Parse читает все *.html каталога и собирает из них карточки.
//
// Ошибка возвращается только на фатальном: каталог не задан или недоступен.
// Сбой отдельного файла (не та страница, обрезанная копия) логируется и
// пропускается — из-за одной кривой копии не должен пропадать сбор остальных.
func (p *Provider) Parse(ctx context.Context) ([]contract.Object, error) {
	if strings.TrimSpace(p.dir) == "" {
		return nil, fmt.Errorf("avito: не задан каталог со страницами (--input)")
	}

	files, err := htmlFiles(p.dir)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("avito: в каталоге %s нет ни одного .html", p.dir)
	}

	objects := make([]contract.Object, 0, len(files))
	seen := make(map[int]bool, len(files))

	for _, path := range files {
		// Отмену проверяем между файлами: разбор одного файла — это чтение
		// мегабайта с диска, прерывать его на середине незачем.
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		obj, err := parseFile(path)
		if err != nil {
			slog.Warn("avito: страница пропущена", "file", filepath.Base(path), "err", err)
			continue
		}
		if seen[obj.SourceID] {
			slog.Warn("avito: дубль объявления", "file", filepath.Base(path), "sourceId", obj.SourceID)
			continue
		}
		seen[obj.SourceID] = true
		objects = append(objects, obj)
		slog.Info("avito: объявление разобрано", "sourceId", obj.SourceID, "title", obj.Title, "фото", len(obj.Photos))
	}

	if len(objects) == 0 {
		return nil, fmt.Errorf("avito: ни одна из %d страниц не разобрана", len(files))
	}
	dedupeSlugs(objects)
	return objects, nil
}

// parseFile читает файл и превращает его в карточку.
func parseFile(path string) (contract.Object, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return contract.Object{}, fmt.Errorf("прочитать файл: %w", err)
	}
	bi, err := extractBuyerItem(string(data))
	if err != nil {
		return contract.Object{}, err
	}
	return toObject(bi), nil
}

// htmlFiles — отсортированный список .html каталога (без обхода вложенных:
// рядом с каждой страницей Chrome кладёт папку «<имя>_files» с превью и
// скриптами — там нам делать нечего).
func htmlFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("avito: открыть каталог %s: %w", dir, err)
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(e.Name()), ".html") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

// dedupeSlugs разводит тёзок: у Авито «Отдых для двоих» — название на десяток
// объявлений сразу, а слаг обязан быть уникальным (по нему строится адрес
// страницы). Первому по порядку слаг остаётся как есть, следующим дописывается
// id источника — тот самый случай, ради которого SourceID и живёт в контракте.
func dedupeSlugs(objects []contract.Object) {
	taken := make(map[string]bool, len(objects))
	for i := range objects {
		s := objects[i].Slug
		if s == "" {
			s = fmt.Sprintf("avito-%d", objects[i].SourceID)
		}
		if taken[s] {
			s = fmt.Sprintf("%s-%d", s, objects[i].SourceID)
		}
		taken[s] = true
		objects[i].Slug = s
	}
}
