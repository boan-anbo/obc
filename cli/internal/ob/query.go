package ob

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

type Error struct {
	Code    string
	Message string
	Exit    int
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }
func Invalid(s string) error   { return &Error{"INVALID_QUERY", s, 2} }

type Query struct {
	Q        string              `json:"q"`
	Author   string              `json:"author"`
	Filters  map[string][]string `json:"filters"`
	Sort     string              `json:"sort"`
	Offset   int                 `json:"-"`
	Limit    int                 `json:"-"`
	Output   string              `json:"-"`
	Snapshot string              `json:"-"`
	Facets   []string            `json:"-"`
}

func Parse(p url.Values) (Query, error) {
	q := Query{Q: p.Get("q"), Author: p.Get("author"), Sort: p.Get("sort"), Limit: 50, Output: p.Get("output"), Snapshot: p.Get("snapshot"), Filters: map[string][]string{}}
	if q.Sort == "" {
		q.Sort = "recommended"
	}
	if q.Output == "" {
		q.Output = "json"
	}
	allowed := append(slices.Clone(Facets), "q", "author", "sort", "offset", "limit", "output", "snapshot", "facets")
	for k, vs := range p {
		if !slices.Contains(allowed, k) || len(vs) != 1 {
			return q, Invalid("Unknown or repeated parameter: " + k)
		}
	}
	if !slices.Contains([]string{"recommended", "title", "year-new", "year-old"}, q.Sort) {
		return q, Invalid("Unknown sort")
	}
	if !slices.Contains([]string{"json", "jsonl"}, q.Output) {
		return q, Invalid("Use json or jsonl output; Markdown is not supported")
	}
	if utf8.RuneCountInString(q.Q) > 512 {
		return q, Invalid("Query exceeds 512 characters")
	}
	distinct := map[string]bool{}
	for _, t := range Tokens(q.Q, true) {
		distinct[t] = true
	}
	if len(distinct) > 30 {
		return q, Invalid("Query exceeds 30 search terms")
	}
	if len(utf16.Encode([]rune(q.Author))) > 256 {
		return q, Invalid("Selection is too large")
	}
	if q.Snapshot != "" && !regexp.MustCompile(`^[a-zA-Z0-9-]{1,80}$`).MatchString(q.Snapshot) {
		return q, Invalid("Invalid snapshot")
	}
	for _, k := range []string{"limit", "offset"} {
		if vs, ok := p[k]; ok {
			v := vs[0]
			if !regexp.MustCompile(`^\d+$`).MatchString(v) {
				return q, Invalid("Invalid " + k)
			}
			n, e := strconv.ParseUint(v, 10, 53)
			if e != nil || n > 9007199254740991 {
				return q, Invalid("Invalid " + k)
			}
			if k == "limit" {
				if n < 1 || n > 100 {
					return q, Invalid("Invalid limit")
				}
				q.Limit = int(n)
			} else {
				q.Offset = int(n)
			}
		}
	}
	list := func(k string) []string {
		out := []string{}
		for _, v := range strings.Split(p.Get(k), ",") {
			if v != "" {
				out = appendUnique(out, v)
			}
		}
		return out
	}
	for _, k := range Facets {
		q.Filters[k] = list(k)
		if len(q.Filters[k]) > 100 {
			return q, Invalid("Selection is too large")
		}
		for _, v := range q.Filters[k] {
			if len(utf16.Encode([]rune(v))) > 128 {
				return q, Invalid("Selection is too large")
			}
		}
	}
	q.Facets = list("facets")
	for _, d := range q.Facets {
		if !slices.Contains(Facets, d) {
			return q, Invalid("Unknown facet")
		}
	}
	return q, nil
}

type PublicWork struct {
	ID             string              `json:"id"`
	Title          string              `json:"title"`
	Language       string              `json:"language,omitempty"`
	TitleRomanized string              `json:"titleRomanized,omitempty"`
	TitleEnglish   *EnglishTitle       `json:"titleEnglish,omitempty"`
	Creators       []Creator           `json:"creators"`
	Format         string              `json:"format"`
	Labels         map[string][]string `json:"labels"`
	Year           *int                `json:"year,omitempty"`
	Venue          string              `json:"venue,omitempty"`
	WorkType       []string            `json:"workType,omitempty"`
	ISBN           []string            `json:"isbn,omitempty"`
	DOI            string              `json:"doi,omitempty"`
	Href           string              `json:"href"`
	Relations      Relations           `json:"relations,omitempty"`
}

func Public(w Work) PublicWork {
	return PublicWork{ID: w.ID, Title: w.Title, Language: w.Language, TitleRomanized: w.TitleRomanized, TitleEnglish: w.TitleEnglish, Creators: w.Creators, Format: w.Format, Labels: w.Labels, Year: w.Year, Venue: w.Venue, WorkType: w.WorkType, ISBN: w.ISBN, DOI: w.DOI, Href: "/works/" + w.ID + "/", Relations: w.Relations}
}

