// Command parser — HTTP-микросервис: по запросу собирает карточку объекта из
// VK и отдаёт её JSON-ом. Фронтенд ходит сюда вместо чтения статичного файла.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"vk-parser/internal/cache"
	"vk-parser/internal/config"
	"vk-parser/internal/extract"
	"vk-parser/internal/geocode"
	"vk-parser/internal/objects"
	"vk-parser/internal/vk"
)

const (
	serverAddr = ":8080"
	// cacheTTL — сколько держим карточку в кэше, прежде чем перепарсить VK.
	cacheTTL = 5 * time.Minute
	// dataDir — каталог с пер-объектными конфигами data/<domain>.json.
	dataDir = "data"
	// maxPhotos — сколько лучших фото оставляем в галерее.
	maxPhotos = 15
)

// Coords — гео-координаты объекта. Указатель в GlampingData (см. ниже), чтобы
// omitempty мог их «выкинуть»: у структуры-значения нет понятия «пустая», а
// nil-указатель omitempty уберёт.
type Coords struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Cabin — ОДИН домик глэмпинга (A-frame, BALI и т.п.). У каждого своя цена и
// своё описание → свои удобства. Property — структурированный результат: что
// именно есть в этом домике (главное, что нам нужно).
type Cabin struct {
	Title       string            `json:"title"`
	Price       string            `json:"price,omitempty"`
	Description string            `json:"description,omitempty"`
	Property    *extract.Property `json:"property,omitempty"`
	// Variants — заголовки почти-одинаковых домиков, схлопнутых в этот (напр.
	// «тёмный» вариант того же А-фрейма). omitempty: если дублей не было — поля нет.
	Variants []string `json:"variants,omitempty"`
}

// GlampingData — карточка глэмпинга: ОБЪЕКТ-уровень (название, локация, галерея)
// + список домиков. omitempty убирает поля, которых нет.
type GlampingData struct {
	Title    string   `json:"title,omitempty"`    // название глэмпинга (из группы)
	About    string   `json:"about,omitempty"`    // описание сообщества
	Location string   `json:"location,omitempty"` // адрес/город
	Coords   *Coords  `json:"coords,omitempty"`   // координаты (если заданы)
	MapURL   string   `json:"mapUrl,omitempty"`   // ссылка на карту (если задана)
	Contact  string   `json:"contact,omitempty"`  // телефон
	Photos   []string `json:"photos"`             // общая галерея
	Cabins   []Cabin  `json:"cabins"`             // домики с удобствами
}

// glampingQuery — разобранные параметры запроса. Объект-параметр вместо длинного
// списка аргументов buildGlampingData(domain, items, coords, map, ...): добавить
// новый параметр = новое поле здесь, сигнатуры функций не пухнут (тот же приём,
// что и со структурой server).
type glampingQuery struct {
	domain string
	items  string // товары-домики (URL/id через запятую)
	coords string // "lat,lon" — если VK не отдал координаты
	mapURL string // ссылка на карту (Яндекс/Google)
}

// cacheKey — детерминированный ключ кэша из всех параметров: разный ввод = разный
// ответ. Поля простые строки, порядок фиксирован, так что ключ стабилен.
func (q glampingQuery) cacheKey() string {
	return strings.Join([]string{q.domain, q.items, q.coords, q.mapURL}, "|")
}

