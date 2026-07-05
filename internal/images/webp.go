package images

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

const (
	// maxEdge — макс. сторона результата (только уменьшение).
	maxEdge = 1280
	// webpQuality — качество webp.
	webpQuality = 80
)

// encodeWebP берёт JPEG-байты, уменьшает до maxEdge (только вниз) и кодирует в
// webp через imagemagick. Шелл-аут выбран сознательно: без cgo, и это ровно тот
// эталон, что применяли вручную (`magick -resize '1280x1280>' -quality 80`).
func encodeWebP(ctx context.Context, jpeg []byte, dstPath string) error {
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

	resizeArg := fmt.Sprintf("%dx%d>", maxEdge, maxEdge) // ">" — только уменьшать
	cmd := exec.CommandContext(ctx, "magick",
		tmp.Name(),
		"-resize", resizeArg,
		"-quality", strconv.Itoa(webpQuality),
		"-define", "webp:method=6",
		dstPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("images: magick encode: %w: %s", err, out)
	}
	return nil
}
