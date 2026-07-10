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
	"vk-parser/internal/geocode"
	"vk-parser/internal/vk"
	vkprovider "vk-parser/providers/vk"
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
	// parser — VK-провайдер (собирает карточку объекта из VK). Вся VK-логика
	// изолирована в providers/vk; HTTP-обработчик лишь делегирует ей. store — кэш
	// ответов. В тестах parser собирается с фейковым VK-клиентом (без сети).
	parser *vkprovider.Parser
	store  *cache.Cache[GlampingData]
}

func main() {
	// Флаги. Если задан -export <domain> — собираем галерею фото объекта и выходим
	// (CLI-режим экспорта), иначе поднимаем HTTP-сервер.
	exportDomain := flag.String("export", "", "домен объекта: собрать photo-N.webp и выйти (вместо сервера)")
	exportOut := flag.String("out", "", "каталог вывода (-export фото / --provider JSON)")
	providerName := flag.String("provider", "", "пакетный сбор источника: glamping → generated/<name>/objects.json")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}

	// Режим провайдера: пакетный сбор источника в generated/ и выход. VK-токен
	// здесь не требуется (провайдер glamping ходит на свой сайт).
	if *providerName != "" {
		if err := runProvider(cfg, *providerName, *exportOut); err != nil {
			slog.Error("provider failed", "err", err)
			os.Exit(1)
		}
		return
	}

	// Дальше — VK-режимы (сервер / -export): им нужен токен.
	if cfg.VKToken == "" {
		slog.Error("startup failed", "err", "VK_TOKEN is not set")
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

	// Composition root: собираем VK-провайдер (движок извлечения выбирает
	// chooseExtractor: LLM при ключе, иначе бесплатная эвристика) и HTTP-сервер
	// поверх него. Вся VK-логика — в providers/vk; server лишь делегирует.
	srv := &server{
		parser: vkprovider.New(vk.NewClient(cfg.VKToken), chooseExtractor(cfg), geocode.New(), cfg.DataDir),
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
