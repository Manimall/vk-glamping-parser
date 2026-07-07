package images

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"  // регистрируем декодеры для image.Decode
	_ "image/jpeg" //
	_ "image/png"  //
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/corona10/goimagehash"
)

const (
	// dupHammingThreshold — макс. расстояние перцептивных хэшей, при котором
	// кадры считаем дублями (одинаковые репосты идут с расстоянием 0).
	dupHammingThreshold = 8
)

// photo — кадр с посчитанными признаками для отбора.
type photo struct {
	data    []byte
	hash    *goimagehash.ImageHash
	hasFace bool
	outdoor float64 // «уличность» (доля зелени/неба) — для обложки/порядка
	w, h    int     // размеры кадра — насколько красиво ляжет в 3:2 обложки
}

// Process превращает сырые скачанные фото в обложку cover.webp + галерею
// photo-1..N.webp в outDir: декод → дедуп → порядок (без людей вперёд, лучший
// экстерьер первый) → обложка (лучший кадр, кроп 3:2) → ресайз/webp галереи.
// Обложку ИСКЛЮЧАЕМ из галереи (issue #6: желательно, чтобы обложки не было
// среди фото карусели). Возвращает число записанных фото галереи. Ошибка одного
// кадра не роняет остальные (graceful, лог WARN).
func Process(ctx context.Context, raws [][]byte, outDir string, limit int) (int, error) {
	photos := make([]photo, 0, len(raws))
	for i, data := range raws {
		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			slog.Warn("images: decode skipped", "idx", i, "err", err)
			continue
		}
		h, err := Hash(img)
		if err != nil {
			slog.Warn("images: hash skipped", "idx", i, "err", err)
			continue
		}
		b := img.Bounds()
		photos = append(photos, photo{
			data:    data,
			hash:    h,
			hasFace: HasFace(img),
			outdoor: OutdoorScore(img),
			w:       b.Dx(),
			h:       b.Dy(),
		})
	}

	photos = dedup(photos)
	photos = order(photos)
	cover, gallery := splitCoverGallery(photos, limit)

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, fmt.Errorf("images: mkdir %s: %w", outDir, err)
	}
	if err := clearWebp(outDir); err != nil {
		return 0, err
	}

	// Обложка — лучший кадр (экстерьер без людей), кропнутый под 3:2. Он же
	// исключён из галереи (см. splitCoverGallery).
	if cover != nil {
		slog.Info("images: обложка выбрана",
			"outdoor", cover.outdoor, "w", cover.w, "h", cover.h, "score", coverScore(*cover))
		dst := filepath.Join(outDir, "cover.webp")
		if err := encodeCover(ctx, cover.data, dst); err != nil {
			slog.Warn("images: cover encode skipped", "err", err)
		}
	}

	written := 0
	for i, p := range gallery {
		dst := filepath.Join(outDir, fmt.Sprintf("photo-%d.webp", i+1))
		if err := encodeWebP(ctx, p.data, dst); err != nil {
			slog.Warn("images: encode skipped", "file", dst, "err", err)
			continue
		}
		written++
	}
	return written, nil
}

const (
	// coverAspectIdeal — целевые пропорции обложки (3:2).
	coverAspectIdeal = 1.5
	// coverOutdoorSweet — «золотая середина» уличности для обложки: домик СО
	// средой (двор/лес/небо). Слишком высокая уличность = чистый лес/небо без
	// объекта; слишком низкая = интерьер. Пик оценки — около этого значения.
	coverOutdoorSweet = 0.6
)

// coverScore — насколько кадр годится в обложку. Два множителя:
//   - outdoorFit: близость «уличности» к coverOutdoorSweet (домик + среда, а не
//     чистый лес или интерьер);
//   - fit: как красиво кадр ляжет в 3:2 без сильной обрезки (портрет режется
//     сильнее ландшафта → ниже, обложка выглядит пропорциональнее).
func coverScore(p photo) float64 {
	if p.h == 0 {
		return 0
	}
	fit := (float64(p.w) / float64(p.h)) / coverAspectIdeal
	if fit > 1 {
		fit = 1 / fit // симметрично вокруг 3:2: слишком широкие тоже хуже
	}
	outdoorFit := 1 - math.Abs(p.outdoor-coverOutdoorSweet)/coverOutdoorSweet
	if outdoorFit < 0 {
		outdoorFit = 0
	}
	// «Что на кадре» (домик + среда) важнее «как ляжет» — иначе побеждает любой
	// ландшафтный кадр (чистый лес) над вертикальным кадром с домиком.
	return outdoorFit * (0.85 + 0.15*fit)
}

// splitCoverGallery выбирает обложку и галерею. Обложка — кадр без лица с
// максимальным coverScore (уличный и хорошо ложится в 3:2). Его ИСКЛЮЧАЕМ из
// галереи (issue #6). Галерея — остальные в исходном порядке, не более limit.
func splitCoverGallery(photos []photo, limit int) (*photo, []photo) {
	if len(photos) == 0 {
		return nil, nil
	}
	best, bestScore := 0, -1.0
	for i, p := range photos {
		if p.hasFace {
			continue // на обложке — без людей
		}
		if s := coverScore(p); s > bestScore {
			best, bestScore = i, s
		}
	}
	cover := &photos[best]
	gallery := make([]photo, 0, len(photos)-1)
	gallery = append(gallery, photos[:best]...)
	gallery = append(gallery, photos[best+1:]...)
	if limit > 0 && len(gallery) > limit {
		gallery = gallery[:limit]
	}
	return cover, gallery
}

// dedup убирает похожие кадры по перцептивному хэшу, сохраняя порядок.
func dedup(in []photo) []photo {
	out := make([]photo, 0, len(in))
	for _, p := range in {
		isDup := false
		for _, kept := range out {
			if d, err := p.hash.Distance(kept.hash); err == nil && d <= dupHammingThreshold {
				isDup = true
				break
			}
		}
		if !isDup {
			out = append(out, p)
		}
	}
	return out
}

// order задаёт порядок галереи:
//   - кадры без лиц идут первыми (→ первые ~10 без людей), с лицами — в конец;
//   - внутри «без лиц» экстерьеры (высокий OutdoorScore) идут раньше интерьеров,
//     поэтому обложка получается обзорной (лес/двор), а не деталью (ванна).
//
// Сортировка стабильная — при равном score сохраняется порядок альбома.
func order(in []photo) []photo {
	noFace := make([]photo, 0, len(in))
	face := make([]photo, 0)
	for _, p := range in {
		if p.hasFace {
			face = append(face, p)
		} else {
			noFace = append(noFace, p)
		}
	}
	sort.SliceStable(noFace, func(i, j int) bool { return noFace[i].outdoor > noFace[j].outdoor })
	return append(noFace, face...)
}

// clearWebp удаляет старые photo-*.webp перед перезаписью.
func clearWebp(dir string) error {
	matches, _ := filepath.Glob(filepath.Join(dir, "photo-*.webp"))
	for _, m := range matches {
		if err := os.Remove(m); err != nil {
			return fmt.Errorf("images: clear %s: %w", m, err)
		}
	}
	return nil
}
