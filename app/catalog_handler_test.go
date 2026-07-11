package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"vk-parser/internal/catalog"
	"vk-parser/internal/contract"
)

// newTestAPI собирает catalogAPI над временным generated/ с одним объектом.
func newTestAPI(t *testing.T) *catalogAPI {
	t.Helper()
	dir := t.TempDir()
	pdir := filepath.Join(dir, "vk")
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	objs := []contract.Object{{
		Slug:   "aframe-ivanovo",
		Title:  "AFRAME",
		Cover:  "https://s/cover.webp",
		Photos: []string{"https://s/1.webp"},
		Cabins: []contract.Cabin{{Title: "AFRAME", Price: "7 000 ₽"}},
	}}
	raw, _ := json.Marshal(objs)
	if err := os.WriteFile(filepath.Join(pdir, "objects.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return &catalogAPI{repo: catalog.New(dir)}
}

// newTestMux — роутер с маршрутами v1 + CORS (как в main).
func newTestMux(api *catalogAPI) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/glampings", api.handleList)
	mux.HandleFunc("GET /api/v1/glampings/{slug}", api.handleGet)
	return withCORS(mux)
}

func TestCatalogGetBySlug(t *testing.T) {
	h := newTestMux(newTestAPI(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/glampings/aframe-ivanovo", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("CORS-заголовок отсутствует: %q", got)
	}
	var obj contract.Object
	if err := json.NewDecoder(rec.Body).Decode(&obj); err != nil {
		t.Fatal(err)
	}
	if obj.Title != "AFRAME" || obj.Cabins[0].Price != "7 000 ₽" || obj.Cover == "" {
		t.Errorf("карточка неполная: %+v", obj)
	}
}

func TestCatalogNotFound(t *testing.T) {
	h := newTestMux(newTestAPI(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/glampings/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, ожидал 404", rec.Code)
	}
}

func TestCatalogList(t *testing.T) {
	h := newTestMux(newTestAPI(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/glampings", nil))

	var list []contract.Preview
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Slug != "aframe-ivanovo" || list[0].Price != "7 000 ₽" {
		t.Fatalf("превью: %+v", list)
	}
}

func TestCORSPreflight(t *testing.T) {
	h := newTestMux(newTestAPI(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodOptions, "/api/v1/glampings", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, ожидал 204", rec.Code)
	}
}
