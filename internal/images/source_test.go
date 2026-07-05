package images

import "testing"

func TestAlbumRefsFromDescription(t *testing.T) {
	desc := `Аренда дома.
ВСЕ ФОТО ДОМА : https://vk.com/album-211011668_282686075
...текст...
ещё: vk.com/album-211011668_282686075 (дубликат)
и другой: https://vk.com/album-211011668_282686121`

	refs := AlbumRefsFromDescription(desc)
	if len(refs) != 2 {
		t.Fatalf("ожидал 2 уникальных альбома, получил %d: %+v", len(refs), refs)
	}
	if refs[0].OwnerID != -211011668 || refs[0].AlbumID != "282686075" {
		t.Errorf("первый альбом разобран неверно: %+v", refs[0])
	}
	if refs[1].AlbumID != "282686121" {
		t.Errorf("второй альбом разобран неверно: %+v", refs[1])
	}
}

func TestAlbumRefsNone(t *testing.T) {
	if refs := AlbumRefsFromDescription("описание без ссылок на альбомы"); len(refs) != 0 {
		t.Errorf("ожидал 0 альбомов, получил %+v", refs)
	}
}
