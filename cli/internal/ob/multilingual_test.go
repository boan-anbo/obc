package ob

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func multilingualFixture(t *testing.T) []byte {
	t.Helper()
	x := fixtureIndex(t)
	c := x.Catalog
	// One accepted record with explicit variants; no generated spelling/translation.
	c.Works[0].Title = "四位一體的社會學之道"
	c.Works[0].Language = "zh"
	c.Works[0].TitleRomanized = "Siwei yiti de shehuixue zhi dao"
	c.Works[0].TitleEnglish = &EnglishTitle{Value: "The Four-in-One Way of Sociology", Editorial: true}
	c.Works[0].Creators = []Creator{{Name: "謝國雄", Role: "author", Romanized: "Shieh, Gwo-Shyong"}}
	var overlay AuthorOverlay
	if e := Decode([]byte(x.Bundle.Retrieval.OverlayJSON), &overlay); e != nil {
		t.Fatal(e)
	}
	// Fixture uses no reviewed author associations for this synthetic work.
	groups := overlay.Groups[:0]
	for _, g := range overlay.Groups {
		members := g.Members[:0]
		for _, m := range g.Members {
			if m.Work != c.Works[0].ID {
				members = append(members, m)
			}
		}
		g.Members = members
		if len(members) > 0 {
			groups = append(groups, g)
		}
	}
	overlay.Groups = groups
	cat, e := json.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	x.Bundle.Catalog = string(cat)
	x.Bundle.Retrieval.CatalogSHA256 = Digest(cat)
	overlay.CatalogSHA256 = Digest(cat)
	overlay.Catalog = "/data/catalog/" + Digest(cat)[:16] + ".json"
	o, e := json.Marshal(overlay)
	if e != nil {
		t.Fatal(e)
	}
	x.Bundle.Retrieval.OverlayJSON = string(o)
	x.Bundle.Retrieval.OverlaySHA256 = Digest(o)
	b, e := json.Marshal(x.Bundle)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestMultilingualKnownFieldsSearchAndPublicRoundTrip(t *testing.T) {
	b := multilingualFixture(t)
	x, e := LoadBytes(b)
	if e != nil {
		t.Fatal(e)
	}
	id := x.Catalog.Works[0].ID
	for _, q := range []string{"四位一體的社會學之道", "Siwei yiti de shehuixue zhi dao", "The Four-in-One Way of Sociology", "謝國雄", "Shieh, Gwo-Shyong"} {
		t.Run(q, func(t *testing.T) {
			query, e := Parse(url.Values{"q": {q}, "limit": {"100"}})
			if e != nil {
				t.Fatal(e)
			}
			r, e := x.Execute(query)
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, w := range r.Works {
				found = found || w.ID == id
			}
			if !found {
				t.Fatalf("accepted variant not retrieved: %s", q)
			}
		})
	}
	out, e := json.Marshal(Public(x.Catalog.Works[0]))
	if e != nil {
		t.Fatal(e)
	}
	var w Work
	// Public has an href field in addition to these same Work fields.
	var obj map[string]json.RawMessage
	json.Unmarshal(out, &obj)
	delete(obj, "href")
	raw, _ := json.Marshal(obj)
	if e = Decode(raw, &w); e != nil || w.TitleRomanized == "" || w.TitleEnglish == nil || !w.TitleEnglish.Editorial || w.Creators[0].Romanized == "" || w.Language != "zh" {
		t.Fatal("lost accepted public fields", string(out), e)
	}
}
func TestMultilingualStrictKnownTypesAndUpdateFailureKeepsCache(t *testing.T) {
	good := fixtureBytes(t)
	root := t.TempDir()
	cache := filepath.Join(root, "cache")
	old := filepath.Join(root, "old.json")
	os.WriteFile(old, good, 0600)
	if _, e := Update(context.Background(), cache, "", old, nil); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(filepath.Join(cache, "current.json"))
	multi := multilingualFixture(t)
	for _, invalid := range []struct{ from, to string }{{`"editorial":true`, `"editorial":"yes"`}, {`"language":"zh"`, `"language":["zh"]`}, {`"romanized":"Shieh, Gwo-Shyong"`, `"romanized":[]`}, {`"titleRomanized":`, `"unknownTitleVariant":`}} {
		// Modify decoded catalog, then repair only its outer hash, so refusal tests the schema itself.
		var b Bundle
		json.Unmarshal(multi, &b)
		b.Catalog = string(bytes.Replace([]byte(b.Catalog), []byte(invalid.from), []byte(invalid.to), 1))
		b.Retrieval.CatalogSHA256 = Digest([]byte(b.Catalog))
		var o AuthorOverlay
		json.Unmarshal([]byte(b.Retrieval.OverlayJSON), &o)
		o.CatalogSHA256 = b.Retrieval.CatalogSHA256
		o.Catalog = "/data/catalog/" + o.CatalogSHA256[:16] + ".json"
		ob, _ := json.Marshal(o)
		b.Retrieval.OverlayJSON = string(ob)
		b.Retrieval.OverlaySHA256 = Digest(ob)
		raw, _ := json.Marshal(b)
		p := filepath.Join(root, "bad.json")
		os.WriteFile(p, raw, 0600)
		if _, e := Update(context.Background(), cache, "", p, nil); e == nil {
			t.Fatal("unknown/wrong-type field accepted", invalid)
		}
		after, _ := os.ReadFile(filepath.Join(cache, "current.json"))
		if !bytes.Equal(before, after) {
			t.Fatal("failed validation changed cache")
		}
	}
}

func TestRomanisedAndIdentityMatches(t *testing.T) {
	work := func(id, title, language string, creators ...string) Work {
		w := Work{ID: id, Title: title, Language: language, Labels: map[string][]string{}}
		for _, c := range creators {
			w.Creators = append(w.Creators, Creator{Name: c, Role: "author"})
		}
		return w
	}
	x := &Index{Catalog: Catalog{Works: []Work{
		work("1", "紅太陽是怎樣升起的", "zh", "高華"),
		work("2", "How the Red Sun Rose: The Origins", "en", "Gao Hua"),
		work("3", "文体の変遷", "ja", "高華"),
		work("4", "Max Weber on China", "en", "Otto"),
	}}, Names: map[string]map[int]bool{"高华": {0: true, 2: true}, "gao hua": {1: true}, "otto": {3: true}}, Search: &SearchIndex{DF: map[string]int{}}}
	for _, w := range x.Catalog.Works {
		x.Titles = append(x.Titles, literal(w.Title))
		x.TitleVariants = append(x.TitleVariants, []string{literal(w.Title)})
		x.Romanised = append(x.Romanised, romanisedRecord(w))
	}
	found := func(q string, main map[int]float64) map[int]float64 { return x.withRomanised(q, main) }
	if got := found("hong tai yang", nil); len(got) != 1 || got[0] == 0 {
		t.Fatal("a title spelled in pinyin", got)
	}
	if got := found("gaohua", nil); got[0] == 0 || got[2] != 0 {
		t.Fatal("a name spelled in pinyin, and no pinyin for a Japanese record", got)
	}
	if got := found("高華", nil); got[0] == 0 || got[1] == 0 || got[2] == 0 {
		t.Fatal("a Han name reaches its literal records and its romanised one", got)
	}
	if got := found("Max Weber on China", map[int]float64{}); got[3] == 0 {
		t.Fatal("an exact title is always found", got)
	}
	if got := found("How the Red Sun Rose", nil); got[1] == 0 {
		t.Fatal("an exact main title is always found", got)
	}
	if got := found("no such thing", map[int]float64{}); len(got) != 0 {
		t.Fatal(got)
	}
}
