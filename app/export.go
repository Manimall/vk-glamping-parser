package main

// Этот файл — ОРКЕСТРАТОР режима экспорта фото (CLI-ветка `-export` из main.go).
// Сам он ничего «умного» не считает: он лишь по шагам склеивает уже готовые
// кирпичи — VK-клиент (internal/vk), конфиг объекта (internal/objects) и
// пайплайн обработки картинок (internal/images) — в один сценарий:
//
//	домен → owner → URL фото (альбом дома / стена) → скачать → обработать в webp.
//
// Функции здесь СТРОЧНЫЕ (runExport, photoURLs, …) — значит приватны пакету main
// и наружу не «экспортируются»: это внутренняя кухня CLI, а не публичное API.
//
// Приём разбиения: «толстые» функции с сетью/диском (photoURLs, downloadAll)
// делаем тонкими, а логику выбора выносим в чистые функции (resolveSource) —
// их удобно тестировать без сети (см. export_test.go).

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vk-parser/internal/config"
	"vk-parser/internal/images"
	"vk-parser/internal/objects"
	"vk-parser/internal/vision"
	"vk-parser/internal/vk"
)

const (
	// albumFetchCount — сколько кадров тянем из альбома (с запасом на дедуп).
	albumFetchCount = 60
	// wallFetchCount — сколько тянем со стены (фоллбэк), тоже с запасом.
	wallFetchCount = 40
	// downloadTimeout — таймаут на скачивание одного фото.
	downloadTimeout = 20 * time.Second
)

// exportVK — то, что нужно экспорту от VK-клиента (accept interfaces). *vk.Client
// удовлетворяет автоматически; в тестах подставляем фейк без сети.
type exportVK interface {
	GetMarketItemsByIDs(ctx context.Context, itemIDs []string) ([]vk.MarketItem, error)
	GetAlbumPhotos(ctx context.Context, ownerID int64, albumID string, count int) ([]string, error)
	GetPhotos(ctx context.Context, ownerID int64, limit int) ([]string, error)
}

// runExport — единственная «точка входа» этого файла (её зовёт main.go). Читается
// сверху вниз как линейный сценарий из 4 шагов; каждый шаг делегирован отдельной
// функции, поэтому сам runExport остаётся коротким и обозримым.
//
//	Шаг 0. Куда писать: по умолчанию export/<domain>, если -out не задан.
//	Шаг 1. Кто это: screen name (домен) → числовой owner_id (у групп он < 0).
//	Шаг 2. Что брать: URL фото — из альбома дома или, если его нет, со стены.
//	Шаг 3. Скачать байты картинок (ошибка одной не роняет весь экспорт).
//	Шаг 4. Обработать: дедуп → без людей вперёд → ресайз/webp → файлы на диск.
func runExport(ctx context.Context, cfg *config.Config, domain, outDir string) error {
	// Шаг 0.
	if outDir == "" {
		outDir = filepath.Join("export", domain)
	}
	client := vk.NewClient(cfg.VKToken)

	// Шаг 1. `%w` в Errorf ОБОРАЧИВАЕТ исходную ошибку (не просто вставляет текст):
	// вызывающий код сможет добраться до неё через errors.Is/As. Префикс "export:"
	// добавляет контекст «на каком этапе упало».
	ownerID, err := client.ResolveOwnerID(ctx, domain)
	if err != nil {
		return fmt.Errorf("export: resolve %q: %w", domain, err)
	}

	// Шаг 2. Возвращаем ещё и метку источника ("album"/"wall") — только для лога.
	urls, source := photoURLs(ctx, client, cfg, domain, ownerID)
	if len(urls) == 0 {
		return fmt.Errorf("export: no photos for %q", domain)
	}
	slog.Info("export: источник фото", "domain", domain, "source", source, "urls", len(urls))

	// Шаг 3.
	raws := downloadAll(ctx, urls)
	slog.Info("export: скачано", "ok", len(raws), "из", len(urls))

	// Опциональный выбор обложки локальной vision-моделью (Ollama). Бесплатно,
	// без подписок; если сервис не поднят — picker=nil и images берёт эвристику.
	// Выключить принудительно: COVER_VISION=off.
	var picker images.CoverPicker
	if os.Getenv("COVER_VISION") != "off" {
		if vc := vision.New(os.Getenv("OLLAMA_URL"), ""); vc.Available(ctx) {
			picker = vc
			slog.Info("export: обложку выбирает vision-модель (Ollama)")
		}
	}

	// Шаг 4. Вся тяжёлая логика с картинками спрятана за одним вызовом пакета
	// images — runExport про неё ничего не знает (разделение ответственности).
	n, err := images.Process(ctx, raws, outDir, maxPhotos, picker)
	if err != nil {
		return fmt.Errorf("export: process: %w", err)
	}
	slog.Info("export: готово", "domain", domain, "photos", n, "out", outDir)
	return nil
}

// photoURLs — тонкая I/O-ОБЁРТКА. Её единственная задача: сходить на диск за
// конфигом объекта (configuredItemIDs читает файл), а само РЕШЕНИЕ «откуда брать
// фото» передать чистой resolveSource. Такое разделение (грязный ввод-вывод
// отдельно, логика отдельно) — чтобы логику можно было тестировать, не подсовывая
// ей файловую систему. Поэтому photoURLs почти пустая: получил id → передал дальше.
func photoURLs(ctx context.Context, client exportVK, cfg *config.Config, domain string, ownerID int64) ([]string, string) {
	return resolveSource(ctx, client, configuredItemIDs(cfg, domain, ownerID), ownerID)
}