type Concepts struct {
	Named []string `json:"named"`
	Near  []string `json:"near"`
}
type Result struct {
	Schema         string                    `json:"schema"`
	Snapshot       string                    `json:"snapshot"`
	Development    bool                      `json:"development"`
	Request        Query                     `json:"request"`
	Matched        int                       `json:"matched"`
	Returned       int                       `json:"returned"`
	Offset         int                       `json:"offset"`
	Limit          int                       `json:"limit"`
	NextOffset     *int                      `json:"nextOffset"`
	Works          []PublicWork              `json:"works"`
	Facets         map[string]map[string]int `json:"facets"`
	Concepts       Concepts                  `json:"concepts"`
	SuggestedWorks []PublicWork              `json:"suggestedWorks"`
}

func (x *Index) Execute(q Query) (Result, error) {
	r := Result{Schema: "openbibliographies.query.v1", Snapshot: x.Bundle.Snapshot, Development: x.Bundle.Development, Request: q, Offset: q.Offset, Limit: q.Limit, Works: []PublicWork{}, Facets: map[string]map[string]int{}, Concepts: Concepts{[]string{}, []string{}}, SuggestedWorks: []PublicWork{}}
	if q.Snapshot != "" && q.Snapshot != r.Snapshot {
		return r, &Error{"SNAPSHOT_CHANGED", "Use current snapshot and restart pagination", 6}
	}
	authors := map[int]bool{}
	if q.Author != "" {
		if direct := x.Authors[q.Author]; direct != nil {
			for i := range direct {
				authors[i] = true
			}
		}
		groups := []AuthorGroup{}
		for _, g := range x.Bundle.Retrieval.Authors {
			for _, m := range g.Members {
				if authorSlug(m.Name) == q.Author {
					groups = append(groups, g)
					break
				}
			}
		}
		if len(groups) == 1 {
			for i := range x.Authors[groups[0].Key] {
				authors[i] = true
			}
		}
		if len(authors) == 0 {
			return r, Invalid("Unknown author")
		}
	}
	for _, d := range Facets {
		for _, id := range q.Filters[d] {
			known := false
			switch d {
			case "format":
				known = slices.Contains(Formats, id)
			case "workType":
				known = slices.Contains(WorkTypes, id)
			default:
				_, known = x.Terms[d+":"+id]
				if d == "region" && id == "china-all" {
					known = true
				}
			}
			if !known {
				return r, Invalid(fmt.Sprintf("Unknown %s: %s", d, id))
			}
		}
	}
	scores := x.scores(q.Q)
	matches := func(i int, skip string) bool {
		if q.Author != "" && !authors[i] {
			return false
		}
		if scores != nil {
			if _, ok := scores[i]; !ok {
				return false
			}
		}
		for _, d := range Facets {
			if d == skip || len(q.Filters[d]) == 0 {
				continue
			}
			yes := false
			for _, id := range q.Filters[d] {
				if x.Keys[i][d+":"+id] {
					yes = true
					break
				}
			}
			if !yes {
				return false
			}
		}
		return true
	}
	found := []int{}
	for i := range x.Catalog.Works {
		if matches(i, "") {
			found = append(found, i)
		}
	}
	slices.SortStableFunc(found, func(a, b int) int {
		if q.Sort == "recommended" {
			if scores != nil {
				if scores[a] > scores[b] {
					return -1
				}
				if scores[a] < scores[b] {
					return 1
				}
			}
			return a - b
		}
		wa, wb := x.Catalog.Works[a], x.Catalog.Works[b]
		if q.Sort != "title" {
			if wa.Year == nil && wb.Year != nil {
				return 1
			}
			if wa.Year != nil && wb.Year == nil {
				return -1
			}
			if wa.Year != nil && wb.Year != nil && *wa.Year != *wb.Year {
				if q.Sort == "year-new" {
					return *wb.Year - *wa.Year
				}
				return *wa.Year - *wb.Year
			}
		}
		return x.TitleRanks[a] - x.TitleRanks[b]
	})
	r.Matched = len(found)
	start := min(q.Offset, len(found))
	end := start + min(q.Limit, len(found)-start)
	for _, i := range found[start:end] {
		r.Works = append(r.Works, Public(x.Catalog.Works[i]))
	}
	r.Returned = len(r.Works)
	if end < len(found) {
		r.NextOffset = &end
	}
	for _, d := range q.Facets {
		counts := map[string]int{}
		prefix := d + ":"
		for i, keys := range x.Keys {
			if matches(i, d) {
				for k := range keys {
					if strings.HasPrefix(k, prefix) {
						counts[strings.TrimPrefix(k, prefix)]++
					}
				}
			}
		}
		r.Facets[d] = counts
	}
	if strings.TrimSpace(q.Q) != "" {
		key := strings.TrimSpace(strings.ReplaceAll(lower(norm.NFKC.String(q.Q)), "-", " "))
		r.Concepts.Named = append(r.Concepts.Named, x.TermNames[key]...)
		for _, k := range r.Concepts.Named {
			for _, near := range x.Terms[k].Near {
				if !slices.Contains(r.Concepts.Named, near) {
					r.Concepts.Near = appendUnique(r.Concepts.Near, near)
				}
			}
		}
	}
	return r, nil
}
