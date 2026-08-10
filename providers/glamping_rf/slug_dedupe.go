package glamping_rf

// Тёзки после ухода id из слага (issue #26): «Дома у озера» встречается
// трижды. Правило (решение владельца 10.08): имя уникально в выдаче — слаг
// чистый; есть тёзки — ВСЕ в группе остаются с id (doma-u-ozera-1579).
// Id вечен, поэтому URL тёзок не зависят друг от друга: смерть или появление
// соседа-тёзки никого не переименовывает. Известный трейд схемы: появление
// тёзки у ранее уникального имени переводит и его на слаг с id.
import (
	"fmt"
	"sort"

	"vk-parser/internal/contract"
)

// dedupeSlugs полагается на инвариант «SourceID уникален и > 0» — его даёт
// фильтр it.ID == 0 в collectPlace.
func dedupeSlugs(objects []contract.Object) []contract.Object {
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
	// листинга даже в ветке коллизии ниже.
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
