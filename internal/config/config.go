// Package config загружает конфигурацию приложения из окружения / .env.
package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config — типизированный снимок настроек приложения.
// Вся остальная программа зависит от этой структуры, а не от os.Getenv напрямую,
// поэтому "грязное" чтение окружения изолировано в одном месте.
type Config struct {
	VKToken string
}

// Load читает .env (если он есть) и собирает Config.
// Возвращает ошибку, если обязательная переменная не задана.
func Load() (*Config, error) {
	// godotenv.Load кладёт пары из .env в окружение процесса.
	// Ошибку намеренно игнорируем: в проде .env-файла нет — переменные
	// приходят прямо из окружения (Docker/CI), и это нормальный сценарий.
	_ = godotenv.Load()

	token := os.Getenv("VK_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("config: VK_TOKEN is not set")
	}

	return &Config{VKToken: token}, nil
}
