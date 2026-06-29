// Command parser — HTTP-микросервис: по запросу собирает карточку объекта из
// VK и отдаёт её JSON-ом. Фронтенд ходит сюда вместо чтения статичного файла.
//
// Код пакета разбит по файлам: main.go — точка входа и сборка зависимостей;
// types.go — структуры данных ответа; handler.go — обработка запроса;
// helpers.go — мелкие чистые функции (парсинг, дедуп, запись JSON).
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vk-parser/internal/cache"
	"vk-parser/internal/config"
	"vk-parser/internal/extract"
	"vk-parser/internal/geocode"
	"vk-parser/internal/vk"
)

const (
	serverAddr = ":8080"
	// cacheTTL — сколько держим карточку в кэше, прежде чем перепарсить VK.
	cacheTTL = 5 * time.Minute
	// dataDir — каталог с пер-объектными конфигами data/<domain>.json.
	dataDir = "data"
	// maxPhotos — сколько лучших фото оставляем в галерее.
	maxPhotos = 15
)

// server держит ОБЩИЕ зависимости приложения. Хендлеры — это методы server,
// поэтому они берут зависимости из ресивера (s.client, s.store), а не из
// длинного списка аргументов. Новая зависимость = новое поле здесь, без правки
// сигнатур хендлеров.
type server struct {
	// client и geocoder — ИНТЕРФЕЙСЫ (см. deps.go), а не конкретные типы. server
	// не привязан к *vk.Client/*geocode.Client: в проде кладём настоящие, в
	// тестах — фейки без сети.
	client    vkAPI
	store     *cache.Cache[GlampingData]
	extractor extract.Extractor
	geocoder  geocoderAPI
	// dataDir — каталог конфигов объектов. Поле (а не глобальная константа),
	// чтобы тест мог указать свой testdata.
	dataDir string
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("startup: %v", err)
	}

	// Composition root: единственное место, где собираем граф зависимостей.
	srv := &server{
		client:   vk.NewClient(cfg.VKToken),
		store:    cache.New[GlampingData](cacheTTL),
		geocoder: geocode.New(),
		dataDir:  dataDir,
	}

	// Выбор движка извлечения. Есть ключ — берём умный LLM; нет — бесплатную
	// эвристику. Структура одинаковая, отличается только «начинка» — в этом и
	// смысл интерфейса: остальной код (хендлер) не меняется ни на строку.
	if cfg.AnthropicKey != "" {
		srv.extractor = extract.NewLLM(cfg.AnthropicKey)
		log.Println("извлечение: LLM (ANTHROPIC_API_KEY задан)")
	} else {
		srv.extractor = extract.NewHeuristic()
		log.Println("извлечение: эвристика (бесплатно, без ключа)")
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
