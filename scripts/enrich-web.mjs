#!/usr/bin/env node
// Обогащение объектов каталога данными из интернета: Tavily (поиск) + локальная
// Ollama (извлечение структуры). Итерирует по объектам с ДЫРАМИ (услуги без
// цены / нет правил), ищет официальные источники, извлекает структуру LLM-ом и
// подмешивает БЕЗ перезаписи данных источника. Результат — enriched_objects.json.
// Логика поиска/извлечения/merge — в lib/enrich.mjs.
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
//   - цены вне здравого диапазона отбрасываются (см. lib/enrich.mjs).

import { readFileSync, writeFileSync, existsSync } from 'node:fs'
import { resolve, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { searchTavily, extractWithOllama, mergeExtraction, assertOllamaAlive } from './lib/enrich.mjs'

// ── Конфиг путей и темпа ─────────────────────────────────────────────────────
const ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const INPUT_FILE = resolve(ROOT, 'generated/glamping_rf/objects.json')
const OUTPUT_FILE = resolve(ROOT, 'generated/glamping_rf/enriched_objects.json')
const PAUSE_MS = 6000 // dev-тир Tavily жёстко режет частоту — щадящий темп
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
  const pricelessExtra = (obj.extras ?? []).some((e) => !e.price)
  const rules = obj.cabins?.[0]?.property?.rules ?? []
  return pricelessExtra || rules.length === 0
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
  await assertOllamaAlive() // fail fast: без LLM прогон бессмыслен

  let processed = 0
  let totalFilled = 0
  for (const obj of targets) {
    try {
      const search = await searchTavily(obj)
      if (!search) {
        log.warn(`${obj.slug}: поиск пуст — пропуск`)
      } else {
        const extraction = await extractWithOllama(obj, search.context)
        const filled = mergeExtraction(obj, extraction, search.sources)
        totalFilled += filled
        const foundExtra = obj.webEnrichment.foundServices?.length ?? 0
        log.info(`${obj.slug}: +${filled} цен (${foundExtra} новых в foundServices), sauna=${extraction.has_sauna} pets=${extraction.pets_allowed}`)
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
