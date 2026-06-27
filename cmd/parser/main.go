// Command parser — HTTP-микросервис: по запросу собирает карточку объекта из
// VK и отдаёт её JSON-ом. Фронтенд ходит сюда вместо чтения статичного файла.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vk-parser/internal/cache"
	"vk-parser/internal/config"
	"vk-parser/internal/vk"
)

const (
	serverAddr = ":8080"
	// productID — пока конкретный товар ЁлкиДом. В «боевом» сервисе id товара
	// резолвился бы по домену (отдельная задача), здесь фиксируем для демо.
	productID = "-211011668_6377368"
	// cacheTTL — сколько держим карточку в кэше, прежде чем перепарсить VK.
	cacheTTL = 5 * time.Minute
)

// GlampingData — контракт ответа для фронтенда.
type GlampingData struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Price       string   `json:"price"`
	Photos      []string `json:"photos"`
}

// server держит ОБЩИЕ зависимости приложения. Хендлеры — это методы server,
// поэтому они берут зависимости из ресивера (s.client, s.store), а не из
// длинного списка аргументов. Новая зависимость = новое поле здесь, без правки
// сигнатур хендлеров.
type server struct {
	client *vk.Client
	store  *cache.Cache[GlampingData]
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("startup: %v", err)
	}

	// Composition root: единственное место, где собираем граф зависимостей.
	srv := &server{
		client: vk.NewClient(cfg.VKToken),
		store:  cache.New[GlampingData](cacheTTL),
	}

	// Роутер. srv.handleGlamping — это «method value»: метод, привязанный к srv,
	// который сам по себе реализует http.HandlerFunc. Фабрика-замыкание больше
	// не нужна — зависимости уже внутри srv.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/glamping", srv.handleGlamping)

	// Транспорт (http.Server) — отдельная сущность от нашего server. Свой сервер
	// с таймаутами (НЕ http.ListenAndServe без настроек — он без таймаутов).
	httpServer := &http.Server{
		Addr:         serverAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second, // поход в VK может быть небыстрым
		IdleTimeout:  60 * time.Second,
	}

	// ctx отменяется при SIGINT (Ctrl+C) или SIGTERM (docker stop / systemd).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Слушаем в отдельной горутине, чтобы main мог ждать сигнал.
	go func() {
		log.Printf("VK parser слушает на %s", serverAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("получен сигнал остановки, завершаем…")

	// Даём до 10с на доработку текущих запросов, потом обрываем.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown: %v", err)
	}
	log.Println("сервер остановлен корректно")
}

// handleGlamping — теперь МЕТОД server. Сигнатура — ровно http.HandlerFunc
// (w, r), без зависимостей в аргументах: они доступны через ресивер s.
func (s *server) handleGlamping(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		http.Error(w, "query param 'domain' is required", http.StatusBadRequest)
		return
	}

	// Cache hit — отдаём из памяти, в VK не ходим.
	if data, ok := s.store.Get(domain); ok {
		w.Header().Set("X-Cache", "HIT")
		writeJSON(w, data)
		return
	}

	// Cache miss → идём в VK. r.Context() отменяется, если клиент отвалился.
	data, err := s.buildGlampingData(r.Context(), domain)
	if err != nil {
		log.Printf("build data for %q: %v", domain, err)
		http.Error(w, "failed to fetch data from VK", http.StatusBadGateway)
		return
	}

	s.store.Set(domain, data)
	w.Header().Set("X-Cache", "MISS")
	writeJSON(w, data)
}

// buildGlampingData — тоже метод server: «бизнес-логика» одного запроса.
// Берёт VK-клиент из ресивера (s.client), HTTP не знает.
func (s *server) buildGlampingData(ctx context.Context, domain string) (GlampingData, error) {
	ownerID, err := s.client.ResolveOwnerID(ctx, domain)
	if err != nil {
		return GlampingData{}, fmt.Errorf("resolve: %w", err)
	}

	photos, err := s.client.GetPhotos(ctx, ownerID)
	if err != nil {
		return GlampingData{}, fmt.Errorf("photos: %w", err)
	}

	item, err := s.client.GetMarketItemByID(ctx, productID)
	if err != nil {
		return GlampingData{}, fmt.Errorf("market item: %w", err)
	}

	return GlampingData{
		Title:       item.Title,
		Description: item.Description,
		Price:       item.Price.Text,
		Photos:      photos,
	}, nil
}

// writeJSON — без зависимостей, поэтому остаётся обычной функцией (не методом).
// DRY: одна запись JSON-ответа для веток HIT и MISS.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}
