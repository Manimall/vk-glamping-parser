package main

import (
	"log/slog"
	"net/http"

	vkprovider "vk-parser/providers/vk"
)

// handleGlamping — HTTP-обёртка над VK-провайдером: разбирает и валидирует query,
// кэширует ответ, а сам сбор карточки делегирует providers/vk (вся VK-логика там).
func (s *server) handleGlamping(w http.ResponseWriter, r *http.Request) {
	q := glampingQuery{
		domain: r.URL.Query().Get("domain"),
		items:  r.URL.Query().Get("items"),
		coords: r.URL.Query().Get("coords"),
		mapURL: r.URL.Query().Get("map"),
	}
	if q.domain == "" {
		http.Error(w, "query param 'domain' is required", http.StatusBadRequest)
		return
	}
	// domain идёт и в VK-вызовы, и в путь файла конфига (objects.Load), поэтому
	// валидируем: только буквы/цифры/._ — отсекаем path-traversal ("../secret").
	if !isValidDomain(q.domain) {
		http.Error(w, "invalid 'domain'", http.StatusBadRequest)
		return
	}

	// Cache hit — отдаём из памяти, в VK не ходим.
	if data, ok := s.store.Get(q.cacheKey()); ok {
		w.Header().Set("X-Cache", "HIT")
		writeJSON(w, data)
		return
	}

	// Cache miss → собираем через VK-провайдер. r.Context() отменяется, если клиент
	// отвалился. Обработчик лишь маппит query в vkprovider.Query.
	data, err := s.parser.Build(r.Context(), vkprovider.Query{
		Domain: q.domain,
		Items:  q.items,
		Coords: q.coords,
		MapURL: q.mapURL,
	})
	if err != nil {
		slog.Warn("build data failed", "domain", q.domain, "err", err)
		http.Error(w, "failed to fetch data from VK", http.StatusBadGateway)
		return
	}

	s.store.Set(q.cacheKey(), data)
	w.Header().Set("X-Cache", "MISS")
	writeJSON(w, data)
}
