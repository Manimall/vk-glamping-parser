// Package images — конвейер обработки фото объекта: выбор источника (альбом дома
// вместо стены), дедуп, отбор порядка (обзорные/без людей вперёд), ресайз в webp.
package images

import (
	"regexp"
	"strconv"
)

// AlbumRef — ссылка на альбом VK: владелец (со знаком) + id альбома.
type AlbumRef struct {
	OwnerID int64
	AlbumID string
}

// reAlbum ловит ссылки вида .../album-211011668_282686075 в тексте описания
// («ВСЕ ФОТО ДОМА: vk.com/album-<owner>_<album>»). Владелец группы — отрицательный.
var reAlbum = regexp.MustCompile(`album(-?\d+)_(\d+)`)

// AlbumRefsFromDescription достаёт все альбомы из текста описания товара.
// Дедупим по (owner,album), сохраняя порядок появления.
func AlbumRefsFromDescription(desc string) []AlbumRef {
	matches := reAlbum.FindAllStringSubmatch(desc, -1)
	seen := make(map[string]bool)
	refs := make([]AlbumRef, 0, len(matches))
	for _, m := range matches {
		owner, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil {
			continue
		}
		key := m[1] + "_" + m[2]
		if seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, AlbumRef{OwnerID: owner, AlbumID: m[2]})
	}
	return refs
}
