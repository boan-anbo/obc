package ob

import (
	"encoding/json"
	"net/url"
	"testing"
)

func authorKeyBundle(t *testing.T, x *Index, c Catalog) []byte {
	t.Helper()
	var overlay AuthorOverlay
	if e := Decode([]byte(x.Bundle.Retrieval.OverlayJSON), &overlay); e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	x.Bundle.Catalog = string(raw)
	x.Bundle.Retrieval.CatalogSHA256 = Digest(raw)
	overlay.CatalogSHA256 = Digest(raw)
	overlay.Catalog = "/data/catalog/" + Digest(raw)[:16] + ".json"
	raw, e = json.Marshal(overlay)
	if e != nil {
		t.Fatal(e)
	}
	x.Bundle.Retrieval.OverlayJSON = string(raw)
	x.Bundle.Retrieval.OverlaySHA256 = Digest(raw)
	raw, e = json.Marshal(x.Bundle)
	if e != nil {
		t.Fatal(e)
	}
	return raw
}

func TestAuthorKeyObservedRomanizedFallbackAndDualRole(t *testing.T) {
	x, e := LoadBytes(multilingualFixture(t))
	if e != nil {
		t.Fatal(e)
	}
	c := x.Catalog
	id := c.Works[0].ID
	c.Works[0].Creators = append(c.Works[0].Creators, Creator{Name: "謝國雄", Role: "editor", Romanized: "Shieh, Gwo-Shyong"})
	x, e = LoadBytes(authorKeyBundle(t, x, c))
	if e != nil {
		t.Fatal(e)
	}
	key := authorSlug("Shieh, Gwo-Shyong")
	if !x.Authors[key][0] || len(x.Authors[key]) != 1 || len(x.Authors[authorSlug("謝國雄")]) != 0 {
		t.Fatal("author fallback/dedup differs from shared workAuthorSlug")
	}
	q, e := Parse(url.Values{"author": {key}, "limit": {"100"}})
	if e != nil {
		t.Fatal(e)
	}
	result, e := x.Execute(q)
	if e != nil || len(result.Works) != 1 || result.Works[0].ID != id {
		t.Fatalf("normal author filter lost work: %+v %v", result, e)
	}
}

func TestAuthorKeyReviewedGroupPrecedesRomanized(t *testing.T) {
	x := fixtureIndex(t)
	c := x.Catalog
	memberCreator := c.Works[0].Creators[0]
	g := AuthorGroup{Key: "reviewed-person", Name: "Reviewed Fixture Person", Members: []Member{{Work: c.Works[0].ID, Name: memberCreator.Name, Role: memberCreator.Role}}}
	var overlay AuthorOverlay
	if e := Decode([]byte(x.Bundle.Retrieval.OverlayJSON), &overlay); e != nil {
		t.Fatal(e)
	}
	overlay.Groups = []AuthorGroup{g}
	overlayRaw, e := json.Marshal(overlay)
	if e != nil {
		t.Fatal(e)
	}
	x.Bundle.Retrieval.OverlayJSON = string(overlayRaw)
	x.Bundle.Retrieval.OverlaySHA256 = Digest(overlayRaw)
	member := g.Members[0]
	found := false
	for i := range c.Works {
		if c.Works[i].ID != member.Work {
			continue
		}
		for j := range c.Works[i].Creators {
			cr := &c.Works[i].Creators[j]
			if cr.Name == member.Name && cr.Role == member.Role {
				cr.Romanized = "Separate Supplied Spelling"
				found = true
			}
		}
	}
	if !found {
		t.Fatal("missing reviewed fixture member")
	}
	x, e = LoadBytes(authorKeyBundle(t, x, c))
	if e != nil {
		t.Fatal(e)
	}
	if len(x.Authors[authorSlug("Separate Supplied Spelling")]) != 0 || len(x.Authors[g.Key]) == 0 {
		t.Fatal("fallback overrode reviewed group")
	}
}
