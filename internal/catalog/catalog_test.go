package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vk-parser/internal/contract"
)

// writeProvider пишет generated/<name>/objects.json во временном каталоге теста.
func writeProvider(t *testing.T, dir, name string, objs []contract.Object) {
	t.Helper()
	pdir := filepath.Join(dir, name)
	if err := os.MkdirAll(pdir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(objs)
	if err := os.WriteFile(filepath.Join(pdir, objectsFile), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func obj(slug, title, price string) contract.Object {
	return contract.Object{
		Slug: slug, Title: title, Cover: "https://s/" + slug + ".webp",
		Cabins: []contract.Cabin{{Title: title, Price: price}},
	}
}

func TestGetAndList(t *testing.T) {
	dir := t.TempDir()
	writeProvider(t, dir, "vk", []contract.Object{obj("aframe-ivanovo", "AFRAME", "7 000 ₽")})
	writeProvider(t, dir, "glamping_rf", []contract.Object{obj("ilino-959", "Ильино", "7 360 ₽")})

	r := New(dir)

	got, ok := r.Get("aframe-ivanovo")
	if !ok || got.Title != "AFRAME" {
		t.Fatalf("Get(aframe-ivanovo) = %+v, %v", got, ok)
	}
	if _, ok := r.Get("nope"); ok {
		t.Fatal("Get(nope) должен вернуть false")
	}

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("List: ожидал 2 превью, получил %d", len(list))
	}
	// Превью содержит цену первого домика и обложку.
	for _, p := range list {
		if p.Price == "" || p.Cover == "" || p.Slug == "" {
			t.Errorf("превью неполное: %+v", p)
		}
	}
}

func TestHotReloadOnFileChange(t *testing.T) {
	dir := t.TempDir()
	writeProvider(t, dir, "vk", []contract.Object{obj("a", "Старое имя", "1 ₽")})
	r := New(dir)

	if got, _ := r.Get("a"); got.Title != "Старое имя" {
		t.Fatalf("до обновления: %+v", got)
	}

	time.Sleep(1100 * time.Millisecond) // mtime-гранулярность ФС — 1с
	writeProvider(t, dir, "vk", []contract.Object{obj("a", "Новое имя", "2 ₽")})

	if got, _ := r.Get("a"); got.Title != "Новое имя" {
		t.Fatalf("hot-reload не сработал: %+v", got)
	}
}

func TestSkipsBrokenAndDuplicates(t *testing.T) {
	dir := t.TempDir()
	writeProvider(t, dir, "vk", []contract.Object{
		obj("dup", "Первый", "1 ₽"),
		obj("dup", "Дубль", "2 ₽"), // дубль slug — пропущен
		{Title: "Без слага"},       // без slug — пропущен
	})
	// Битый JSON второго провайдера не роняет загрузку.
	pdir := filepath.Join(dir, "broken")
	_ = os.MkdirAll(pdir, 0o755)
	_ = os.WriteFile(filepath.Join(pdir, objectsFile), []byte("{не json"), 0o644)

	r := New(dir)
	list := r.List()
	if len(list) != 1 || list[0].Title != "Первый" {
		t.Fatalf("ожидал 1 объект «Первый», получил %+v", list)
	}
}

func TestEmptyDirIsNotFatal(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "нет-такого"))
	if got := r.List(); len(got) != 0 {
		t.Fatalf("пустой каталог: ожидал 0, получил %d", len(got))
	}
}
