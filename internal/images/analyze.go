package images

import (
	_ "embed"
	"fmt"
	"image"
	"image/draw"

	"github.com/corona10/goimagehash"
	pigo "github.com/esimov/pigo/core"
	"github.com/nfnt/resize"
)

// Каскад для детекции лиц зашит в бинарь — пакет самодостаточен.
//
//go:embed assets/facefinder
var cascade []byte

// faceClassifier — распакованный каскад pigo. Инициализируется один раз.
var faceClassifier *pigo.Pigo

func init() {
	c, err := pigo.NewPigo().Unpack(cascade)
	if err != nil {
		panic(fmt.Sprintf("images: unpack face cascade: %v", err))
	}
	faceClassifier = c
}

// Порог качества детекции лица (pigo). Выше — меньше ложных срабатываний.
const faceQualityThreshold = 6.0

// analysisWidth — до какой ширины уменьшаем кадр перед анализом (скорость).
const analysisWidth = 480

// Hash считает перцептивный хэш кадра (для дедупа похожих).
func Hash(img image.Image) (*goimagehash.ImageHash, error) {
	return goimagehash.PerceptionHash(img)
}

// HasFace — есть ли на кадре фронтальное лицо (best-effort через pigo).
// ВАЖНО: детектор фронтальных лиц; человека со спины/сбоку может не найти —
// поэтому это дополнение к главному приёму «брать фото из альбома дома».
func HasFace(img image.Image) bool {
	small := resize.Resize(analysisWidth, 0, img, resize.Bilinear)
	nrgba := toNRGBA(small)
	gray := pigo.RgbToGrayscale(nrgba)
	cols, rows := nrgba.Bounds().Dx(), nrgba.Bounds().Dy()

	params := pigo.CascadeParams{
		MinSize:     40,
		MaxSize:     cols, // лицо может занимать почти весь кадр
		ShiftFactor: 0.1,
		ScaleFactor: 1.1,
		ImageParams: pigo.ImageParams{Pixels: gray, Rows: rows, Cols: cols, Dim: cols},
	}

	dets := faceClassifier.RunCascade(params, 0.0)
	dets = faceClassifier.ClusterDetections(dets, 0.2)
	for _, d := range dets {
		if d.Q > faceQualityThreshold {
			return true
		}
	}
	return false
}

// scoreWidth — до какой ширины уменьшаем кадр для оценки «уличности» (скорость).
const scoreWidth = 160

// OutdoorScore — грубая эвристика «обзорность/экстерьер»: доля пикселей похожих
// на листву (зелёный) или небо (яркий синий). У экстерьеров с природой она
// высокая, у интерьеров (белые стены, ванна) — низкая. Истинная классификация
// экстерьер/интерьер требует вижн-ML; это дешёвый пуре-Go прокси для выбора
// обложки и порядка (обзорные вперёд). Заснеженные кадры набирают мало — это ок:
// на обложку просто попадёт зелёный/лесной кадр.
func OutdoorScore(img image.Image) float64 {
	small := resize.Resize(scoreWidth, 0, img, resize.Bilinear)
	b := small.Bounds()
	total, outdoor := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := small.At(x, y).RGBA()
			r8, g8, b8 := int(r>>8), int(g>>8), int(bl>>8)
			total++
			switch {
			case g8 > 60 && g8 > r8+10 && g8 > b8+10: // листва
				outdoor++
			case b8 > 120 && b8 > r8+10 && b8 >= g8-20: // синее небо
				outdoor++
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(outdoor) / float64(total)
}

// toNRGBA приводит любой image.Image к *image.NRGBA (нужно pigo).
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	dst := image.NewNRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}