// server держит ОБЩИЕ зависимости приложения. Хендлеры — это методы server,
// поэтому они берут зависимости из ресивера (s.client, s.store), а не из
// длинного списка аргументов. Новая зависимость = новое поле здесь, без правки
// сигнатур хендлеров.
type server struct {
	client *vk.Client
	store  *cache.Cache[GlampingData]
	// extractor — ИНТЕРФЕЙС, а не конкретный тип. server не знает (и не должен),
	// чем именно извлекают: бесплатной эвристикой или платным LLM. Подменить
	// движок = передать сюда другой объект, реализующий extract.Extractor.
	extractor extract.Extractor
	// geocoder — получает координаты из адреса, когда их не задали вручную.
	geocoder *geocode.Client
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("startup: %v", err)
	}

	// Composition root: единственное место, где собираем граф зависимостей.
	srv := &server{
		client:   vk.NewClient(cfg.VKToken),
		store:    cache.New[GlampingData](cacheTTL),
		geocoder: geocode.New(),
	}

	// Выбор движка извлечения. Есть ключ — берём умный LLM; нет — бесплатную
	// эвристику. Структура одинаковая, отличается только «начинка» — в этом и
	// смысл интерфейса: остальной код (хендлер) не меняется ни на строку.
	if cfg.AnthropicKey != "" {
		srv.extractor = extract.NewLLM(cfg.AnthropicKey)
		log.Println("извлечение: LLM (ANTHROPIC_API_KEY задан)")
	} else {
		srv.extractor = extract.NewHeuristic()
		log.Println("извлечение: эвристика (бесплатно, без ключа)")
	}

	// Роутер. srv.handleGlamping — это «method value»: метод, привязанный к srv,
	// который сам по себе реализует http.HandlerFunc. Фабрика-замыкание больше
	// не нужна — зависимости уже внутри srv.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/glamping", srv.handleGlamping)

	// Транспорт (http.Server) — отдельная сущность от нашего server. Свой сервер
	// с таймаутами (НЕ http.ListenAndServe без настроек — он без таймаутов).
	httpServer := &http.Server{
		Addr:         serverAddr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 30 * time.Second, // поход в VK может быть небыстрым
		IdleTimeout:  60 * time.Second,
	}

	// ctx отменяется при SIGINT (Ctrl+C) или SIGTERM (docker stop / systemd).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Слушаем в отдельной горутине, чтобы main мог ждать сигнал.
	go func() {
		log.Printf("VK parser слушает на %s", serverAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("получен сигнал остановки, завершаем…")

	// Даём до 10с на доработку текущих запросов, потом обрываем.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("graceful shutdown: %v", err)
	}
	log.Println("сервер остановлен корректно")
}

// handleGlamping — теперь МЕТОД server. Сигнатура — ровно http.HandlerFunc
// (w, r), без зависимостей в аргументах: они доступны через ресивер s.
func (s *server) handleGlamping(w http.ResponseWriter, r *http.Request) {
	q := glampingQuery{
		domain: r.URL.Query().Get("domain"),
		items:  r.URL.Query().Get("items"),
		coords: r.URL.Query().Get("coords"),
		mapURL: r.URL.Query().Get("map"),
	}
	if q.domain == "" {
		http.Error(w, "query param 'domain' is required", http.StatusBadRequest)
		return
	}

	// Cache hit — отдаём из памяти, в VK не ходим.
	if data, ok := s.store.Get(q.cacheKey()); ok {
		w.Header().Set("X-Cache", "HIT")
		writeJSON(w, data)
		return
	}

	// Cache miss → идём в VK. r.Context() отменяется, если клиент отвалился.
	data, err := s.buildGlampingData(r.Context(), q)
	if err != nil {
		log.Printf("build data for %q: %v", q.domain, err)
		http.Error(w, "failed to fetch data from VK", http.StatusBadGateway)
		return
	}

	s.store.Set(q.cacheKey(), data)
	w.Header().Set("X-Cache", "MISS")
	writeJSON(w, data)
}

