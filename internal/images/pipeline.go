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
}

// Process превращает сырые скачанные фото в готовую галерею photo-1..N.webp в
// outDir: декод → дедуп → порядок (без людей вперёд, обложка = первый обзорный
// кадр из порядка альбома) → ресайз/webp. Возвращает число записанных файлов.
// Ошибка одного кадра не роняет остальные (graceful, лог WARN).
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
		photos = append(photos, photo{
			data:    data,
			hash:    h,
			hasFace: HasFace(img),
			outdoor: OutdoorScore(img),
		})
	}

	photos = dedup(photos)
	photos = order(photos)
	if limit > 0 && len(photos) > limit {
		photos = photos[:limit]
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, fmt.Errorf("images: mkdir %s: %w", outDir, err)
	}
	if err := clearWebp(outDir); err != nil {
		return 0, err
	}

	written := 0
	for i, p := range photos {
		dst := filepath.Join(outDir, fmt.Sprintf("photo-%d.webp", i+1))
		if err := encodeWebP(ctx, p.data, dst); err != nil {
			slog.Warn("images: encode skipped", "file", dst, "err", err)
			continue
		}
		written++
	}
	return written, nil
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
