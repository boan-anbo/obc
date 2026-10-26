package ob

import (
	"encoding/json"
	"net/url"
	"testing"
)

func relationsFixture(t *testing.T) (*Index, Catalog) {
	t.Helper()
	x := fixtureIndex(t)
	c := x.Catalog
	if len(c.Works) < 2 {
		t.Fatal("fixture needs pair")
	}
	c.Works[0].Relations = Relations{{Type: "hasVersion", Work: c.Works[1].ID}}
	c.Works[1].Relations = Relations{{Type: "isVersionOf", Work: c.Works[0].ID}}
	return x, c
}
func TestRelationsKnownFieldsPublicAndQueryJSON(t *testing.T) {
	x, c := relationsFixture(t)
	raw := authorKeyBundle(t, x, c)
	next, e := LoadBytes(raw)
	if e != nil {
		t.Fatal(e)
	}
	out, e := json.Marshal(Public(next.Catalog.Works[0]))
	if e != nil {
		t.Fatal(e)
	}
	var shown PublicWork
	if e = Decode(out, &shown); e != nil || len(shown.Relations) != 1 || shown.Relations[0].Work != c.Works[1].ID {
		t.Fatal(string(out), e)
	}
	q, e := Parse(url.Values{"limit": {"100"}})
	if e != nil {
		t.Fatal(e)
	}
	result, e := next.Execute(q)
	if e != nil {
		t.Fatal(e)
	}
	seen := 0
	for _, w := range result.Works {
		if w.ID == c.Works[0].ID || w.ID == c.Works[1].ID {
			if len(w.Relations) != 1 {
				t.Fatal("query lost relations")
			}
			seen++
		}
	}
	if seen != 2 {
		t.Fatal("pair missing from normal result")
	}
}
func TestRelationsStrictTypesAndCounterparts(t *testing.T) {
	for _, raw := range []string{`{"relations":[]}`, `{"relations":null}`, `{"relations":[{"type":"isTranslationOf","work":"1"}]}`, `{"relations":[{"type":"hasVersion","work":"1","evidence":"private"}]}`, `{"relations":[{"type":"hasVersion","work":1}]}`} {
		var w Work
		if e := Decode([]byte(raw), &w); e == nil {
			t.Fatal("unknown/invalid relation accepted", raw)
		}
	}
	for _, mode := range []string{"missing", "inverse", "type"} {
		t.Run(mode, func(t *testing.T) {
			x, c := relationsFixture(t)
			switch mode {
			case "missing":
				c.Works[0].Relations[0].Work = "absent-target"
			case "inverse":
				c.Works[1].Relations = nil
			default:
				c.Works[1].Relations[0].Type = "hasVersion"
			}
			if _, e := LoadBytes(authorKeyBundle(t, x, c)); e == nil {
				t.Fatal("invalid counterpart accepted")
			}
		})
	}
}
