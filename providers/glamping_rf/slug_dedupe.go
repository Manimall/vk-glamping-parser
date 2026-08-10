package glamping_rf

// Тёзки после ухода id из слага (issue #26): «Дома у озера» встречается трижды.
// Правило: старший по SourceID держит чистый слаг НАВСЕГДА (id новичка всегда
// больше — новый тёзка не отберёт URL у старожила), остальные — -2, -3 в
// порядке возраста. Суффикс проверяется на занятость глобально: настоящий
// объект «Дома у озера 2» не будет растоптан сгенерированным «-2».

import (
	"fmt"
	"sort"

	"vk-parser/internal/contract"
)

func dedupeSlugs(objects []contract.Object) []contract.Object {
	byBase := make(map[string][]int, len(objects))
	for i, o := range objects {
		byBase[o.Slug] = append(byBase[o.Slug], i)
	}
	used := make(map[string]bool, len(objects))
	for base, idxs := range byBase {
		if len(idxs) == 1 {
			used[base] = true
		}
	}
	for base, idxs := range byBase {
		if len(idxs) == 1 {
			continue
		}
		sort.Slice(idxs, func(a, b int) bool {
			return objects[idxs[a]].SourceID < objects[idxs[b]].SourceID
		})
		for pos, i := range idxs {
			candidate := base
			if pos > 0 || used[base] {
				for n := pos + 1; ; n++ {
					candidate = fmt.Sprintf("%s-%d", base, n)
					if !used[candidate] {
						break
					}
				}
			}
			used[candidate] = true
			objects[i].Slug = candidate
		}
	}
	return objects
}
