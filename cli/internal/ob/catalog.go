package ob

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

const BundleSchema = "openbibliographies.bundle.v1"
const MaxData = 64 << 20

var Dimensions = []string{"discipline", "region", "period", "subject", "topic", "keyword"}
var Facets = []string{"discipline", "region", "period", "subject", "topic", "format", "workType"}
var Formats = []string{"book", "article", "chapter", "review", "other"}
var WorkTypes = []string{"study", "survey", "reference", "translation", "edition", "source-collection", "catalogue", "collection"}

type Creator struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Romanized string `json:"romanized,omitempty"`
}
type EnglishTitle struct {
	Value     string `json:"value"`
	Editorial bool   `json:"editorial"`
}
type Work struct {
	ID             string              `json:"id"`
	Title          string              `json:"title"`
	Language       string              `json:"language,omitempty"`
	TitleRomanized string              `json:"titleRomanized,omitempty"`
	TitleEnglish   *EnglishTitle       `json:"titleEnglish,omitempty"`
	Creators       []Creator           `json:"creators"`
	Year           *int                `json:"year,omitempty"`
	Venue          string              `json:"venue,omitempty"`
	Format         string              `json:"format"`
	WorkType       []string            `json:"workType,omitempty"`
	Labels         map[string][]string `json:"labels"`
	ISBN           []string            `json:"isbn,omitempty"`
	DOI            string              `json:"doi,omitempty"`
	Related        []int               `json:"related,omitempty"`
}
type Term struct {
	ID         string   `json:"id"`
	En         string   `json:"en"`
	Zh         string   `json:"zh"`
	Zht        string   `json:"zht"`
	Parents    []string `json:"parents"`
	Aliases    []string `json:"aliases,omitempty"`
	Near       []string `json:"near,omitempty"`
	Navigation []string `json:"navigation,omitempty"`
	Grouping   bool     `json:"grouping,omitempty"`
	Order      *float64 `json:"order,omitempty"`
}
type Catalog struct {
	Schema                string            `json:"schema"`
	Preview               bool              `json:"preview"`
	Generated             string            `json:"generated"`
	ClassificationVersion string            `json:"classificationVersion"`
	Taxonomy              map[string][]Term `json:"taxonomy"`
	Works                 []Work            `json:"works"`
	Coverage              map[string]int    `json:"coverage"`
}
type Member struct {
	Work string `json:"work"`
	Name string `json:"name"`
	Role string `json:"role"`
}
type AuthorGroup struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Members []Member `json:"members"`
}
type Retrieval struct {
	Schema          string        `json:"schema"`
	CatalogSHA256   string        `json:"catalogSHA256"`
	AuthorVersion   string        `json:"authorVersion"`
	SemanticsSHA256 string        `json:"semanticsSHA256"`
	OverlayJSON     string        `json:"authorOverlayJSON"`
	OverlaySHA256   string        `json:"authorOverlaySHA256"`
	Authors         []AuthorGroup `json:"-"`
	TitleOrder      []int         `json:"titleOrder"`
}
type AuthorOverlay struct {
	Schema        string        `json:"schema"`
	Snapshot      string        `json:"snapshot"`
	Catalog       string        `json:"catalog"`
	CatalogSHA256 string        `json:"catalogSha256"`
	Version       string        `json:"version"`
	MappingSHA256 string        `json:"mappingSha256"`
	Groups        []AuthorGroup `json:"groups"`
}
type Bundle struct {
	Schema      string    `json:"schema"`
	Snapshot    string    `json:"snapshot"`
	Development bool      `json:"development"`
	Source      string    `json:"source"`
	Catalog     string    `json:"catalogJSON"`
	Retrieval   Retrieval `json:"retrieval"`
}
type Index struct {
	Bundle        Bundle
	Catalog       Catalog
	Keys          []map[string]bool
	Terms         map[string]Term
	Authors       map[string]map[int]bool
	Names         map[string]map[int]bool
	TitleRanks    []int
	Titles        []string
	TitleVariants [][]string
	TermNames     map[string][]string
	Phrases       []Phrase
	Periods       map[string][]Span
	TitlePeriods  [][]Span
	Search        *SearchIndex
}
type Phrase struct {
	Value string
	Keys  []string
}

