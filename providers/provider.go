// Package providers описывает единый интерфейс источника данных объектов
// (паттерн «Стратегия»). main выбирает конкретного провайдера по флагу
// --provider и вызывает Parse полиморфно, ничего не зная о его внутренностях.
//
// Каждая реализация (providers/vk, providers/glamping_rf) сама решает, откуда и
// как брать данные, но ОБЯЗАНА вернуть их в едином контракте contract.Object —
// поэтому фронту не нужны изменения при добавлении источника.
package providers

import (
	"context"
	"time"

	"vk-parser/internal/contract"
)

// Provider — источник объектов размещения.
type Provider interface {
	// Name — короткое имя источника (для логов и каталога вывода generated/<name>).
	Name() string
	// Parse собирает объекты источника в единый контракт. Возвращает ошибку только
	// на фатальном сбое (сеть/конфиг недоступны); сбой отдельного объекта или
	// страницы не должен ронять весь сбор — это логируется (WARN) и пропускается.
	Parse(ctx context.Context) ([]contract.Object, error)
}

// SleepCtx — «вежливая» пауза между запросами источника (анти-бан), прерываемая
// отменой ctx. false — ctx отменён, сбор надо сворачивать. Общий хелпер для всех
// провайдеров (DRY: не дублировать в каждом).
func SleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
