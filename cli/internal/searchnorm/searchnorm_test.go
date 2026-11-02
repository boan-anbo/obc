package searchnorm

import (
	_ "embed"
	"encoding/json"
	"reflect"
	"testing"
)

//go:embed data/vectors.v1.json
var vectorJSON []byte

type pair[T any] struct {
	Input  string `json:"input"`
	Output T      `json:"output"`
}

func TestSharedVectors(t *testing.T) {
	var v struct {
		Schema        string             `json:"schema"`
		Normalise     []pair[string]     `json:"normalise"`
		Literal       []pair[string]     `json:"literal"`
		IndexTokens   []pair[[]string]   `json:"indexTokens"`
		QueryTokens   []pair[[]string]   `json:"queryTokens"`
		Romanise      []pair[string]     `json:"romanise"`
		PinyinQueries []pair[[][]string] `json:"pinyinQueries"`
		NameReadings  []pair[[]string]   `json:"nameReadings"`
		SameForm      [][]string         `json:"sameForm"`
		DifferentForm [][]string         `json:"differentForm"`
	}
	if e := json.Unmarshal(vectorJSON, &v); e != nil {
		t.Fatal(e)
	}
	if v.Schema != "openbibliographies.search-vectors.v1" {
		t.Fatal(v.Schema)
	}
	n := 0
	check := func(name, input string, got, want any) {
		t.Helper()
		n++
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s(%q) = %#v, want %#v", name, input, got, want)
		}
	}
	for _, c := range v.Normalise {
		check("normalise", c.Input, Normalise(c.Input), c.Output)
		check("idempotent", c.Input, Normalise(Normalise(c.Input)), c.Output)
		n--
	}
	for _, c := range v.Literal {
		check("literal", c.Input, Literal(c.Input), c.Output)
	}
	for _, c := range v.IndexTokens {
		check("indexTokens", c.Input, HanTokens(c.Input, false), c.Output)
	}
	for _, c := range v.QueryTokens {
		check("queryTokens", c.Input, HanTokens(c.Input, true), c.Output)
	}
	for _, c := range v.Romanise {
		check("romanise", c.Input, Romanise(c.Input), c.Output)
	}
	for _, c := range v.PinyinQueries {
		check("pinyinQueries", c.Input, PinyinQueries(c.Input), c.Output)
	}
	for _, c := range v.NameReadings {
		check("nameReadings", c.Input, NameReadings(c.Input), c.Output)
	}
	for _, c := range v.SameForm {
		check("sameForm", c[0], Normalise(c[0]), Normalise(c[1]))
	}
	for _, c := range v.DifferentForm {
		n++
		if Normalise(c[0]) == Normalise(c[1]) {
			t.Errorf("differentForm %q %q merged", c[0], c[1])
		}
	}
	if n != 114 {
		t.Fatalf("ran %d vectors, want the 114 published", n)
	}
}

func TestSyllableMatching(t *testing.T) {
	title := Romanise("红太阳是怎样升起的：延安")
	if !TitleMatch(title, []string{"hong", "tai", "yang"}) || TitleMatch(title, []string{"tai", "hong"}) {
		t.Fatal(title)
	}
	names := Romanise("高华") + " | " + Romanise("张三")
	if !NameMatch(names, []string{"gao", "hua"}) || !NameMatch(names, []string{"hua", "gao"}) || NameMatch(names, []string{"gao"}) {
		t.Fatal(names)
	}
}

func TestAJapaneseRecordHasNoPinyin(t *testing.T) {
	if got := RomaniseRecord("文体の変遷", "", []string{"高華"}); got != ([2]string{}) {
		t.Fatal(got)
	}
	if got := RomaniseRecord("高华", "ja", []string{"高華"}); got != ([2]string{}) {
		t.Fatal(got)
	}
	if got := RomaniseRecord("高华", "zh", []string{"高華", "Gao"}); got != ([2]string{"gao hua", "gao hua"}) {
		t.Fatal(got)
	}
}