// buildGlampingData — «бизнес-логика» одного запроса: объект-уровень (инфо
// группы + галерея) и список домиков из переданных items.
func (s *server) buildGlampingData(ctx context.Context, q glampingQuery) (GlampingData, error) {
	ownerID, err := s.client.ResolveOwnerID(ctx, q.domain)
	if err != nil {
		return GlampingData{}, fmt.Errorf("resolve: %w", err)
	}

	photos, err := s.client.GetPhotos(ctx, ownerID, maxPhotos)
	if err != nil {
		return GlampingData{}, fmt.Errorf("photos: %w", err)
	}

	data := GlampingData{Photos: photos, Cabins: []Cabin{}}

	// Объект-уровень: название/локация/контакт берём из инфо сообщества.
	// Метод только для групп; для пользователей VK вернёт ошибку — graceful.
	if info, err := s.client.GetGroupInfo(ctx, q.domain); err != nil {
		log.Printf("group info for %q: %v (пропускаю инфо объекта)", q.domain, err)
	} else {
		data.Title = info.Name
		data.About = info.Description
		data.Location = info.Address
		data.Contact = info.Phone
		if info.Latitude != 0 || info.Longitude != 0 {
			data.Coords = &Coords{Lat: info.Latitude, Lon: info.Longitude}
		}
	}

	// Пер-объектный конфиг data/<domain>.json — ручные данные, которых нет в VK
	// (координаты, карта, id товаров, «ручные» домики с Avito). Нет файла — nil.
	cfg, err := objects.Load(dataDir, q.domain)
	if err != nil {
		log.Printf("object config %q: %v (пропускаю)", q.domain, err)
	}

	// Слияние источников. Приоритет — у параметров запроса; чего нет в запросе,
	// берём из конфига. Так конфиг = «значения по умолчанию для объекта».
	coordsRaw, mapURL, itemsRaw := q.coords, q.mapURL, q.items
	var manual []objects.Cabin
	if cfg != nil {
		if coordsRaw == "" {
			coordsRaw = cfg.Coords
		}
		if mapURL == "" {
			mapURL = cfg.Map
		}
		if itemsRaw == "" && len(cfg.Items) > 0 {
			itemsRaw = strings.Join(cfg.Items, ",")
		}
		// Адрес из конфига точнее города из VK — если задан, используем его.
		if cfg.Address != "" {
			data.Location = cfg.Address
		}
		manual = cfg.Cabins
	}

	if c, ok := parseCoords(coordsRaw); ok {
		data.Coords = c
	}
	data.MapURL = mapURL

	// Фоллбэк: координат нет, но есть адрес → получаем их геокодером (бесплатно).
	// Ручные координаты приоритетнее, поэтому ходим в сеть только при их отсутствии.
	if data.Coords == nil && data.Location != "" {
		if c, err := s.geocoder.Geocode(ctx, data.Location); err != nil {
			log.Printf("geocode %q: %v (без координат)", data.Location, err)
		} else {
			data.Coords = &Coords{Lat: c.Lat, Lon: c.Lon}
		}
	}

	// «Сырые» домики из двух источников: товары VK (по прямым id) и ручные
	// домики из конфига (например, описание с Avito, который ботами не парсится).
	raw := make([]objects.Cabin, 0)
	if ids := marketIDsFromParam(itemsRaw, ownerID); len(ids) > 0 {
		if items, err := s.client.GetMarketItemsByIDs(ctx, ids); err != nil {
			log.Printf("market items %v: %v (пропускаю товары)", ids, err)
		} else {
			for _, item := range items {
				raw = append(raw, objects.Cabin{
					Title:       item.Title,
					Price:       item.Price.Text,
					Description: item.Description,
				})
			}
		}
	}
	raw = append(raw, manual...)

	data.Cabins = s.structureCabins(ctx, raw, data.Location, len(photos))
	return data, nil
}

// parseCoords разбирает строку "lat,lon" в Coords. Возвращает (nil,false), если
// формат неверный — тогда вызывающий просто не трогает координаты.
func parseCoords(raw string) (*Coords, bool) {
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return nil, false
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return nil, false
	}
	return &Coords{Lat: lat, Lon: lon}, true
}

