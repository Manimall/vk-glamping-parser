package main

import (
	"context"

	"vk-parser/internal/geocode"
	"vk-parser/internal/vk"
)

// Интерфейсы-зависимости хендлера. По идиоме Go «accept interfaces, return
// structs» их объявляет ПОТРЕБИТЕЛЬ (main), а не пакеты vk/geocode. Так main не
// привязан к конкретным типам: в проде сюда кладём настоящие *vk.Client /
// *geocode.Client, в тестах — фейки. Методы — ровно те, что реально зовёт хендлер.

// vkAPI — то, что хендлеру нужно от VK. *vk.Client удовлетворяет этому интерфейсу
// автоматически (Go использует структурную типизацию — никаких "implements").
type vkAPI interface {
	ResolveOwnerID(ctx context.Context, domain string) (int64, error)
	GetPhotos(ctx context.Context, ownerID int64, limit int) ([]string, error)
	GetGroupInfo(ctx context.Context, domain string) (*vk.GroupInfo, error)
	GetMarketItemsByIDs(ctx context.Context, itemIDs []string) ([]vk.MarketItem, error)
}

// geocoderAPI — то, что хендлеру нужно от геокодера. *geocode.Client удовлетворяет.
type geocoderAPI interface {
	Geocode(ctx context.Context, address string) (*geocode.Coords, error)
}
