package avito

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Поведение при кривом входе: что пропускаем с предупреждением, а что считаем
// фатальным. Граница важна — «успешный» пустой objects.json затёр бы прошлый
// сбор, а падение из-за одной чужой страницы обесценило бы ручное сохранение
// остальных.

func TestParseErrors(t *testing.T) {
	t.Run("каталог не задан", func(t *testing.T) {
		if _, err := New("").Parse(context.Background()); err == nil {
			t.Fatal("ожидал ошибку про --input")
		}
	})

	t.Run("каталога нет на диске", func(t *testing.T) {
		if _, err := New(filepath.Join(t.TempDir(), "нет-такого")).Parse(context.Background()); err == nil {
			t.Fatal("ожидал ошибку открытия каталога")
		}
	})

	t.Run("каталог без html", func(t *testing.T) {
		if _, err := New(t.TempDir()).Parse(context.Background()); err == nil {
			t.Fatal("ожидал ошибку про отсутствие .html")
		}
	})

	// Чужая страница не должна ронять сбор целиком — но если разобрать нечего
	// вовсе, это ошибка, а не «успешный» пустой objects.json.
	t.Run("не та страница", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "left.html"), []byte("<html>привет</html>"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := New(dir).Parse(context.Background()); err == nil {
			t.Fatal("ожидал ошибку: ни одна страница не разобрана")
		}
	})

	// Одна кривая копия рядом с рабочими страницами лишь пропускается.
	t.Run("кривой файл пропускается, остальные собираются", func(t *testing.T) {
		dir := t.TempDir()
		src, err := os.ReadFile(filepath.Join("testdata", "furako-muzykant-7826460306.html"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "ok.html"), src, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "broken.html"), []byte("<html>мусор</html>"), 0o644); err != nil {
			t.Fatal(err)
		}
		objects, err := New(dir).Parse(context.Background())
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(objects) != 1 || objects[0].SourceID != idFurako {
			t.Fatalf("объекты = %+v", objects)
		}
	})

	t.Run("отмена контекста", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := New("testdata").Parse(ctx); err == nil {
			t.Fatal("ожидал ошибку отмены")
		}
	})
}

// Слаг обязан быть уникальным: по нему строится адрес страницы каталога, а
// «Отдых для двоих» на Авито — название сразу у десятка объявлений.
func TestDedupeSlugs(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile(filepath.Join("testdata", "dlya-dvoih-7862471298.html"))
	if err != nil {
		t.Fatal(err)
	}
	// Копия того же объявления под другим id: подменяем идентификатор по всему
	// файлу, чтобы получить настоящего тёзку (то же название, другой объект),
	// а не дубль, который провайдер отсеет раньше проверки слагов.
	twin := strings.ReplaceAll(string(src), "7862471298", "7999999999")
	for name, data := range map[string]string{"a.html": string(src), "b.html": twin} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := New(dir).Parse(context.Background())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("объектов: %d, ожидалось 2 (тёзки, а не дубль)", len(got))
	}
	// Правило общее с glamping_rf (решение владельца 10.08): при тёзках id
	// получают ВСЕ в группе, а не только второй. Иначе URL уже опубликованного
	// объекта менялся бы от одного лишь появления соседа-тёзки.
	for _, o := range got {
		want := fmt.Sprintf("otdyh-dlya-dvoih-%d", o.SourceID)
		if o.Slug != want {
			t.Errorf("слаг %q, ожидался %q — тёзки обязаны нести id оба", o.Slug, want)
		}
	}
}