func Digest(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func Decode(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	if d.Decode(new(any)) != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
func ReadBounded(r io.Reader, max int64) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, max+1))
	if e != nil {
		return nil, e
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("data exceeds %d bytes", max)
	}
	return b, nil
}
func ReadFile(path string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return ReadBounded(f, MaxData)
}
func Load(path string) (*Index, error) {
	b, e := ReadFile(path)
	if e != nil {
		return nil, e
	}
	return LoadBytes(b)
}
func Inflate(b []byte) ([]byte, error) {
	if len(b) > MaxData {
		return nil, fmt.Errorf("dataset too large")
	}
	if len(b) > 2 && b[0] == 31 && b[1] == 139 {
		z, e := gzip.NewReader(bytes.NewReader(b))
		if e != nil {
			return nil, e
		}
		defer z.Close()
		b, e = ReadBounded(z, MaxData)
		if e != nil {
			return nil, e
		}
	}
	return b, nil
}
func LoadBytes(b []byte) (*Index, error) {
	b, e := Inflate(b)
	if e != nil {
		return nil, e
	}
	var bundle Bundle
	if e := Decode(b, &bundle); e != nil {
		return nil, e
	}
	return Build(bundle)
}

var validID = regexp.MustCompile(`^[a-z0-9-]+$`)
var validHash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var validDOI = regexp.MustCompile(`^10\.\d{4,9}/[^\s"<>]+$`)

