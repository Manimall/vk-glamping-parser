package main

// Каталожный API v1 для SPA-сценария фронта («скелетон → fetch → отрисовка»):
//   GET /api/v1/glampings         → превью всех объектов (плитки главной)
//   GET /api/v1/glampings/{slug}  → полная карточка (цены, фото, SEO/OG, обложка)
// Источник — репозиторий catalog поверх generated/ (сбор `--provider=<name>`);
// hot-reload по mtime: после пересбора сервер отдаёт свежие данные без рестарта.

import (
	"net/http"

	"vk-parser/internal/catalog"
)

// catalogAPI — HTTP-обёртка над репозиторием каталога (тонкая: роутинг + JSON).
type catalogAPI struct {
	repo *catalog.Repository
}

// handleList — GET /api/v1/glampings: превью для главной страницы.
func (a *catalogAPI) handleList(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, a.repo.List())
}

// handleGet — GET /api/v1/glampings/{slug}: полная карточка или 404.
//
// [Go для изучения] {slug} в шаблоне маршрута и r.PathValue("slug") — нативный
// роутинг стандартной библиотеки (Go 1.22+): аналог :slug и req.params.slug в
// Express, только без единой внешней зависимости.
func (a *catalogAPI) handleGet(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	obj, ok := a.repo.Get(slug)
	if !ok {
		http.Error(w, `{"error":"glamping not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, obj)
}

// withCORS разрешает браузерные запросы фронта с другого origin (SPA дергает
// API напрямую из браузера). API только читает публичные данные — «*» безопасно.
//
// [Go для изучения] Классическая миддлвара по-гошному: функция принимает handler
// и возвращает НОВЫЙ handler-обёртку (замыкание) — как app.use() в Express, но
// это просто композиция функций без фреймворка. http.HandlerFunc — адаптер,
// превращающий обычную функцию в тип с методом ServeHTTP (реализацию интерфейса
// http.Handler).
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions { // preflight
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
