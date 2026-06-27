# vk-glamping-parser

HTTP-микросервис на Go: парсит данные глэмпинга/А-фрейма из VK API (5.131) —
фотографии и карточку товара (название, описание, цена) — и отдаёт их JSON-ом
для фронтенда.

## Стек

- Go 1.24
- стандартный `net/http` (HTTP-сервер и клиент к VK)
- `github.com/joho/godotenv` — загрузка `.env`

## Структура

```
cmd/parser/        точка входа (HTTP-сервер)
internal/config/   загрузка конфигурации из .env
internal/vk/       клиент VK API (resolveScreenName, photos.get, market.getById)
```

## Запуск

```bash
cp .env.example .env        # и подставь свой VK_TOKEN
go mod tidy
go run ./cmd/parser         # сервис слушает :8080
```

## API

```
GET /api/glamping?domain=<screen_name>
```

Пример:

```bash
curl "http://localhost:8080/api/glamping?domain=elkidom37"
```

Ответ:

```json
{
  "title": "AFRAME светлый (аренда )",
  "description": "АРЕНДА светлого ДОМА...",
  "price": "7,000–9,500 ₽",
  "photos": ["https://sun9-...userapi.com/...jpg", "..."]
}
```

| Код | Когда |
|-----|-------|
| 200 | успех |
| 400 | не передан параметр `domain` |
| 502 | ошибка при обращении к VK API |

## Конфигурация

| Переменная | Описание |
|------------|----------|
| `VK_TOKEN` | токен доступа к VK API (обязателен) |

`.env` в репозиторий **не коммитится** (см. `.gitignore`).
