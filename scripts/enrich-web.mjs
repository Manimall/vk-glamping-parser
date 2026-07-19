#!/usr/bin/env node
// Обогащение объектов каталога данными из интернета: Tavily (поиск) + локальная
// Ollama (извлечение структуры). Итерирует по объектам с ДЫРАМИ (услуги без
// цены / нет правил), ищет официальные источники, извлекает структуру LLM-ом и
// подмешивает БЕЗ перезаписи данных источника. Результат — enriched_objects.json.
//
// Зависимости: НЕТ npm-пакетов (голый fetch, Node 18+). Опционально вместо
// fetch можно поставить официальный клиент: `npm i @tavily/core`.
// Требования окружения:
//   - TAVILY_API_KEY  — ключ https://app.tavily.com (бесплатный тир 1000 req/мес);
//   - Ollama локально: `ollama pull llama3.1 && ollama serve` (порт 11434).
//
// Запуск: TAVILY_API_KEY=tvly-... node scripts/enrich-web.mjs [--limit=10] [--dry-run]
//
// Честность данных (важно):
//   - обогащение только ЗАПОЛНЯЕТ пустое (цены «», отсутствующие правила),
//     распарсенное с глэмпинги.рф не перезаписывается никогда;
//   - LLM обязан вернуть null, если не уверен, что данные ИМЕННО этого объекта;
//   - рядом с результатом сохраняются URL-источники (webSources) — проверяемо;
//   - цены вне здравого диапазона отбрасываются.

import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

// ── Конфиг (zero hardcode в логике) ──────────────────────────────────────────
const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const INPUT_FILE = resolve(ROOT, 'generated/glamping_rf/objects.json')
const OUTPUT_FILE = resolve(ROOT, 'generated/glamping_rf/enriched_objects.json')
const TAVILY_URL = 'https://api.tavily.com/search'
const OLLAMA_URL = 'http://127.0.0.1:11434/api/generate'
const OLLAMA_MODEL = 'llama3.1'
const MAX_CONTEXT_CHARS = 6000 // бюджет контекста для llama3.1 (8k окно)
const MIN_PRICE_RUB = 100 // валидация извлечённых цен: дешевле — мусор
const MAX_PRICE_RUB = 1_000_000 // дороже — галлюцинация
const PAUSE_MS = 1200 // вежливая пауза между объектами (лимиты Tavily)
const CHECKPOINT_EVERY = 5 // как часто сбрасывать прогресс на диск

const log = {
  info: (...a) => console.log('[enrich]', ...a),
  warn: (...a) => console.warn('[enrich:warn]', ...a),
}

// ── Аргументы CLI ────────────────────────────────────────────────────────────
const args = new Map(
  process.argv.slice(2).map((a) => {
    const [k, v] = a.replace(/^--/, '').split('=')
    return [k, v ?? true]
  }),
)
const LIMIT = Number(args.get('limit')) || Infinity
const DRY_RUN = args.has('dry-run')

// ── Дыры в данных: чего не хватает объекту ───────────────────────────────────
/** @returns {boolean} есть ли услуги без цены или нет правил */
function hasGaps(obj) {
  const extras = obj.extras ?? []
  const pricelessExtra = extras.some((e) => !e.price)
  const rules = obj.cabins?.[0]?.property?.rules ?? []
  return pricelessExtra || rules.length === 0
}

