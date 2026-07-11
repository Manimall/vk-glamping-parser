package main

// CLI-режим пакетного сбора: `--provider=<name>` выбирает источник (паттерн
// «Стратегия»), запускает его Parse и пишет унифицированный JSON в generated/.
// main об устройстве провайдеров не знает — работает через интерфейс providers.Provider.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"vk-parser/internal/config"
	"vk-parser/internal/contract"
	"vk-parser/internal/extract"
	"vk-parser/internal/geocode"
	"vk-parser/internal/vk"
	"vk-parser/providers"
	"vk-parser/providers/glamping_rf"
	vkprovider "vk-parser/providers/vk"
)

// providerTimeout — общий бюджет пакетного сбора (сеть + пагинация с задержками).
const providerTimeout = 10 * time.Minute

// selectProvider — фабрика: имя из CLI → конкретная реализация Provider. Здесь
// (и только здесь) main знает про конкретные пакеты; дальше — работа через интерфейс.
func selectProvider(name string, cfg *config.Config) (providers.Provider, error) {
	switch name {
	case "glamping", "glamping_rf":
		return glamping_rf.New(), nil
	case "vk":
		// Fail-fast: без токена каждый домен молча упал бы per-domain (graceful
		// WARN) и вышел бы «успешный» пустой objects.json — маскировка мисконфига.
		if cfg.VKToken == "" {
			return nil, fmt.Errorf("провайдер vk требует VK_TOKEN")
		}
		return vkprovider.New(vk.NewClient(cfg.VKToken), chooseExtractor(cfg), geocode.New(), cfg.DataDir), nil
	default:
		return nil, fmt.Errorf("неизвестный провайдер %q (доступно: vk, glamping)", name)
	}
}

// chooseExtractor выбирает движок структурирования: LLM при заданном ключе, иначе
// бесплатная эвристика. Единый выбор для HTTP-сервера и провайдера vk (DRY).
func chooseExtractor(cfg *config.Config) extract.Extractor {
	if cfg.AnthropicKey != "" {
		slog.Info("извлечение: LLM (ANTHROPIC_API_KEY задан)")
		return extract.NewLLM(cfg.AnthropicKey)
	}
	slog.Info("извлечение: эвристика (бесплатно, без ключа)")
	return extract.NewHeuristic()
}

// runProvider запускает сбор выбранным провайдером и пишет результат в
// generated/<provider>/objects.json (или в outDir, если задан -out).
func runProvider(cfg *config.Config, name, outDir string) error {
	p, err := selectProvider(name, cfg)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), providerTimeout)
	defer cancel()

	slog.Info("провайдер: старт сбора", "provider", p.Name())
	objects, err := p.Parse(ctx)
	if err != nil {
		return fmt.Errorf("provider %s: %w", p.Name(), err)
	}

	if outDir == "" {
		outDir = filepath.Join(cfg.GeneratedDir, p.Name())
	}
	if err := writeObjects(outDir, objects); err != nil {
		return err
	}
	slog.Info("провайдер: готово", "provider", p.Name(), "объектов", len(objects), "out", outDir)
	return nil
}

// writeObjects пишет массив объектов единым JSON-файлом objects.json (формат
// contract.Object — тот же, что у VK; фронту изменения не нужны).
func writeObjects(dir string, objects []contract.Object) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("provider: mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(objects, "", "  ")
	if err != nil {
		return fmt.Errorf("provider: marshal: %w", err)
	}
	dest := filepath.Join(dir, "objects.json")
	if err := os.WriteFile(dest, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("provider: write %s: %w", dest, err)
	}
	return nil
}
