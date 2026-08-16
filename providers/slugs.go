package providers

import (
	"fmt"
	"sort"

	"vk-parser/internal/contract"
)

// DedupeSlugs разводит тёзок в выдаче одного провайдера.
//
// Тёзки после ухода id из слага (issue #26): «Дома у озера» встречается
// трижды. Правило (решение владельца 10.08): имя уникально в выдаче — слаг
// чистый; есть тёзки — ВСЕ в группе остаются с id (doma-u-ozera-1579).
// Id вечен, поэтому URL тёзок не зависят друг от друга: смерть или появление
// соседа-тёзки никого не переименовывает. Известный трейд схемы: появление
// тёзки у ранее уникального имени переводит и его на слаг с id.
//
// Живёт здесь, а не внутри провайдера, потому что правило про URL, а не про
// источник. Второй экземпляр этой логики неизбежно разошёлся бы с первым — и
// разошёлся: у Авито названия ещё типовее («Отдых для двоих» — на десяток
// объявлений), а первая версия его дедупа оставляла первому чистый слаг и
// сортировала по имени файла, из-за чего URL уже опубликованного объекта
// менялся от появления соседа.
//
// Полагается на инвариант «SourceID уникален и > 0» — провайдер обязан
// отфильтровать объекты без id до вызова.
func DedupeSlugs(objects []contract.Object) []contract.Object {
	counts := make(map[string]int, len(objects))
	for _, o := range objects {
		counts[o.Slug]++
	}

	// Предрегистрация ВСЕХ баз (принцип из ревью PR #27): сгенерированный
	// base-<id> не растопчет настоящее имя другого объекта («Дом 2»).
	used := make(map[string]bool, len(objects))
	for s := range counts {
		used[s] = true
	}

	// Тёзки обходятся по возрастанию SourceID: выдача не зависит от порядка
	// листинга — у glamping_rf от пагинации, у Авито от имени файла на диске.
	homonyms := make([]int, 0, len(objects))
	for i, o := range objects {
		if counts[o.Slug] > 1 {
			homonyms = append(homonyms, i)
		}
	}
	sort.Slice(homonyms, func(a, b int) bool {
		return objects[homonyms[a]].SourceID < objects[homonyms[b]].SourceID
	})

	for _, i := range homonyms {
		candidate := fmt.Sprintf("%s-%d", objects[i].Slug, objects[i].SourceID)
		// Достижимо только когда base-<id> совпал с настоящим именем
		// другого объекта («Дом 2»).
		for used[candidate] {
			candidate += "-2"
		}
		used[candidate] = true
		objects[i].Slug = candidate
	}
	return objects
}