// structureCabins прогоняет каждый «сырой» домик через извлекатель (что в нём
// есть) и схлопывает почти одинаковые варианты (дедуп).
func (s *server) structureCabins(ctx context.Context, raw []objects.Cabin, location string, photoCount int) []Cabin {
	cabins := make([]Cabin, 0, len(raw))
	for _, rc := range raw {
		cabin := Cabin{Title: rc.Title, Price: rc.Price, Description: rc.Description}
		// Структурируем описание домика → что в нём есть (главное для нас).
		listing := extract.Listing{
			Title:       rc.Title,
			Description: rc.Description,
			Location:    location,
			Price:       rc.Price,
			PhotoCount:  photoCount,
		}
		if prop, err := s.extractor.Extract(ctx, listing); err != nil {
			log.Printf("extract cabin %q: %v", rc.Title, err)
		} else {
			cabin.Property = prop
		}
		cabins = append(cabins, cabin)
	}
	return dedupCabins(cabins)
}

// reItemTail — хвост числа из URL/строки товара. У ссылки вида
// .../aframe-svetly-arenda-211011668-6377368 последнее число — это id товара.
var reItemTail = regexp.MustCompile(`(\d+)\D*$`)

// marketIDsFromParam превращает параметр items (URL или id через запятую) в
// абсолютные market-id вида "<ownerID>_<item>". Принимаем три формата:
//   - полный id "-211011668_6377368" → как есть;
//   - ссылку ".../...-211011668-6377368" → берём хвостовое число + наш ownerID;
//   - голый id товара "6377368" → тоже + ownerID.
// ownerID уже со знаком (минус для групп), поэтому склейка даёт верный market-id.
func marketIDsFromParam(raw string, ownerID int64) []string {
	ids := make([]string, 0)
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if strings.Contains(tok, "_") {
			ids = append(ids, tok) // уже полный market-id
			continue
		}
		if m := reItemTail.FindStringSubmatch(tok); m != nil {
			ids = append(ids, fmt.Sprintf("%d_%s", ownerID, m[1]))
		}
	}
	return ids
}

// dedupCabins схлопывает почти одинаковые домики. Сигнатура домика — набор его
// удобств (из Property). Если у двух домиков удобства совпадают на ≥80%, второй
// считаем вариантом первого: его заголовок уходит в Variants, а отдельной
// карточкой он не дублируется.
const dupThreshold = 0.8

func dedupCabins(cabins []Cabin) []Cabin {
	kept := make([]Cabin, 0, len(cabins))
	sigs := make([]map[string]bool, 0, len(cabins))

	for _, c := range cabins {
		sig := amenitySignature(c)
		merged := false
		for i := range kept {
			if jaccard(sig, sigs[i]) >= dupThreshold {
				// Дубль: добавляем его название как вариант к уже сохранённому.
				kept[i].Variants = append(kept[i].Variants, c.Title)
				merged = true
				break
			}
		}
		if !merged {
			kept = append(kept, c)
			sigs = append(sigs, sig)
		}
	}
	return kept
}

// amenitySignature — множество «меток» домика: удобства + доп.услуги. Это и есть
// его «отпечаток» для сравнения. Пустой Property → пустая сигнатура (не схлопнем).
func amenitySignature(c Cabin) map[string]bool {
	sig := make(map[string]bool)
	if c.Property == nil {
		return sig
	}
	for _, g := range c.Property.AmenityGroups {
		for _, item := range g.Items {
			sig[item] = true
		}
	}
	for _, e := range c.Property.Extras {
		sig[e.Name] = true
	}
	return sig
}

// jaccard — мера схожести двух множеств: |пересечение| / |объединение|.
// 1.0 = одинаковы, 0.0 = ничего общего. Классический способ сравнить наборы.
func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0 // обе пустые — НЕ считаем дублями (нечего сравнивать)
	}
	inter := 0
	for k := range a {
		if b[k] {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// writeJSON — без зависимостей, поэтому остаётся обычной функцией (не методом).
// DRY: одна запись JSON-ответа для веток HIT и MISS.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("encode response: %v", err)
	}
}
