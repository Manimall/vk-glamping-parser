// Библиотека веб-обогащения: Tavily (поиск) → выжимка → Ollama (извлечение) →
// merge без перезаписи данных источника. CLI-обвязка — в ../enrich-web.mjs.

// ── Конфиг внешних сервисов ──────────────────────────────────────────────────
const TAVILY_URL = 'https://api.tavily.com/search'
const OLLAMA_URL = 'http://127.0.0.1:11434/api/generate'
const OLLAMA_MODEL = 'llama3.1'
const OLLAMA_NUM_CTX = 8192 // дефолтный num_ctx Ollama мал — задаём явно
const MAX_CONTEXT_CHARS = 12000 // бюджет текста (≈4-5k токенов при num_ctx 8192)
const KEYWORD_WINDOW = 300 // размер окна вокруг ключевого слова при выжимке
const MIN_PRICE_RUB = 100 // валидация извлечённых цен: дешевле — мусор
const MAX_PRICE_RUB = 1_000_000 // дороже — галлюцинация

const RETRIES = 2 // повторы на сетевые сбои/лимиты (dev-ключ Tavily душит частоту)
const RETRY_PAUSE_MS = 4000

// Каталоги-агрегаторы: страницы-списки «все глэмпинги области» с чужими ценами.
const AGGREGATOR_DOMAINS = [
  'glampi.ru',
  'glampinginfo.ru',
  'mirturbaz.ru',
  'sutochno.com',
  'avito.ru',
  '101hotels.com',
  'tutu.ru',
  'ostrovok.ru',
  'yandex.ru',
  'tripadvisor.ru',
]

/** @param {string} url @returns {boolean} страница каталога-агрегатора? */
function isAggregator(url) {
  try {
    const host = new URL(url).hostname
    return AGGREGATOR_DOMAINS.some((d) => host === d || host.endsWith(`.${d}`))
  } catch {
    return false
  }
}

const REQUEST_TIMEOUT_MS = 60000 // advanced-поиск Tavily отвечает до 10-30с

/** fetch с повторами: сетевые сбои и 429/5xx ретраятся с паузой.
 *  Connection:close — на сериях запросов undici переиспользует сокет, а сервер
 *  его дропает («fetch failed» залпами при живом одиночном запросе). */
async function fetchRetry(url, init) {
  let lastErr
  for (let attempt = 0; attempt <= RETRIES; attempt++) {
    try {
      const res = await fetch(url, {
        ...init,
        headers: { ...init.headers, Connection: 'close' },
        signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
      })
      if (res.ok || (res.status < 500 && res.status !== 429)) return res
      lastErr = new Error(`http ${res.status}`)
    } catch (err) {
      // cause в текст: голое «fetch failed» не диагностируемо.
      lastErr = new Error(`${err.message}${err.cause ? ` (${err.cause.code ?? err.cause.message})` : ''}`)
    }
    await new Promise((r) => setTimeout(r, RETRY_PAUSE_MS * (attempt + 1)))
  }
  throw lastErr
}

// ── Tavily: поиск официальных сведений ───────────────────────────────────────
/** @returns {Promise<{context: string, sources: string[]}|null>} */
export async function searchTavily(obj) {
  const query = `глэмпинг ${obj.title} ${obj.location} официальный сайт цены допы правила`
  const res = await fetchRetry(TAVILY_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      api_key: process.env.TAVILY_API_KEY,
      query,
      search_depth: 'advanced',
      include_raw_content: true,
      max_results: 3,
    }),
  })
  if (!res.ok) throw new Error(`tavily ${res.status}`)
  const data = await res.json()
  // Атрибуция: страницы КАТАЛОГОВ-агрегаторов («все глэмпинги области») полны
  // ЧУЖИХ цен — для них требуем упоминания названия объекта. Прочие сайты
  // (в т.ч. официальные) проходят всегда: у глэмпинги.рф названия-псевдонимы,
  // реальный сайт объекта может зваться иначе («Точка Немо»).
  const title = norm(obj.title)
  const results = (data.results ?? []).filter((r) => {
    if (!isAggregator(r.url)) return true
    return norm(`${r.title ?? ''} ${r.raw_content ?? r.content ?? ''}`).includes(title)
  })
  if (results.length === 0) return null
  const context = results
    .map((r) => `Источник: ${r.url}\n${distill(r.raw_content || r.content || '')}`)
    .join('\n\n---\n\n')
  return { context, sources: results.map((r) => r.url) }
}

// Слова-сигналы: услуги, цены, правила — то, ради чего ходим в веб.
const SIGNAL_RE = /бан[иья]|чан|сауна|купел|питомц|животн|прокат|аренд|рыбалк|трансфер|цен[аы]|стоимост|доплат|руб|₽|прожив|заезд|правил/gi

/**
 * Выжимка длинной страницы: окна ±KEYWORD_WINDOW вокруг сигнальных слов,
 * склеенные с перекрытием. Тупая обрезка головы страницы теряла цены: страницы
 * по 70к символов, а бюджет контекста — MAX_CONTEXT_CHARS.
 * @param {string} text @returns {string}
 */
