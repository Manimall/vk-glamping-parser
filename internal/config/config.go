// Package config загружает конфигурацию приложения из окружения / .env.
package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config — типизированный снимок настроек приложения.
// Вся остальная программа зависит от этой структуры, а не от os.Getenv напрямую,
// поэтому "грязное" чтение окружения изолировано в одном месте.
type Config struct {
	VKToken string
	// AnthropicKey — ключ для LLM-извлечения структуры (Шаг 2). НЕОБЯЗАТЕЛЬНЫЙ:
	// если пуст, сервис отдаёт только «сырьё» из VK, без структурирования.
	AnthropicKey string
	// ServerAddr — адрес HTTP-сервера, DataDir — каталог конфигов объектов.
	// Берём из окружения с дефолтами, чтобы не хардкодить в логике.
	ServerAddr string
	DataDir    string
}

// envOr возвращает значение переменной окружения или дефолт, если она пуста.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load читает .env (если он есть) и собирает Config.
// Возвращает ошибку, если обязательная переменная не задана.
func Load() (*Config, error) {
	// godotenv.Load кладёт пары из .env в окружение процесса.
	// Ошибку намеренно игнорируем: в проде .env-файла нет — переменные
	// приходят прямо из окружения (Docker/CI), и это нормальный сценарий.
	_ = godotenv.Load()

	// VK_TOKEN и ANTHROPIC_API_KEY читаем БЕЗ проверки на пустоту: обязательность
	// зависит от режима. VK_TOKEN нужен только VK-путям (HTTP-сервер, -export,
	// провайдер vk); провайдеру glamping он не нужен. Проверку делает вызывающий
	// (main) там, где токен реально используется.
	return &Config{
		VKToken:      os.Getenv("VK_TOKEN"),
		AnthropicKey: os.Getenv("ANTHROPIC_API_KEY"),
		ServerAddr:   envOr("SERVER_ADDR", ":8080"),
		DataDir:      envOr("DATA_DIR", "data"),
	}, nil
}
