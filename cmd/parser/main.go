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

// GlampingData — контракт ответа для фронтенда. Поля экспортируемые (с Большой
// буквы), теги задают lowercase-ключи JSON.
type GlampingData struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Price       string   `json:"price"`
	Photos      []string `json:"photos"`
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("startup: %v", err)
	}
	client := vk.NewClient(cfg.VKToken)
	store := cache.New[GlampingData](cacheTTL)

	// Роутер. Go 1.22+ умеет метод+путь прямо в паттерне ("GET /api/...").
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/glamping", handleGlamping(client, store))

	// СВОЙ http.Server с таймаутами. НЕ используем http.ListenAndServe(...)
	// без настроек: у него нет таймаутов → уязвимость к «медленным» клиентам.
	srv := &http.Server{
		Addr:         serverAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second, // поход в VK может быть небыстрым
		IdleTimeout:  60 * time.Second,
	}

	// ctx отменяется при SIGINT (Ctrl+C) или SIGTERM (docker stop / systemd).
	// signal.NotifyContext — идиома Go 1.16+: сигнал → отмена контекста.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Сервер слушает в отдельной горутине, чтобы main мог параллельно ждать
	// сигнал. ListenAndServe блокирует, поэтому его нельзя звать в main напрямую.
	go func() {
		log.Printf("VK parser слушает на %s", serverAddr)
		// При штатной остановке Shutdown заставляет ListenAndServe вернуть
		// ErrServerClosed — это НЕ ошибка, поэтому отсеиваем её.
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	// Блокируемся до сигнала.
	<-ctx.Done()
	log.Println("получен сигнал остановки, завершаем…")

	// Даём до 10 секунд на доработку текущих запросов, потом обрываем.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown: %v", err)
	}
	log.Println("сервер остановлен корректно")
}

// handleGlamping — фабрика хендлера: принимает зависимость (*vk.Client) и
// возвращает http.HandlerFunc-замыкание. Это идиоматичный для Go способ
// «прокинуть» зависимость в обработчик (вместо глобальной переменной).
func handleGlamping(client *vk.Client, store *cache.Cache[GlampingData]) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain := r.URL.Query().Get("domain")
		if domain == "" {
			// Ошибка клиента → 400 Bad Request.
			http.Error(w, "query param 'domain' is required", http.StatusBadRequest)
			return
		}

		// Cache hit — отдаём из памяти, в VK не ходим.
		if data, ok := store.Get(domain); ok {
			w.Header().Set("X-Cache", "HIT")
			writeJSON(w, data)
			return
		}

		// Cache miss → идём в VK. r.Context() отменяется, когда клиент закрыл
		// соединение → отмена долетит до VK-запросов внутри buildGlampingData.
		data, err := buildGlampingData(r.Context(), client, domain)
		if err != nil {
			// Сбой на нашей стороне / на стороне VK → 502 Bad Gateway.
			// Наружу не светим детали ошибки, в лог — пишем полностью.
			log.Printf("build data for %q: %v", domain, err)
			http.Error(w, "failed to fetch data from VK", http.StatusBadGateway)
			return
		}

		store.Set(domain, data)
		w.Header().Set("X-Cache", "MISS")
		writeJSON(w, data)
	}
}

// writeJSON — единая запись JSON-ответа. Вынесена, чтобы не дублировать
// кодирование для веток HIT и MISS (DRY). Кодируем потоково прямо в
// ResponseWriter (он реализует io.Writer).
func writeJSON(w http.ResponseWriter, v any) {
	// Content-Type ставим ДО тела (после первого Write заголовки уже ушли).
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}

// buildGlampingData — вся «бизнес-логика» одного запроса: резолвим домен,
// тянем фото и товар, собираем контракт. Отделена от HTTP, чтобы её можно было
// переиспользовать (например, в тесте) и не мешать транспорт с логикой.
func buildGlampingData(ctx context.Context, client *vk.Client, domain string) (GlampingData, error) {
	ownerID, err := client.ResolveOwnerID(ctx, domain)
	if err != nil {
		return GlampingData{}, fmt.Errorf("resolve: %w", err)
	}

	photos, err := client.GetPhotos(ctx, ownerID)
	if err != nil {
		return GlampingData{}, fmt.Errorf("photos: %w", err)
	}

	item, err := client.GetMarketItemByID(ctx, productID)
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