export function distill(text) {
  if (text.length <= MAX_CONTEXT_CHARS / 3) return text
  /** @type {Array<[number, number]>} */
  const spans = []
  for (const m of text.matchAll(SIGNAL_RE)) {
    const start = Math.max(0, m.index - KEYWORD_WINDOW)
    const end = Math.min(text.length, m.index + KEYWORD_WINDOW)
    const last = spans[spans.length - 1]
    if (last && start <= last[1]) last[1] = end // перекрытие — расширяем окно
    else spans.push([start, end])
  }
  if (spans.length === 0) return text.slice(0, MAX_CONTEXT_CHARS / 3)
  return spans.map(([s, e]) => text.slice(s, e)).join(' …\n')
}

// ── Ollama: извлечение структуры из текста ───────────────────────────────────
// per: «час» если цена почасовая («1200 руб/час») — иначе смержим её как цену
// «за всё» и обманем гостя (у бань часто «минимум 3 часа»).
const EXTRACT_SCHEMA = `{"has_sauna": boolean|null, "pets_allowed": boolean|null, "extras": [{"name": "строка", "price": число|null, "per": "час"|"разово"|null}]}`

/** @returns {Promise<{has_sauna: boolean|null, pets_allowed: boolean|null, extras: Array<{name: string, price: number|null, per: string|null}>}>} */
export async function extractWithOllama(obj, webContext) {
  const context = `${obj.about ?? ''}\n\n${webContext}`.slice(0, MAX_CONTEXT_CHARS)
  const prompt = `Ты извлекаешь факты о базе отдыха «${obj.title}» (${obj.location}).
Ниже — описание объекта и тексты веб-страниц. В текстах могут быть ДРУГИЕ базы отдыха:
извлекай ТОЛЬКО сведения, которые явно относятся к «${obj.title}» в «${obj.location}».
Если не уверен — ставь null. Цены только в рублях, числом, без диапазонов (бери минимум).
Если цена указана ЗА ЧАС («1200 руб. в час») — ставь per: "час"; если разово — per: "разово".
Ответ строго в JSON формата: ${EXTRACT_SCHEMA}

ТЕКСТЫ:
${context}`

  const res = await fetch(OLLAMA_URL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      model: OLLAMA_MODEL,
      prompt,
      format: 'json',
      stream: false,
      options: { num_ctx: OLLAMA_NUM_CTX },
    }),
  })
  if (!res.ok) throw new Error(`ollama ${res.status}`)
  const data = await res.json()
  try {
    return JSON.parse(data.response)
  } catch {
    throw new Error('ollama вернула невалидный JSON')
  }
}

// ── Merge: подмешать извлечённое, НЕ трогая данные источника ────────────────
function validPrice(p) {
  return typeof p === 'number' && p >= MIN_PRICE_RUB && p <= MAX_PRICE_RUB
}

const norm = (s) => (s ?? '').toLowerCase().replace(/[^a-zа-яё0-9]+/g, ' ').trim()

// Тематические группы услуг: матчим дыру объекта с извлечённой услугой, если
// обе попадают в одну группу — строкового равенства мало из-за морфологии и
// синонимов («Горячий чан» на сайте объекта называется «Горячие купели»).
const SERVICE_GROUPS = [
  /чан|купел|джакузи|бочк[аи]/i,
  /бан[еяю]|парн/i,
  /саун/i,
  /питомц|животн|собак|кошк/i,
  /рыбалк|рыболов/i,
  /мангал|барбекю|гриль/i,
  /трансфер/i,
  /завтрак|питани|еда/i,
]

/** Услуги об одном и том же? Точное/подстрочное совпадение или одна тем-группа. */
function sameService(a, b) {
  const na = norm(a)
  const nb = norm(b)
  if (na === nb || na.includes(nb) || nb.includes(na)) return true
  return SERVICE_GROUPS.some((g) => g.test(a) && g.test(b))
}

/** Цена в формате каталога; per из ответа модели нормализуется в суффикс. */
function formatWebPrice(price, per) {
  const rub = `${new Intl.NumberFormat('ru-RU').format(price)} ₽`
  if (per === 'час') return `${rub}/час` // фронт не даст заказать фикс-суммой
  if (per === 'день' || per === 'сутки') return `${rub}/сутки`
  return rub
}

/** @returns {number} сколько цен реально подмешано в услуги объекта */
export function mergeExtraction(obj, extraction, sources) {
  let filled = 0
  const extracted = (extraction.extras ?? []).filter((e) => e?.name && validPrice(e.price))
  const matchedNames = new Set()
  for (const target of obj.extras ?? []) {
    if (target.price) continue // цена источника — не перезаписываем
    const match = extracted.find((e) => sameService(e.name, target.name))
    if (match) {
      matchedNames.add(match.name)
      target.price = formatWebPrice(match.price, match.per)
      target.priceSource = 'web' // provenance: цена из веб-обогащения
      filled += 1
    }
  }
  // Услуги, найденные в вебе, но отсутствующие у объекта, — отдельным полем
  // (в основной прайс не подмешиваем без ручной проверки: риск чужих цен).
  const found = extracted
    .filter((e) => !matchedNames.has(e.name))
    .map((e) => ({ name: e.name, price: formatWebPrice(e.price, e.per) }))
  obj.webEnrichment = {
    hasSauna: extraction.has_sauna ?? null,
    petsAllowed: extraction.pets_allowed ?? null,
    foundServices: found.length ? found : undefined,
    sources,
    enrichedAt: new Date().toISOString(),
  }
  return filled
}
