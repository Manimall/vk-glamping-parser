// Command parser — HTTP-микросервис: по запросу собирает карточку объекта из
// VK и отдаёт её JSON-ом. Фронтенд ходит сюда вместо чтения статичного файла.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"vk-parser/internal/config"
	"vk-parser/internal/vk"
)

const (
	serverAddr = ":8080"
	// productID — пока конкретный товар ЁлкиДом. В «боевом» сервисе id товара
	// резолвился бы по домену (отдельная задача), здесь фиксируем для демо.
	productID = "-211011668_6377368"
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

	// Роутер. Go 1.22+ умеет метод+путь прямо в паттерне ("GET /api/...").
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/glamping", handleGlamping(client))

	// СВОЙ http.Server с таймаутами. НЕ используем http.ListenAndServe(...)
	// без настроек: у него нет таймаутов → уязвимость к «медленным» клиентам.
	srv := &http.Server{
		Addr:         serverAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second, // поход в VK может быть небыстрым
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("VK parser слушает на %s", serverAddr)
	// ListenAndServe блокирует и возвращает ошибку только при падении.
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// handleGlamping — фабрика хендлера: принимает зависимость (*vk.Client) и
// возвращает http.HandlerFunc-замыкание. Это идиоматичный для Go способ
// «прокинуть» зависимость в обработчик (вместо глобальной переменной).
func handleGlamping(client *vk.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		domain := r.URL.Query().Get("domain")
		if domain == "" {
			// Ошибка клиента → 400 Bad Request.
			http.Error(w, "query param 'domain' is required", http.StatusBadRequest)
			return
		}

		// r.Context() отменяется, когда клиент закрыл соединение → отмена
		// долетит до VK-запросов внутри buildGlampingData.
		data, err := buildGlampingData(r.Context(), client, domain)
		if err != nil {
			// Сбой на нашей стороне / на стороне VK → 502 Bad Gateway.
			// Наружу не светим детали ошибки, в лог — пишем полностью.
			log.Printf("build data for %q: %v", domain, err)
			http.Error(w, "failed to fetch data from VK", http.StatusBadGateway)
			return
		}

		// Content-Type ставим ДО записи тела (после первого Write заголовки
		// уже отправлены и менять их поздно).
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		// Кодируем JSON ПРЯМО в ResponseWriter (он реализует io.Writer) —
		// потоково, без промежуточного []byte. Сравни с файловой версией, где
		// мы сначала собирали out []byte для os.WriteFile.
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(data); err != nil {
			log.Printf("encode response: %v", err)
		}
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
