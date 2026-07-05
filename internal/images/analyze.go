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

const (
	// faceQualityThreshold — порог качества детекции лица (pigo). Выше — меньше
	// ложных срабатываний.
	faceQualityThreshold = 6.0
	// analysisWidth — до какой ширины уменьшаем кадр перед анализом (скорость).
	analysisWidth = 480
	// Параметры каскада pigo (рекомендованные значения из документации pigo).
	faceMinSize     = 40  // мин. сторона лица в пикселях уменьшенного кадра
	faceShiftFactor = 0.1 // шаг скользящего окна (доля размера окна)
	faceScaleFactor = 1.1 // во сколько раз растёт окно между проходами
	faceClusterIoU  = 0.2 // порог слияния пересекающихся детекций
)

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
		MinSize:     faceMinSize,
		MaxSize:     cols, // лицо может занимать почти весь кадр
		ShiftFactor: faceShiftFactor,
		ScaleFactor: faceScaleFactor,
		ImageParams: pigo.ImageParams{Pixels: gray, Rows: rows, Cols: cols, Dim: cols},
	}

	dets := faceClassifier.RunCascade(params, 0.0)
	dets = faceClassifier.ClusterDetections(dets, faceClusterIoU)
	for _, d := range dets {
		if d.Q > faceQualityThreshold {
			return true
		}
	}
	return false
}

const (
	// scoreWidth — до какой ширины уменьшаем кадр для оценки «уличности».
	scoreWidth = 160
	// Цветовые пороги «уличности» (0..255 на канал).
	greenMinLevel    = 60  // насколько ярким должен быть зелёный у листвы
	channelDominance = 10  // на сколько доминирующий канал превышает другие
	skyBlueMinLevel  = 120 // насколько ярким должен быть синий у неба
	skyGreenSlack    = 20  // допуск: синее небо может быть чуть зеленее (b8>=g8-slack)
)

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
			case g8 > greenMinLevel && g8 > r8+channelDominance && g8 > b8+channelDominance: // листва
				outdoor++
			case b8 > skyBlueMinLevel && b8 > r8+channelDominance && b8 >= g8-skyGreenSlack: // синее небо
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