func Build(bundle Bundle) (*Index, error) {
	if bundle.Retrieval.SemanticsSHA256 != SemanticsSHA256 {
		return nil, fmt.Errorf("unsupported query semantics version; update the reader before acquiring this dataset")
	}
	if bundle.Schema != BundleSchema || bundle.Retrieval.Schema != "openbibliographies.retrieval.v1" || bundle.Snapshot == "" || bundle.Source == "" || !validHash.MatchString(bundle.Retrieval.SemanticsSHA256) || bundle.Retrieval.AuthorVersion == "" {
		return nil, fmt.Errorf("unsupported bundle/retrieval schema or missing provenance")
	}
	if Digest([]byte(bundle.Catalog)) != bundle.Retrieval.CatalogSHA256 {
		return nil, fmt.Errorf("catalog hash mismatch")
	}
	var overlay AuthorOverlay
	if Digest([]byte(bundle.Retrieval.OverlayJSON)) != bundle.Retrieval.OverlaySHA256 {
		return nil, fmt.Errorf("author overlay hash mismatch")
	}
	if e := Decode([]byte(bundle.Retrieval.OverlayJSON), &overlay); e != nil {
		return nil, e
	}
	if overlay.Schema != "openbibliographies.author-aggregation.v1" || overlay.Snapshot != bundle.Snapshot || overlay.CatalogSHA256 != bundle.Retrieval.CatalogSHA256 || overlay.Catalog != "/data/catalog/"+overlay.CatalogSHA256[:16]+".json" || overlay.Version != bundle.Retrieval.AuthorVersion || !validHash.MatchString(overlay.MappingSHA256) || !strings.HasSuffix(overlay.Version, overlay.MappingSHA256) {
		return nil, fmt.Errorf("author overlay schema/snapshot/catalog/version mismatch")
	}
	bundle.Retrieval.Authors = overlay.Groups
	var c Catalog
	if e := Decode([]byte(bundle.Catalog), &c); e != nil {
		return nil, e
	}
	if c.Schema != "openbibliographies.preview.v4" || !c.Preview || c.Generated == "" || c.ClassificationVersion == "" || c.Coverage["shown"] != len(c.Works) || len(c.Works) == 0 {
		return nil, fmt.Errorf("invalid catalog schema/count")
	}
	x := &Index{Bundle: bundle, Catalog: c, Terms: map[string]Term{}, Authors: map[string]map[int]bool{}, Names: map[string]map[int]bool{}, TermNames: map[string][]string{}, Periods: map[string][]Span{}}
	for _, d := range Dimensions {
		ts, ok := c.Taxonomy[d]
		if !ok || ts == nil {
			return nil, fmt.Errorf("missing taxonomy %s", d)
		}
		for _, t := range ts {
			if t.Parents == nil {
				return nil, fmt.Errorf("missing term parents")
			}
			key := d + ":" + t.ID
			if _, ok := x.Terms[key]; ok || t.ID == "" {
				return nil, fmt.Errorf("duplicate/empty term")
			}
			x.Terms[key] = t
		}
	}
	if len(c.Taxonomy) != len(Dimensions) {
		return nil, fmt.Errorf("unknown taxonomy dimension")
	}
	memo := map[string]map[string]bool{}
	active := map[string]bool{}
	var up func(string) (map[string]bool, error)
	up = func(key string) (map[string]bool, error) {
		if active[key] {
			return nil, fmt.Errorf("taxonomy cycle")
		}
		if known := memo[key]; known != nil {
			return known, nil
		}
		t, ok := x.Terms[key]
		if !ok {
			return nil, fmt.Errorf("unknown parent %s", key)
		}
		active[key] = true
		out := map[string]bool{key: true}
		d, _, _ := strings.Cut(key, ":")
		for _, p := range t.Parents {
			if !strings.Contains(p, ":") {
				p = d + ":" + p
			}
			a, e := up(p)
			if e != nil {
				return nil, e
			}
			for k := range a {
				out[k] = true
			}
		}
		for _, n := range append(slices.Clone(t.Navigation), t.Near...) {
			if _, ok := x.Terms[n]; !ok {
				return nil, fmt.Errorf("unknown taxonomy link %s", n)
			}
		}
		delete(active, key)
		memo[key] = out
		return out, nil
	}
	for key := range x.Terms {
		if _, e := up(key); e != nil {
			return nil, e
		}
	}
	ids := map[string]int{}
	members := map[Member]AuthorGroup{}
	groupKeys := map[string]bool{}
	for _, g := range bundle.Retrieval.Authors {
		if !validID.MatchString(g.Key) || groupKeys[g.Key] || len(g.Members) == 0 {
			return nil, fmt.Errorf("invalid reviewed author group")
		}
		groupKeys[g.Key] = true
		for _, m := range g.Members {
			if _, ok := members[m]; ok {
				return nil, fmt.Errorf("conflicting author member")
			}
			members[m] = g
		}
	}
	for i, w := range c.Works {
		if _, ok := ids[w.ID]; ok || !validID.MatchString(w.ID) || w.Title == "" || w.Creators == nil || !slices.Contains(Formats, w.Format) || len(w.Labels) != len(Dimensions) {
			return nil, fmt.Errorf("invalid work %s", w.ID)
		}
		ids[w.ID] = i
		keys := map[string]bool{"format:" + w.Format: true}
		for _, t := range w.WorkType {
			if !slices.Contains(WorkTypes, t) {
				return nil, fmt.Errorf("unknown work type")
			}
			keys["workType:"+t] = true
		}
		for _, d := range Dimensions {
			labels, ok := w.Labels[d]
			if !ok || labels == nil {
				return nil, fmt.Errorf("missing labels")
			}
			for _, id := range labels {
				t, ok := x.Terms[d+":"+id]
				if !ok || t.Grouping {
					return nil, fmt.Errorf("invalid label %s:%s", d, id)
				}
				for k := range memo[d+":"+id] {
					keys[k] = true
				}
			}
		}
		if len(w.Labels["region"]) == 0 {
			keys["region:china-all"] = true
		}
		x.Keys = append(x.Keys, keys)
		for _, isbn := range w.ISBN {
			sum := 0
			if len(isbn) != 13 || (!strings.HasPrefix(isbn, "978") && !strings.HasPrefix(isbn, "979")) {
				return nil, fmt.Errorf("bad ISBN")
			}
			for j, r := range isbn {
				if r < '0' || r > '9' {
					return nil, fmt.Errorf("bad ISBN")
				}
				sum += int(r-'0') * (1 + 2*(j%2))
			}
			if sum%10 != 0 {
				return nil, fmt.Errorf("bad ISBN checksum")
			}
		}
		if w.DOI != "" && !validDOI.MatchString(w.DOI) {
			return nil, fmt.Errorf("bad DOI")
		}
		for _, r := range w.Related {
			if r < 0 || r >= len(c.Works) || r == i {
				return nil, fmt.Errorf("invalid related index")
			}
		}
		for _, cr := range w.Creators {
			g, ok := members[Member{w.ID, cr.Name, cr.Role}]
			key := authorSlug(cr.Name)
			if cr.Romanized != "" {
				key = authorSlug(cr.Romanized)
			}
			names := []string{cr.Name}
			if cr.Romanized != "" {
				names = appendUnique(names, cr.Romanized)
			}
			if ok {
				key = g.Key
				names = append(names, g.Name)
				for _, m := range g.Members {
					names = append(names, m.Name)
				}
			}
			addID(x.Authors, key, i)
			for _, name := range names {
				addID(x.Names, literal(name), i)
			}
		}
		x.Titles = append(x.Titles, literal(w.Title))
		variants := []string{}
		for _, title := range workTitles(w) {
			variants = appendUnique(variants, literal(title))
		}
		x.TitleVariants = append(x.TitleVariants, variants)
		x.TitlePeriods = append(x.TitlePeriods, intervals(w.Title))
	}
	for m := range members {
		i, ok := ids[m.Work]
		if !ok {
			return nil, fmt.Errorf("stale author member %s", m.Work)
		}
		count := 0
		for _, cr := range c.Works[i].Creators {
			if cr.Name == m.Name && cr.Role == m.Role {
				count++
			}
		}
		if count != 1 {
			return nil, fmt.Errorf("stale author signature")
		}
	}
	if len(bundle.Retrieval.TitleOrder) != len(c.Works) {
		return nil, fmt.Errorf("invalid title order length")
	}
	x.TitleRanks = make([]int, len(c.Works))
	seen := map[int]bool{}
	for rank, i := range bundle.Retrieval.TitleOrder {
		if i < 0 || i >= len(c.Works) || seen[i] {
			return nil, fmt.Errorf("invalid title order")
		}
		seen[i] = true
		x.TitleRanks[i] = rank
	}
	forms := map[string][]string{}
	for _, d := range Dimensions {
		for _, t := range c.Taxonomy[d] {
			key := d + ":" + t.ID
			canonical := []string{t.En, t.Zh, t.Zht, t.ID}
			bounds := []Span{}
			for _, n := range canonical {
				bounds = append(bounds, intervals(n)...)
			}
			if d == "period" {
				x.Periods[key] = bounds
			}
			names := append(slices.Clone(canonical), t.Aliases...)
			if d == "period" {
				for _, n := range []string{t.Zh, t.Zht} {
					if isHan(n) && len([]rune(n)) <= 3 {
						for _, s := range []string{"代", "朝", "时期", "時期"} {
							names = append(names, n+s)
						}
					}
				}
			}
			for _, n := range names {
				v := text(n)
				if v == "" {
					continue
				}
				skip := false
				if d == "period" {
					if span, ok := interval(v); ok {
						skip = true
						for _, b := range bounds {
							if b.End-b.Start <= span.End-span.Start {
								skip = false
							}
						}
					}
				}
				if !skip {
					forms[v] = appendUnique(forms[v], key)
				}
			}
			for _, n := range append(append([]string{t.En, t.Zh, t.Zht, t.ID}, t.Aliases...), t.En) {
				n = strings.TrimSpace(strings.ReplaceAll(lower(norm.NFKC.String(n)), "-", " "))
				x.TermNames[n] = appendUnique(x.TermNames[n], key)
			}
		}
	}
	for v, keys := range forms {
		x.Phrases = append(x.Phrases, Phrase{v, keys})
	}
	sortPhrases(x.Phrases)
	return x, nil
}
func appendUnique(a []string, v string) []string {
	if !slices.Contains(a, v) {
		a = append(a, v)
	}
	return a
}
func addID(m map[string]map[int]bool, key string, i int) {
	if key == "" {
		return
	}
	if m[key] == nil {
		m[key] = map[int]bool{}
	}
	m[key][i] = true
}
func lower(s string) string { return cases.Lower(language.Und).String(s) }
func fold(s string, all bool) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if (all && unicode.Is(unicode.M, r)) || (!all && r >= 0x300 && r <= 0x36f) {
			continue
		}
		b.WriteRune(r)
	}
	return lower(norm.NFKC.String(b.String()))
}
func literal(s string) string {
	var b strings.Builder
	space := true
	for _, r := range fold(s, true) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}
func authorSlug(s string) string {
	var b strings.Builder
	space := true
	for _, r := range norm.NFKD.String(lower(norm.NFKC.String(s))) {
		if r >= 0x300 && r <= 0x36f {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte('-')
			space = true
		}
	}
	v := strings.Trim(b.String(), "-")
	if v == "" {
		return "anonymous"
	}
	return v
}
func isHan(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.Is(unicode.Han, r) {
			return false
		}
	}
	return true
}
func sortPhrases(p []Phrase) {
	slices.SortStableFunc(p, func(a, b Phrase) int {
		if len([]rune(a.Value)) != len([]rune(b.Value)) {
			return len([]rune(b.Value)) - len([]rune(a.Value))
		}
		return strings.Compare(a.Value, b.Value)
	})
}
