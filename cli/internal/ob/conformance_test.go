package ob

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestRealConformance(t *testing.T) {
	path := os.Getenv("OB_REAL_DATA")
	if path == "" {
		t.Skip("set OB_REAL_DATA to release bundle for real catalog conformance")
	}
	x, e := Load(path)
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join("..", "..", "testdata", "real-golden.json"))
	if e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		Params map[string]string
		Result json.RawMessage
	}
	if e = json.Unmarshal(b, &cases); e != nil {
		t.Fatal(e)
	}
	for _, c := range cases {
		p := url.Values{}
		for k, v := range c.Params {
			p.Set(k, v)
		}
		t.Run(p.Encode(), func(t *testing.T) {
			q, e := Parse(p)
			if e != nil {
				t.Fatal(e)
			}
			r, e := x.Execute(q)
			if e != nil {
				t.Fatal(e)
			}
			got, _ := json.Marshal(r)
			var a, b any
			json.Unmarshal(got, &a)
			json.Unmarshal(c.Result, &b)
			if !reflect.DeepEqual(a, b) {
				var expected Result
				json.Unmarshal(c.Result, &expected)
				ids := func(ws []PublicWork) []string {
					r := []string{}
					for _, w := range ws {
						r = append(r, w.ID)
					}
					return r
				}
				t.Errorf("matched %d expected %d; IDs equal %v; concepts equal %v; facets equal %v\ngot first %v\nexpected first %v", r.Matched, expected.Matched, reflect.DeepEqual(ids(r.Works), ids(expected.Works)), reflect.DeepEqual(r.Concepts, expected.Concepts), reflect.DeepEqual(r.Facets, expected.Facets), ids(r.Works)[:min(8, len(r.Works))], ids(expected.Works)[:min(8, len(expected.Works))])
				os.WriteFile(filepath.Join(t.TempDir(), "actual.json"), got, 0600)
			}
		})
	}
}
func BenchmarkRealWarm(b *testing.B) {
	path := os.Getenv("OB_REAL_DATA")
	if path == "" {
		b.Skip("OB_REAL_DATA required")
	}
	x, e := Load(path)
	if e != nil {
		b.Fatal(e)
	}
	q, _ := Parse(url.Values{"q": {"Adams Bodomo"}})
	x.Execute(q)
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		if _, e := x.Execute(q); e != nil {
			b.Fatal(e)
		}
	}
}
