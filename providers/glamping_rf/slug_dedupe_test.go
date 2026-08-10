package glamping_rf

import (
	"testing"

	"vk-parser/internal/contract"
)

func obj(slug string, id int) contract.Object {
	return contract.Object{Slug: slug, SourceID: id}
}

func TestDedupeSlugs_OrderByAge(t *testing.T) {
	// Порядок в выдаче нарочно перемешан: якорь — SourceID, не позиция.
	out := dedupeSlugs([]contract.Object{
		obj("doma-u-ozera", 500), obj("edinstvennyy", 7),
		obj("doma-u-ozera", 100), obj("doma-u-ozera", 300),
	})
	got := map[int]string{}
	for _, o := range out {
		got[o.SourceID] = o.Slug
	}
	if got[100] != "doma-u-ozera" || got[300] != "doma-u-ozera-2" || got[500] != "doma-u-ozera-3" {
		t.Fatalf("нумерация не по возрасту: %v", got)
	}
	if got[7] != "edinstvennyy" {
		t.Fatalf("одиночка пострадал: %v", got)
	}
}

func TestDedupeSlugs_RealNameOccupiesSuffix(t *testing.T) {
	// Настоящий объект «...-2» уже существует — сгенерированный суффикс обязан
	// его обойти, а не растоптать.
	out := dedupeSlugs([]contract.Object{
		obj("dom", 10), obj("dom", 20), obj("dom-2", 5),
	})
	got := map[int]string{}
	for _, o := range out {
		got[o.SourceID] = o.Slug
	}
	if got[5] != "dom-2" {
		t.Fatalf("настоящий dom-2 растоптан: %v", got)
	}
	if got[10] != "dom" || got[20] != "dom-3" {
		t.Fatalf("обход занятого суффикса не сработал: %v", got)
	}
}

func TestDedupeSlugs_StableAcrossRuns(t *testing.T) {
	in1 := []contract.Object{obj("a", 2), obj("a", 1)}
	in2 := []contract.Object{obj("a", 1), obj("a", 2)} // другой порядок выдачи
	r1, r2 := map[int]string{}, map[int]string{}
	for _, o := range dedupeSlugs(in1) {
		r1[o.SourceID] = o.Slug
	}
	for _, o := range dedupeSlugs(in2) {
		r2[o.SourceID] = o.Slug
	}
	if r1[1] != r2[1] || r1[2] != r2[2] || r1[1] != "a" {
		t.Fatalf("нумерация зависит от порядка выдачи: %v vs %v", r1, r2)
	}
}
