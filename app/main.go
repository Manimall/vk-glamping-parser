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
	"flag"
	"log/slog"
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
	// cacheTTL — сколько держим карточку в кэше, прежде чем перепарсить VK.
	cacheTTL = 5 * time.Minute
	// maxPhotos — сколько лучших фото оставляем в галерее.
	maxPhotos = 15
	// HTTP-таймауты сервера.
	readTimeout     = 5 * time.Second
	writeTimeout    = 30 * time.Second // поход в VK может быть небыстрым
	idleTimeout     = 60 * time.Second
	shutdownTimeout = 10 * time.Second
	// exportTimeout — общий бюджет CLI-экспорта (скачать все фото + перекодировать).
	exportTimeout = 5 * time.Minute
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
	// Флаги. Если задан -export <domain> — собираем галерею фото объекта и выходим
	// (CLI-режим экспорта), иначе поднимаем HTTP-сервер.
	exportDomain := flag.String("export", "", "домен объекта: собрать photo-N.webp и выйти (вместо сервера)")
	exportOut := flag.String("out", "", "каталог для экспортированных фото (по умолчанию export/<domain>)")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}

	if *exportDomain != "" {
		ctx, cancel := context.WithTimeout(context.Background(), exportTimeout)
		defer cancel()
		if err := runExport(ctx, cfg, *exportDomain, *exportOut); err != nil {
			slog.Error("export failed", "err", err)
			os.Exit(1)
		}
		return
	}

	// Composition root: единственное место, где собираем граф зависимостей.
	srv := &server{
		client:   vk.NewClient(cfg.VKToken),
		store:    cache.New[GlampingData](cacheTTL),
		geocoder: geocode.New(),
		dataDir:  cfg.DataDir,
	}

	// Выбор движка извлечения. Есть ключ — берём умный LLM; нет — бесплатную
	// эвристику. Структура одинаковая, отличается только «начинка» — в этом и
	// смысл интерфейса: остальной код (хендлер) не меняется ни на строку.
	if cfg.AnthropicKey != "" {
		srv.extractor = extract.NewLLM(cfg.AnthropicKey)
		slog.Info("извлечение: LLM (ANTHROPIC_API_KEY задан)")
	} else {
		srv.extractor = extract.NewHeuristic()
		slog.Info("извлечение: эвристика (бесплатно, без ключа)")
	}

	// Роутер. srv.handleGlamping — это «method value»: метод, привязанный к srv,
	// который сам по себе реализует http.HandlerFunc. Фабрика-замыкание больше
	// не нужна — зависимости уже внутри srv.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/glamping", srv.handleGlamping)

	// Транспорт (http.Server) — отдельная сущность от нашего server. Свой сервер
	// с таймаутами (НЕ http.ListenAndServe без настроек — он без таймаутов).
	httpServer := &http.Server{
		Addr:         cfg.ServerAddr,
		Handler:      mux,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	// ctx отменяется при SIGINT (Ctrl+C) или SIGTERM (docker stop / systemd).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Слушаем в отдельной горутине, чтобы main мог ждать сигнал.
	go func() {
		slog.Info("VK parser слушает", "addr", cfg.ServerAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("получен сигнал остановки, завершаем…")

	// Даём время на доработку текущих запросов, потом обрываем.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	slog.Info("сервер остановлен корректно")
}
