package glamping_rf

import (
	"strings"
	"testing"
)

// Инлайн-скрипт внутри desc-блока должен вырезаться ЦЕЛИКОМ (с содержимым):
// tagRe снимает только теги, и JS-код становился «текстом» описания — так у
// объектов 1915/618 в about уехал обрывок шаблонизатора галереи.
func TestFullDescriptionStripsInlineScript(t *testing.T) {
	page := `<div data-pv12-desc-full class="x">` +
		`<p>Уютный дом у озера.</p>` +
		`<script>var m10 = n % 10; r.images.forEach(function (img) { photosHtml += img.full; });</script>` +
		`<style>.gallery { display: none; }</style>` +
		`<p>Баня и мангал на террасе.</p>` +
		`</div>`

	got := fullDescription(page)
	want := "Уютный дом у озера. Баня и мангал на террасе."
	if got != want {
		t.Errorf("fullDescription: %q, want %q", got, want)
	}
	for _, leak := range []string{"forEach", "photosHtml", "m10", "gallery"} {
		if strings.Contains(got, leak) {
			t.Errorf("утечка кода в описание: %q содержит %q", got, leak)
		}
	}
}

// Блок целиком из скрипта → пустая строка (фоллбэк на ld+json), а не код.
func TestFullDescriptionScriptOnlyBlock(t *testing.T) {
	page := `<div data-pv12-desc-full>` +
		`<script>var a = 1 && b; document.write(a);</script>` +
		`</div>`
	if got := fullDescription(page); got != "" {
		t.Errorf("ожидалась пустая строка, получено %q", got)
	}
}

// Маркер в JS-строке (querySelector в тогглере) при ОТСУТСТВИИ div-блока —
// не описание: ожидаем пустую строку (фоллбэк на ld+json), не куски кода.
func TestFullDescriptionMarkerInsideScriptOnly(t *testing.T) {
	page := `<script>
        if (m100 >= 11 && m100 <= 19) return 'отзывов';
        var el = document.querySelectorAll('[data-pv12-desc-full]');
        el.forEach(function (x) { x.hidden = false; });
    </script><div class="reviews">Отзывы гостей</div>`
	if got := fullDescription(page); got != "" {
		t.Errorf("ожидалась пустая строка, получено %q", got)
	}
}
