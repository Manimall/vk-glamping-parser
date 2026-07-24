package glamping_rf

// Ручной точечный ремонт битых объектов в generated/glamping_rf/objects.json:
// свежий fetchDetail + прод-пайплайн mergeDetail (About, SEO, cabins.summary).
//   go test ./providers/glamping_rf/ -run TestManualRefetch -manual-refetch -v
// Обычный прогон тестов его пропускает (без флага).

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"testing"
	"time"

	"vk-parser/internal/contract"
)

var manualRefetch = flag.Bool("manual-refetch", false, "точечный ремонт битых объектов")

func TestManualRefetch(t *testing.T) {
	if !*manualRefetch {
		t.Skip("ручной запуск: -manual-refetch")
	}
	const file = "../../generated/glamping_rf/objects.json"
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var objs []contract.Object
	if err := json.Unmarshal(raw, &objs); err != nil {
		t.Fatal(err)
	}

	targets := map[string]int{
		"otdyh-po-vzroslomu-v-lesu-1915": 1915,
		"finskiy-dom-na-lugu-618":        618,
	}
	c := newClient()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fixed := 0
	for i := range objs {
		id, ok := targets[objs[i].Slug]
		if !ok {
			continue
		}
		d, err := c.fetchDetail(ctx, id)
		if err != nil {
			t.Fatalf("id %d: %v", id, err)
		}
		if d.Description == "" {
			t.Fatalf("id %d: пустое описание — не чиним", id)
		}
		mergeDetail(&objs[i], d)
		fixed++
		t.Logf("%s: about %d символов: %.100s…", objs[i].Slug, len(objs[i].About), objs[i].About)
	}
	if fixed != len(targets) {
		t.Fatalf("починено %d из %d", fixed, len(targets))
	}
	out, err := json.MarshalIndent(objs, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
