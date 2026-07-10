package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
)

// reDomain — допустимый VK screen name: буквы, цифры, точка, подчёркивание.
// Отсекает path-traversal ("../x" содержит "/") в objects.Load и мусор в VK.
var reDomain = regexp.MustCompile(`^[A-Za-z0-9_.]+$`)

// isValidDomain — domain пришёл из URL и идёт в файловый путь + VK, поэтому
// валидируем его перед использованием.
func isValidDomain(domain string) bool {
	return reDomain.MatchString(domain)
}

// writeJSON — без зависимостей, поэтому остаётся обычной функцией (не методом).
// DRY: одна запись JSON-ответа для веток HIT и MISS.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Error("encode response failed", "err", err)
	}
}
