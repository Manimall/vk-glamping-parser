package glamping_rf

import "testing"

// Название с HTML-сущностями: декод в Title, чистый slug без «quot» (issue #23).
func TestToObject_TitleHTMLEntities(t *testing.T) {
	it := sampleItem()
	it.NameNew = "База отдыха &quot;Веденье&quot;"
	it.ID = 838

	o := toObject(it)

	if o.Title != `База отдыха "Веденье"` {
		t.Errorf("title=%q — сущности не декодированы", o.Title)
	}
	if o.Slug != "baza-otdyha-vedene" {
		t.Errorf("slug=%q, ожидал baza-otdyha-vedene", o.Slug)
	}
	if o.Cabins[0].Title != o.Title {
		t.Errorf("в кабину уехал недекодированный title: %q", o.Cabins[0].Title)
	}
}
