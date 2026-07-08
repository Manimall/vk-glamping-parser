package images

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

const (
	// maxEdge — макс. сторона результата галереи (только уменьшение).
	maxEdge = 1280
	// webpQuality — качество webp галереи.
	webpQuality = 80

	// Обложка каталога — фикс. формат 3:2 под карточку фронта.
	coverWidth   = 1200
	coverHeight  = 800
	coverQuality = 82
	// coverModulate — лёгкая доводка обложки: +яркость,+насыщенность (magick
	// -modulate «brightness,saturation,hue»), чтобы кадр «играл», но без пережога.
	coverModulate = "104,110,100"
)

// runMagick пишет jpeg во временный файл и запускает magick: <temp> + args.
// Общий шелл-аут (без cgo) для галереи и обложки — чтобы не дублировать возню с
// временным файлом.
func runMagick(ctx context.Context, jpeg []byte, args ...string) error {
	tmp, err := os.CreateTemp("", "vkimg-*.jpg")
	if err != nil {
		return fmt.Errorf("images: temp file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(jpeg); err != nil {
		tmp.Close()
		return fmt.Errorf("images: write temp: %w", err)
	}
	tmp.Close()

	cmd := exec.CommandContext(ctx, "magick", append([]string{tmp.Name()}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("images: magick: %w: %s", err, out)
	}
	return nil
}

// encodeWebP: уменьшает до maxEdge (только вниз) и кодирует в webp.
func encodeWebP(ctx context.Context, jpeg []byte, dstPath string) error {
	resize := fmt.Sprintf("%dx%d>", maxEdge, maxEdge) // ">" — только уменьшать
	return runMagick(ctx, jpeg,
		"-resize", resize,
		"-quality", strconv.Itoa(webpQuality),
		"-define", "webp:method=6",
		dstPath,
	)
}

// encodeCover: готовит обложку карточки — заполнить и центр-кропнуть под 3:2
// (1200×800), лёгкая доводка, webp. Портретные VK-кадры кадрируются по центру
// (объект обычно в середине).
func encodeCover(ctx context.Context, jpeg []byte, dstPath string) error {
	dims := fmt.Sprintf("%dx%d", coverWidth, coverHeight)
	return runMagick(ctx, jpeg,
		"-resize", dims+"^", // заполнить 3:2 (по меньшей стороне)
		"-gravity", "center",
		"-extent", dims, // центр-кроп до 3:2
		"-modulate", coverModulate,
		"-quality", strconv.Itoa(coverQuality),
		"-define", "webp:method=6",
		dstPath,
	)
}
