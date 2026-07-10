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

	"vk-parser/internal/contract"
	"vk-parser/providers"
	"vk-parser/providers/glamping_rf"
)

// providerTimeout — общий бюджет пакетного сбора (сеть + пагинация с задержками).
const providerTimeout = 10 * time.Minute

// selectProvider — фабрика: имя из CLI → конкретная реализация Provider. Здесь
// (и только здесь) main знает про конкретные пакеты; дальше — работа через интерфейс.
func selectProvider(name string) (providers.Provider, error) {
	switch name {
	case "glamping", "glamping_rf":
		return glamping_rf.New(), nil
	default:
		return nil, fmt.Errorf("неизвестный провайдер %q (доступно: glamping)", name)
	}
}

// runProvider запускает сбор выбранным провайдером и пишет результат в
// generated/<provider>/objects.json (или в outDir, если задан -out).
func runProvider(name, outDir string) error {
	p, err := selectProvider(name)
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
		outDir = filepath.Join("generated", p.Name())
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
