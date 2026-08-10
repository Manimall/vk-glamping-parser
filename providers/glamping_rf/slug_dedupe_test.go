package glamping_rf

import (
	"testing"

	"vk-parser/internal/contract"
)

func obj(slug string, id int) contract.Object {
	return contract.Object{Slug: slug, SourceID: id}
}

func slugsByID(objects []contract.Object) map[int]string {
	out := map[int]string{}
	for _, o := range objects {
		out[o.SourceID] = o.Slug
	}
	return out
}

func TestDedupeSlugs_HomonymsKeepSourceID(t *testing.T) {
	got := slugsByID(dedupeSlugs([]contract.Object{
		obj("doma-u-ozera", 1585), obj("edinstvennyy", 7), obj("doma-u-ozera", 1579),
	}))
	if got[1579] != "doma-u-ozera-1579" || got[1585] != "doma-u-ozera-1585" {
		t.Fatalf("тёзки должны остаться с id: %v", got)
	}
	if got[7] != "edinstvennyy" {
		t.Fatalf("уникальное имя должно быть чистым: %v", got)
	}
}

func TestDedupeSlugs_IndependentOfNeighbours(t *testing.T) {
	// Смерть тёзки не переименовывает выживших: слаг зависит только от
	// собственного id, каскада суффиксов нет.
	three := slugsByID(dedupeSlugs([]contract.Object{
		obj("dom", 10), obj("dom", 20), obj("dom", 30),
	}))
	two := slugsByID(dedupeSlugs([]contract.Object{
		obj("dom", 20), obj("dom", 30), // id=10 «умер»
	}))
	if two[20] != three[20] || two[30] != three[30] {
		t.Fatalf("уход соседа изменил чужие слаги: %v → %v", three, two)
	}
}

func TestDedupeSlugs_DoubleGroups(t *testing.T) {
	// Кейс «дубль×дубль» из ревью: группы «dom» и «dom-2» (настоящее имя
	// «Дом 2») не пересекаются — каждый несёт свой id.
	got := slugsByID(dedupeSlugs([]contract.Object{
		obj("dom", 10), obj("dom", 20), obj("dom-2", 5), obj("dom-2", 15),
	}))
	want := map[int]string{10: "dom-10", 20: "dom-20", 5: "dom-2-5", 15: "dom-2-15"}
	for id, w := range want {
		if got[id] != w {
			t.Fatalf("id=%d: %q, ожидал %q (всё: %v)", id, got[id], w, got)
		}
	}
}

func TestDedupeSlugs_GeneratedNeverTramplesRealName(t *testing.T) {
	// Тёзка с id=2 породила бы «dom-2» — а это настоящее имя другого объекта.
	got := slugsByID(dedupeSlugs([]contract.Object{
		obj("dom", 2), obj("dom", 7), obj("dom-2", 99),
	}))
	if got[99] != "dom-2" {
		t.Fatalf("настоящий «Дом 2» растоптан: %v", got)
	}
	if got[2] != "dom-2-2" || got[7] != "dom-7" {
		t.Fatalf("коллизия сгенерированного с настоящим: %v", got)
	}
}

func TestDedupeSlugs_DeterministicAcrossRuns(t *testing.T) {
	// Детектор недетерминизма map-порядка из ревью: 20 прогонов — один ответ.
	mk := func() []contract.Object {
		return []contract.Object{
			obj("dom", 2), obj("dom", 7), obj("dom-2", 99),
			obj("doma-u-ozera", 1585), obj("doma-u-ozera", 1579),
		}
	}
	first := slugsByID(dedupeSlugs(mk()))
	for i := 0; i < 20; i++ {
		if got := slugsByID(dedupeSlugs(mk())); len(got) != len(first) ||
			got[2] != first[2] || got[99] != first[99] || got[1579] != first[1579] {
			t.Fatalf("прогон %d дал другую выдачу: %v vs %v", i, got, first)
		}
	}
}
