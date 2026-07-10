// Package catalog — репозиторий собранных провайдерами объектов для каталожного
// API (/api/v1/glampings). Источник — файлы generated/<provider>/objects.json,
// которые пишет `--provider=<name>`; репозиторий индексирует их по slug и
// перечитывает при изменении файлов (hot-reload по mtime) — сервер отдаёт свежие
// данные после каждого пересбора без рестарта.
package catalog

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"vk-parser/internal/contract"
)

// objectsFile — имя файла выдачи провайдера внутри generated/<name>/.
const objectsFile = "objects.json"

// Repository — потокобезопасный индекс объектов по slug поверх каталога dir
// (обычно "generated"). Создаётся один раз, переиспользуется хендлерами.
type Repository struct {
	dir string

	mu      sync.RWMutex
	bySlug  map[string]contract.Object
	order   []string    // слаги в порядке загрузки (стабильный список для главной)
	loaded  time.Time   // когда индекс собран
	sources []time.Time // mtime файлов на момент загрузки (для hot-reload)
}

// New создаёт репозиторий над каталогом dir и делает первую загрузку.
// Отсутствие каталога/файлов — не ошибка (пустой каталог, WARN): сервер может
// подниматься раньше первого сбора провайдеров.
func New(dir string) *Repository {
	r := &Repository{dir: dir}
	if err := r.reload(); err != nil {
		slog.Warn("catalog: первичная загрузка", "dir", dir, "err", err)
	}
	return r
}

// Get возвращает полную карточку по slug (перечитав файлы, если они изменились).
func (r *Repository) Get(slug string) (contract.Object, bool) {
	r.refreshIfStale()
	r.mu.RLock()
	defer r.mu.RUnlock()
	obj, ok := r.bySlug[slug]
	return obj, ok
}

// List возвращает превью всех объектов (плитки главной страницы).
func (r *Repository) List() []contract.Preview {
	r.refreshIfStale()
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]contract.Preview, 0, len(r.order))
	for _, s := range r.order {
		out = append(out, r.bySlug[s].ToPreview())
	}
	return out
}

// refreshIfStale перечитывает данные, если состав/время файлов изменились.
// Дёшево: только os.Stat по паре файлов на запрос; JSON парсится лишь при
// реальном изменении.
func (r *Repository) refreshIfStale() {
	r.mu.RLock()
	stale := !equalTimes(r.sources, statTimes(r.files()))
	r.mu.RUnlock()
	if !stale {
		return
	}
	if err := r.reload(); err != nil {
		slog.Warn("catalog: перечитка не удалась, отдаю прежние данные", "err", err)
	}
}

// reload загружает все generated/*/objects.json и перестраивает индекс.
func (r *Repository) reload() error {
	files := r.files()
	bySlug := make(map[string]contract.Object)
	order := make([]string, 0)

	for _, f := range files {
		objs, err := readObjects(f)
		if err != nil {
			slog.Warn("catalog: файл пропущен", "file", f, "err", err)
			continue
		}
		for _, o := range objs {
			if o.Slug == "" {
				slog.Warn("catalog: объект без slug пропущен", "file", f, "title", o.Title)
				continue
			}
			if _, dup := bySlug[o.Slug]; dup {
				slog.Warn("catalog: дубль slug пропущен", "slug", o.Slug, "file", f)
				continue
			}
			bySlug[o.Slug] = o
			order = append(order, o.Slug)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.bySlug, r.order = bySlug, order
	r.loaded = time.Now()
	r.sources = statTimes(files)
	slog.Info("catalog: индекс собран", "объектов", len(order), "файлов", len(files))
	return nil
}

// files — список файлов выдачи всех провайдеров (generated/*/objects.json).
func (r *Repository) files() []string {
	matches, err := filepath.Glob(filepath.Join(r.dir, "*", objectsFile))
	if err != nil {
		return nil
	}
	return matches
}

// readObjects читает и парсит один файл выдачи провайдера.
func readObjects(path string) ([]contract.Object, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("catalog: read %s: %w", path, err)
	}
	var objs []contract.Object
	if err := json.Unmarshal(raw, &objs); err != nil {
		return nil, fmt.Errorf("catalog: parse %s: %w", path, err)
	}
	return objs, nil
}

// statTimes — mtime файлов (нулевое время для отсутствующих): отпечаток
// состояния источников для сравнения в refreshIfStale.
func statTimes(files []string) []time.Time {
	out := make([]time.Time, len(files))
	for i, f := range files {
		if st, err := os.Stat(f); err == nil {
			out[i] = st.ModTime()
		}
	}
	return out
}

func equalTimes(a, b []time.Time) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}
