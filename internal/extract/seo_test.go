package extract

import (
	"strings"
	"testing"
)

func TestBuildSEO_FromAbout(t *testing.T) {
	seo := BuildSEO(SEOInput{
		Name:     "AFRAME",
		Location: "Берёзовая роща, Ивановская обл.",
		About: "Уединённый и комфортный отдых на природе в уютном домике🌲🦦\n" +
			"🌲Вековой лес и Высоковское озеро\n" +
			"🧖🏼‍♀️ Купель Фурако\n" +
			"📍 18 км от Иваново\n⠀\n" +
			"📮Забронировать и узнать свободные даты — сообщения ВК/Telegram👇🏽",
	})

	// Subtitle — «живая» локация из about, а не структурный адрес.
	if seo.Subtitle != "18 км от Иваново" {
		t.Fatalf("subtitle = %q, ожидал «18 км от Иваново»", seo.Subtitle)
	}
	if seo.Title != "AFRAME — 18 км от Иваново" {
		t.Fatalf("title = %q", seo.Title)
	}
	d := seo.Description
	// Презентует место (питч из описания), а НЕ перечисляет удобства.
	if !strings.HasPrefix(d, "AFRAME — уединённый") {
		t.Errorf("описание должно начинаться «AFRAME — уединённый…»: %q", d)
	}
	if !strings.Contains(d, "Вековой лес и Высоковское озеро") ||
		!strings.Contains(d, "Купель Фурако") {
		t.Errorf("ожидал изюминки места в описании: %q", d)
	}
	if !strings.HasSuffix(d, seoCTA) {
		t.Errorf("описание должно кончаться призывом «%s»: %q", seoCTA, d)
	}
	// Строка-дистанция не дублируется в описании (она уже в Subtitle).
	if strings.Contains(d, "18 км") {
		t.Errorf("описание не должно дублировать дистанцию (она в subtitle): %q", d)
	}
	// Мусор (бронь/контакты) и эмодзи/невидимые символы — вычищены.
	for _, bad := range []string{"Забронировать", "Telegram", "🌲", "🧖", "⠀"} {
		if strings.Contains(d, bad) {
			t.Errorf("описание не должно содержать %q: %q", bad, d)
		}
	}
}

func TestBuildSEO_FallbackWhenNoAbout(t *testing.T) {
	seo := BuildSEO(SEOInput{
		Name:     "Домик",
		Location: "д. Крюково, Ивановская обл.",
		About:    "🌲\n📮 Забронировать — сообщения в Telegram", // питча нет, один мусор
	})
	if seo.Subtitle != "д. Крюково, Ивановская обл." {
		t.Fatalf("subtitle = %q, ожидал адрес-фоллбэк", seo.Subtitle)
	}
	if !strings.HasPrefix(seo.Description, "Домик — "+seoFallbackPitch) {
		t.Errorf("ожидал фоллбэк-питч: %q", seo.Description)
	}
	if !strings.Contains(seo.Description, "д. Крюково") ||
		!strings.HasSuffix(seo.Description, seoCTA) {
		t.Errorf("фоллбэк: локация + CTA: %q", seo.Description)
	}
}

func TestBuildSEO_EmptyName(t *testing.T) {
	if seo := BuildSEO(SEOInput{Name: "  "}); seo != (SEO{}) {
		t.Errorf("пустое имя → пустой SEO, получил %+v", seo)
	}
}
