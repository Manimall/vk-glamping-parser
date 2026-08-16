#!/usr/bin/env python3
"""Готовит компактную фикстуру из страницы Авито, сохранённой браузером.

Зачем. Репозиторий публичный, а сохранённая страница — это мегабайт чужого
контента: полное описание объявления, имя и аватар продавца, служебные хэши
площадки. Класть такое в git нельзя. Но и синтетические фикстуры — самообман:
проверять надо ровно тот формат, который реально отдаёт Авито.

Компромисс: из настоящей страницы вырезается только то, что читает провайдер
(providers/avito/types.go), описание обрезается до нескольких строк, всё
остальное отбрасывается. Формат обёртки сохранён — тот же маркер
window.__staticRouterHydrationData и та же двойная упаковка JSON, поэтому
hydration.go проверяется целиком, а не в обход.

Запуск:
    python3 make_fixture.py "<сохранённая страница>.html" <имя-фикстуры>.html
"""
import json
import re
import sys

MARKER = "window.__staticRouterHydrationData = JSON.parse("
ROUTE = "catalog-or-main-or-item"
# Описание нужно ради проверки подменённых букв — для этого хватает первых строк.
DESCRIPTION_LIMIT = 400

# Поля объявления, которые читает провайдер. Всё, чего здесь нет, в фикстуру
# не попадает — в том числе продавец, отзывы и реклама.
ITEM_FIELDS = (
    "id", "title", "price", "address",
    "geo", "location",
    "imageUrls", "images", "listingImage",
    "priceList", "phone", "seo",
)


def read_state(html: str) -> dict:
    at = html.index(MARKER)
    rest = html[at + len(MARKER):]
    start = rest.index('"')
    i = start + 1
    while i < len(rest):
        if rest[i] == "\\":
            i += 2
            continue
        if rest[i] == '"':
            return json.loads(json.loads(rest[start:i + 1]))
        i += 1
    raise SystemExit("строка состояния не закрыта — файл обрезан?")


def trim(buyer_item: dict) -> dict:
    item = buyer_item["item"]
    slim = {k: item[k] for k in ITEM_FIELDS if k in item}
    for field in ("description", "descriptionHtml"):
        if item.get(field):
            slim[field] = item[field][:DESCRIPTION_LIMIT]
    # Рейтинг принадлежит продавцу: берём числа, выбрасываем userKey (хэш).
    out = {"item": slim}
    if rating := buyer_item.get("rating"):
        out["rating"] = {
            "scoreFloat": rating.get("scoreFloat"),
            "summary": rating.get("summary"),
            "activeReviewsCount": rating.get("activeReviewsCount"),
        }
    return out


def main() -> None:
    if len(sys.argv) != 3:
        raise SystemExit(__doc__)
    src, dst = sys.argv[1], sys.argv[2]

    state = read_state(open(src, encoding="utf-8", errors="replace").read())
    buyer_item = trim(state["loaderData"][ROUTE]["buyerItem"])
    slim = {"loaderData": {ROUTE: {"buyerItem": buyer_item}}}

    # Та же упаковка, что у Авито: JSON внутри JSON-строки внутри HTML.
    payload = json.dumps(json.dumps(slim, ensure_ascii=False), ensure_ascii=False)
    html = (
        "<!DOCTYPE html>\n<html lang=\"ru\"><head><meta charset=\"utf-8\">\n"
        "<!-- Фикстура: вырезка из страницы Авито, сохранённой браузером.\n"
        "     Генератор: providers/avito/testdata/make_fixture.py -->\n"
        "</head><body>\n<script>"
        f"{MARKER}{payload});"
        "</script>\n</body></html>\n"
    )
    open(dst, "w", encoding="utf-8").write(html)
    item = buyer_item["item"]
    print(f"{dst}: id={item['id']} «{item['title']}» — {len(html) // 1024} КБ")


if __name__ == "__main__":
    main()
