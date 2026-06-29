package vk

import "testing"

// photoWith — фото с одним размером заданной площади (для теста отбора).
func photoWith(url string, w, h int) photo {
	return photo{Sizes: []photoSize{{URL: url, Width: w, Height: h}}}
}

func TestSelectBestPhotos(t *testing.T) {
	// Пять фото разной площади. Лучшие 3 по площади: B(big), D(big), A(mid).
	photos := []photo{
		photoWith("A", 800, 600),  // 480000
		photoWith("B", 1000, 800), // 800000 — крупное
		photoWith("C", 100, 100),  // 10000  — мелкое (репост)
		photoWith("D", 1200, 700), // 840000 — крупное
		photoWith("E", 200, 150),  // 30000  — мелкое
	}

	got := selectBestPhotos(photos, 3)
	if len(got) != 3 {
		t.Fatalf("ожидал 3 фото, получил %d: %v", len(got), got)
	}
	// Должны вернуться в ИСХОДНОМ порядке (A раньше B раньше D), а не по площади.
	want := []string{"A", "B", "D"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] получил %q, ожидал %q (порядок галереи)", i, got[i], want[i])
		}
	}
}

func TestSelectBestPhotosUnderLimit(t *testing.T) {
	photos := []photo{photoWith("A", 10, 10), photoWith("B", 20, 20)}
	got := selectBestPhotos(photos, 15)
	if len(got) != 2 {
		t.Fatalf("фоток меньше лимита — ожидал все 2, получил %d", len(got))
	}
}

func TestBestPhotoURL(t *testing.T) {
	sizes := []photoSize{
		{URL: "small", Width: 100, Height: 100},
		{URL: "big", Width: 1000, Height: 1000},
		{URL: "mid", Width: 500, Height: 500},
	}
	url, area := bestPhotoURL(sizes)
	if url != "big" || area != 1000*1000 {
		t.Errorf("получил (%q,%d), ожидал ('big',1000000)", url, area)
	}
}