// configuredItemIDs возвращает абсолютные id товаров объекта из его конфига
// (там же, откуда их берёт хендлер). Нет конфига/товаров — пустой список.
func configuredItemIDs(cfg *config.Config, domain string, ownerID int64) []string {
	obj, err := objects.Load(cfg.DataDir, domain)
	if err != nil {
		slog.Warn("export: object config", "domain", domain, "err", err)
		return nil
	}
	if obj == nil || len(obj.Items) == 0 {
		return nil
	}
	return marketIDsFromParam(strings.Join(obj.Items, ","), ownerID)
}

// resolveSource — «мозг» выбора источника. Стратегия проста и приоритетна:
// сперва пытаемся альбом дома (обычно там курированные обзорные фото без людей),
// и ТОЛЬКО если оттуда ничего не пришло — падаем на стену сообщества (там мусора
// больше: репосты, афиши с текстом). Возвращаем и метку источника — для лога.
//
// Ключевой момент: тип client здесь — интерфейс exportVK, а не конкретный
// *vk.Client. В проде сюда приходит настоящий клиент, в тесте — фейк без сети
// (export_test.go). Функция об этом не знает и знать не должна — в этом и смысл
// «шва»: логику выбора можно гонять в тестах, ни разу не сходив в интернет.
func resolveSource(ctx context.Context, client exportVK, itemIDs []string, ownerID int64) ([]string, string) {
	if urls := albumURLs(ctx, client, itemIDs); len(urls) > 0 {
		return urls, "album"
	}

	// Фоллбэк: стена (когда альбома дома нет — напр. страница-пользователь).
	urls, err := client.GetPhotos(ctx, ownerID, wallFetchCount)
	if err != nil {
		slog.Warn("export: wall photos", "err", err)
		return nil, "wall"
	}
	return urls, "wall"
}

// albumURLs идёт к фото альбома дома в ДВА прохода:
//  1. по id товаров тянем сами товары и из текста их описаний вытаскиваем ссылки
//     на альбомы («ВСЕ ФОТО ДОМА: vk.com/album-<owner>_<album>» → []AlbumRef);
//  2. по каждому AlbumRef запрашиваем фото и складываем в один общий список.
//
// Пусто (nil), если товаров нет, товары не загрузились или альбомы недоступны —
// тогда resolveSource уйдёт в фоллбэк на стену. Ошибка одного альбома не роняет
// остальные (continue) — тот же принцип «graceful», что и при скачивании.
func albumURLs(ctx context.Context, client exportVK, itemIDs []string) []string {
	if len(itemIDs) == 0 {
		return nil
	}
	items, err := client.GetMarketItemsByIDs(ctx, itemIDs)
	if err != nil {
		slog.Warn("export: market items", "err", err)
		return nil
	}

	// Проход 1: описания → ссылки на альбомы. `xs...` — это «spread»: функция
	// вернула СРЕЗ ссылок, а тремя точками мы разворачиваем его в отдельные
	// аргументы append, дописывая все элементы разом в общий refs.
	var refs []images.AlbumRef
	for _, it := range items {
		refs = append(refs, images.AlbumRefsFromDescription(it.Description)...)
	}

	// Проход 2: каждая ссылка → фото альбома, тоже склеиваем через spread.
	var urls []string
	for _, ref := range refs {
		got, err := client.GetAlbumPhotos(ctx, ref.OwnerID, ref.AlbumID, albumFetchCount)
		if err != nil {
			slog.Warn("export: album photos", "album", ref.AlbumID, "err", err)
			continue
		}
		urls = append(urls, got...)
	}
	return urls
}

// downloadAll скачивает картинки по URL и возвращает их сырые байты.
//
// Философия — «graceful»: любой сбой на конкретном URL (битый адрес, сетевая
// ошибка, не-200 ответ, обрыв чтения) НЕ прерывает экспорт — мы логируем и через
// `continue` идём к следующему. Лучше отдать 14 фото из 15, чем упасть целиком.
// Поэтому и длина результата может быть меньше длины urls.
func downloadAll(ctx context.Context, urls []string) [][]byte {
	httpClient := &http.Client{Timeout: downloadTimeout}
	// Предвыделяем ёмкость под len(urls): append не будет перевыделять срез.
	out := make([][]byte, 0, len(urls))
	for _, u := range urls {
		// NewRequestWithContext привязывает запрос к ctx: когда общий бюджет
		// экспорта (exportTimeout) истечёт, зависшая загрузка оборвётся сама.
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			continue
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			slog.Warn("export: download", "err", err)
			continue
		}
		// VK на ошибку может отдать HTML-страницу с кодом 404/403 — её нельзя
		// декодировать как картинку, поэтому такие ответы пропускаем сразу.
		if resp.StatusCode != http.StatusOK {
			slog.Warn("export: download status", "url", u, "status", resp.StatusCode)
			resp.Body.Close() // тело ОБЯЗАТЕЛЬНО закрыть даже на ошибке — иначе утечёт соединение
			continue
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close() // здесь без defer: мы в цикле, defer копил бы закрытия до конца функции
		if err != nil {
			continue
		}
		out = append(out, data)
	}
	return out
}