// ── Tavily: поиск официальных сведений ───────────────────────────────────────
/** @returns {Promise<{context: string, sources: string[]}|null>} */
async function searchTavily(obj) {
  const query = `глэмпинг ${obj.title} ${obj.location} официальный сайт цены допы правила`
  const res = await fetch(TAVILY_URL, {
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
  const results = data.results ?? []
  if (results.length === 0) return null
  const context = results
    .map((r) => `Источник: ${r.url}\n${r.raw_content || r.content || ''}`)
    .join('\n\n---\n\n')
  return { context, sources: results.map((r) => r.url) }
}

// ── Ollama: извлечение структуры из текста ───────────────────────────────────
// per: «час» если цена почасовая («1200 руб/час») — иначе смержим её как цену
// «за всё» и обманем гостя (у бань часто «минимум 3 часа»).
const EXTRACT_SCHEMA = `{"has_sauna": boolean|null, "pets_allowed": boolean|null, "extras": [{"name": "строка", "price": число|null, "per": "час"|"разово"|null}]}`

/** @returns {Promise<{has_sauna: boolean|null, pets_allowed: boolean|null, extras: Array<{name: string, price: number|null}>}|null>} */
async function extractWithOllama(obj, webContext) {
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
    body: JSON.stringify({ model: OLLAMA_MODEL, prompt, format: 'json', stream: false }),
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

/** @returns {number} сколько цен реально подмешано */
function mergeExtraction(obj, extraction, sources) {
  let filled = 0
  const extracted = (extraction.extras ?? []).filter((e) => e?.name && validPrice(e.price))
  for (const target of obj.extras ?? []) {
    if (target.price) continue // цена источника — не перезаписываем
    const match = extracted.find(
      (e) => norm(e.name) === norm(target.name) || norm(e.name).includes(norm(target.name)),
    )
    if (match) {
      const rub = new Intl.NumberFormat('ru-RU').format(match.price)
      // Почасовая цена сохраняет «/час» — фронт не даст заказать её фикс-суммой.
      target.price = match.per === 'час' ? `${rub} ₽/час` : `${rub} ₽`
      target.priceSource = 'web' // provenance: цена из веб-обогащения
      filled += 1
    }
  }
  obj.webEnrichment = {
    hasSauna: extraction.has_sauna ?? null,
    petsAllowed: extraction.pets_allowed ?? null,
    sources,
    enrichedAt: new Date().toISOString(),
  }
  return filled
}

// ── Основной цикл ────────────────────────────────────────────────────────────
async function main() {
  if (!process.env.TAVILY_API_KEY && !DRY_RUN) {
    log.warn('нет TAVILY_API_KEY — задай: TAVILY_API_KEY=tvly-... node scripts/enrich-web.mjs')
    process.exit(1)
  }
  const objects = JSON.parse(readFileSync(INPUT_FILE, 'utf8'))
  // Резюмируемость: если выход уже есть — продолжаем с него (webEnrichment = метка).
  const enriched = existsSync(OUTPUT_FILE)
    ? JSON.parse(readFileSync(OUTPUT_FILE, 'utf8'))
    : objects
  const done = new Set(enriched.filter((o) => o.webEnrichment).map((o) => o.slug))

  const targets = enriched.filter((o) => hasGaps(o) && !done.has(o.slug)).slice(0, LIMIT)
  log.info(`объектов: ${enriched.length}, с дырами к обогащению: ${targets.length} (обработано ранее: ${done.size})`)
  if (DRY_RUN) {
    for (const o of targets) log.info(`  [dry] ${o.slug} — ${o.title}`)
    return
  }

  let processed = 0
  let totalFilled = 0
  for (const obj of targets) {
    try {
      const search = await searchTavily(obj)
      if (!search) {
        log.warn(`${obj.slug}: поиск пуст — пропуск`)
      } else {
        const extraction = await extractWithOllama(obj, search.context)
        if (extraction) {
          const filled = mergeExtraction(obj, extraction, search.sources)
          totalFilled += filled
          log.info(`${obj.slug}: +${filled} цен, sauna=${extraction.has_sauna} pets=${extraction.pets_allowed}`)
        }
      }
    } catch (err) {
      // Ошибка одного объекта не роняет прогон — идём дальше.
      log.warn(`${obj.slug}: ${err.message} — пропуск`)
    }
    processed += 1
    if (processed % CHECKPOINT_EVERY === 0) {
      writeFileSync(OUTPUT_FILE, JSON.stringify(enriched))
      log.info(`checkpoint: ${processed}/${targets.length}`)
    }
    await new Promise((r) => setTimeout(r, PAUSE_MS))
  }

  writeFileSync(OUTPUT_FILE, JSON.stringify(enriched))
  log.info(`готово: обработано ${processed}, подмешано цен ${totalFilled} → ${OUTPUT_FILE}`)
}

main().catch((err) => {
  log.warn('фатально:', err)
  process.exit(1)
})
