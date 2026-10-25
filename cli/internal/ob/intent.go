package ob

import (
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

type Span struct{ Start, End int }

var rangeRE = regexp.MustCompile(`(\d{3,4})\s*[-–—]\s*(\d{3,4})`)

func text(s string) string { return literal(rangeRE.ReplaceAllString(s, "${1} to ${2}")) }

var decadeRE = regexp.MustCompile(`^(\d{3}0)(?:s|年代)$`)
var centuryRE = regexp.MustCompile(`^(\d{1,2})(?:(?:st|nd|rd|th)? century|世纪|世紀)$`)
var spanRE = regexp.MustCompile(`^(\d{3,4}) to (\d{3,4})$`)
var yearRE = regexp.MustCompile(`^\d{4}$`)

func interval(v string) (Span, bool) {
	number := func(s string) int { n, _ := strconv.Atoi(s); return n }
	if m := decadeRE.FindStringSubmatch(v); m != nil {
		n := number(m[1])
		return Span{n, n + 9}, true
	}
	if m := centuryRE.FindStringSubmatch(v); m != nil {
		n := number(m[1])
		return Span{(n-1)*100 + 1, n * 100}, true
	}
	if m := spanRE.FindStringSubmatch(v); m != nil {
		a, b := number(m[1]), number(m[2])
		return Span{a, b}, a <= b
	}
	n := number(v)
	return Span{n, n}, yearRE.MatchString(v)
}

const dateExpression = `\d{3,4} to \d{3,4}|\d{3}0(?:s|年代)|\d{1,2}(?:(?:st|nd|rd|th)? century|世纪|世紀)|\d{4}`

var datesRE = regexp.MustCompile(dateExpression)

func dateBounded(s string, at, end int) bool {
	boundary := func(s string, last bool) bool {
		if s == "" {
			return true
		}
		r := []rune(s)
		c := r[0]
		if last {
			c = r[len(r)-1]
		}
		return (!unicode.IsLetter(c) && !unicode.IsNumber(c)) || unicode.Is(unicode.Han, c)
	}
	return boundary(s[:at], true) && boundary(s[end:], false)
}
func intervals(s string) []Span {
	s = text(s)
	out := []Span{}
	for _, m := range datesRE.FindAllStringIndex(s, -1) {
		if dateBounded(s, m[0], m[1]) {
			if b, ok := interval(s[m[0]:m[1]]); ok {
				out = append(out, b)
			}
		}
	}
	return out
}

type dateClause struct {
	At                       int
	Value                    string
	Bounds                   Span
	Valid, Publication, Bare bool
}

var publicationRE = regexp.MustCompile(`(?:\b(?:published(?: in)?|publication year)\s+|出版[于於]?\s*)(` + dateExpression + `)年?|(` + dateExpression + `)年?出版`)

func dateClauses(s string) []dateClause {
	out := []dateClause{}
	for _, m := range publicationRE.FindAllStringSubmatchIndex(s, -1) {
		if !dateBounded(s, m[0], m[1]) {
			continue
		}
		a, b := m[2], m[3]
		if a < 0 {
			a, b = m[4], m[5]
		}
		span, ok := interval(s[a:b])
		out = append(out, dateClause{len([]rune(s[:m[0]])), s[m[0]:m[1]], span, ok, true, false})
	}
	for _, m := range datesRE.FindAllStringIndex(s, -1) {
		if !dateBounded(s, m[0], m[1]) {
			continue
		}
		at := len([]rune(s[:m[0]]))
		taken := false
		for _, c := range out {
			if at >= c.At && at < c.At+len([]rune(c.Value)) {
				taken = true
			}
		}
		if taken {
			continue
		}
		v := s[m[0]:m[1]]
		span, ok := interval(v)
		if ok {
			out = append(out, dateClause{at, v, span, true, false, yearRE.MatchString(v)})
		}
	}
	return out
}
func (x *Index) timeMatches(b Span, publication, bare bool) map[int]float64 {
	out := map[int]float64{}
	for i, w := range x.Catalog.Works {
		if (publication || bare) && w.Year != nil && *w.Year >= b.Start && *w.Year <= b.End {
			put(out, i, 4)
		}
		if publication {
			continue
		}
		for _, id := range w.Labels["period"] {
			for _, p := range x.Periods["period:"+id] {
				if p.Start >= b.Start && p.End <= b.End {
					put(out, i, 4)
				}
			}
		}
		for _, p := range x.TitlePeriods[i] {
			if p.Start <= b.End && p.End >= b.Start {
				put(out, i, 3)
			}
		}
	}
	return out
}

type Clause struct {
	Value, Kind string
	Matches     map[int]float64
	ConceptKeys []string
	GroupKey    string // canonical deduplication only; never a corroboration vote
}

func (x *Index) scores(query string) map[int]float64 {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	input := text(query)
	whole := map[int]float64{}
	if strings.Contains(input, " ") {
		whole = x.names(input, false)
	}
	if len(whole) > 0 {
		x.literalMatches(input, whole)
		for _, p := range x.Phrases {
			if p.Value == input {
				for i, keys := range x.Keys {
					for _, k := range p.Keys {
						if keys[k] {
							put(whole, i, 2)
						}
					}
				}
				break
			}
		}
		return whole
	}
	lex, _ := x.lexical(query)
	if lex == nil {
		exact := map[int]float64{}
		for i := range x.Titles {
			if x.exactTitle(i, literal(query)) {
				exact[i] = 6
			}
		}
		if len(exact) > 0 {
			return exact
		}
	}
	runes := []rune(input)
	used := make([]bool, len(runes))
	clauses := []Clause{}
	free := func(at, n int) bool {
		for _, b := range used[at : at+n] {
			if b {
				return false
			}
		}
		return true
	}
	take := func(at int, v, kind string, m map[int]float64) {
		for i := at; i < at+len([]rune(v)); i++ {
			used[i] = true
		}
		clauses = append(clauses, Clause{Value: v, Kind: kind, Matches: m})
	}
	for _, c := range dateClauses(input) {
		m := map[int]float64{}
		if c.Valid {
			m = x.timeMatches(c.Bounds, c.Publication, c.Bare)
		}
		kind := "time"
		if c.Publication {
			kind = "publication"
		}
		take(c.At, c.Value, kind, m)
	}
	creatorPhrases := []string{}
	for name := range x.Names {
		if strings.Contains(name, " ") {
			creatorPhrases = append(creatorPhrases, name)
		}
	}
	slices.SortFunc(creatorPhrases, func(a, b string) int {
		if len([]rune(a)) != len([]rune(b)) {
			return len([]rune(b)) - len([]rune(a))
		}
		return strings.Compare(a, b)
	})
	for _, name := range creatorPhrases {
		for _, at := range occurrences(input, name) {
			if free(at, len([]rune(name))) {
				take(at, name, "creator", x.names(name, false))
			}
		}
	}
	remaining := func() string {
		r := slices.Clone(runes)
		for i, b := range used {
			if b {
				r[i] = ' '
			}
		}
		return string(r)
	}
	if slices.ContainsFunc(clauses, func(c Clause) bool { return c.Kind == "creator" }) {
		s := remaining()
		for _, m := range regexp.MustCompile(`[^ ]+(?: [^ ]+)+`).FindAllStringIndex(s, -1) {
			v := strings.TrimSpace(s[m[0]:m[1]])
			at := len([]rune(s[:m[0]]))
			matches := x.literalMatches(v, nil)
			if len(matches) > 0 && free(at, len([]rune(v))) {
				take(at, v, "title", matches)
			}
		}
	}
	// Query-only Han components extend accepted phrases without changing labels.
	phrases := slices.Clone(x.Phrases)
	forms := map[string][]string{}
	for _, p := range phrases {
		forms[p.Value] = slices.Clone(p.Keys)
	}
	for _, d := range Dimensions {
		for _, t := range x.Catalog.Taxonomy[d] {
			for _, n := range []string{t.Zh, t.Zht} {
				for _, part := range strings.FieldsFunc(n, func(r rune) bool { return strings.ContainsRune("与與和及、", r) }) {
					v := text(part)
					if v != "" && v != text(n) {
						forms[v] = appendUnique(forms[v], d+":"+t.ID)
					}
				}
			}
		}
	}
	phrases = nil
	for v, k := range forms {
		phrases = append(phrases, Phrase{v, k})
	}
	sortPhrases(phrases)
	for progressed := true; progressed; {
		progressed = false
		for _, p := range phrases {
			n := len([]rune(p.Value))
			for _, at := range occurrences(input, p.Value) {
				if !free(at, n) {
					continue
				}
				end := at + n
				if hasHan(p.Value) && at > 0 && end < len(runes) && unicode.Is(unicode.Han, runes[at-1]) && unicode.Is(unicode.Han, runes[end]) && !used[at-1] && !used[end] {
					continue
				}
				matches := x.literalMatches(p.Value, nil)
				quality := 2.
				if slices.ContainsFunc(p.Keys, func(k string) bool { return strings.HasPrefix(k, "period:") }) {
					quality = 4
				}
				for i, keys := range x.Keys {
					for _, k := range p.Keys {
						if keys[k] {
							put(matches, i, quality)
						}
					}
				}
				take(at, p.Value, "term", matches)
				keys := slices.Clone(p.Keys)
				sortStrings(keys)
				clauses[len(clauses)-1].GroupKey = "term:" + strings.Join(keys, "|")
				progressed = true
			}
		}
	}
	standalone := len(clauses) == 0 && !strings.Contains(input, " ")
	s := remaining()
	for _, m := range regexp.MustCompile(`[\p{L}\p{N}]+`).FindAllStringIndex(s, -1) {
		v := s[m[0]:m[1]]
		at := len([]rune(s[:m[0]]))
		if !free(at, len([]rune(v))) {
			continue
		}
		word, concepts := x.lexical(v)
		creators := map[int]float64{}
		if standalone {
			creators = x.names(v, word == nil)
		}
		if word == nil && len([]rune(v)) > 2 {
			creators = map[int]float64{}
			for i := range x.Names[v] {
				creators[i] = 5
			}
		}
		if word == nil && len(creators) == 0 {
			continue
		}
		matches := map[int]float64{}
		for i, score := range word {
			matches[i] = 1 + score/(1+score)
		}
		for i, q := range creators {
			put(matches, i, q)
		}
		take(at, v, "words", matches)
		clauses[len(clauses)-1].ConceptKeys = concepts
	}
	eligible := make([]bool, len(x.Titles))
	for i := range eligible {
		eligible[i] = true
	}
	for _, c := range clauses {
		if c.Kind == "publication" {
			for i := range eligible {
				if _, ok := c.Matches[i]; !ok {
					eligible[i] = false
				}
			}
		}
	}
	groups := []Clause{}
	seen := map[string]bool{}
	for _, c := range clauses {
		if c.Kind == "publication" || len(c.Matches) == 0 {
			continue
		}
		key := c.GroupKey
		if key == "" {
			key = c.Kind + ":" + c.Value
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		groups = append(groups, c)
	}
	result := map[int]float64{}
	if len(groups) == 0 {
		if len(clauses) > 0 && !slices.ContainsFunc(clauses, func(c Clause) bool { return c.Kind != "publication" }) {
			for i, yes := range eligible {
				if yes {
					result[i] = 0
				}
			}
		}
		return result
	}
	weights := make([]float64, len(groups))
	total := 0.
	votes := map[string]int{}
	type evidence struct {
		Count           int
		Weight, Quality float64
	}
	ev := map[int]evidence{}
	for n, g := range groups {
		w := math.Log1p(float64(len(x.Titles)) / float64(len(g.Matches)))
		weights[n] = w
		total += w
		keys := map[string]bool{}
		for _, key := range g.ConceptKeys {
			keys[key] = true
		}
		for key := range keys {
			votes[key]++
		}
		for i, q := range g.Matches {
			if !eligible[i] {
				continue
			}
			e := ev[i]
			e.Count++
			e.Weight += w
			e.Quality += w * q
			ev[i] = e
		}
	}
	soft, _ := x.lexical(query)
	identifying := slices.ContainsFunc(groups, func(c Clause) bool { return c.Kind == "creator" }) && slices.ContainsFunc(groups, func(c Clause) bool { return c.Kind == "title" })
	for i, e := range ev {
		coverage := e.Weight / total
		term := false
		for _, g := range groups {
			if g.Kind == "term" {
				if _, ok := g.Matches[i]; ok {
					term = true
				}
			}
		}
		if e.Count < min(2, len(groups)) && (identifying || coverage <= .5 || !term) {
			continue
		}
		bm := soft[i]
		identity := 0.
		if x.exactTitle(i, literal(query)) {
			identity = math.Min(12, total)
		}
		corroborated := 0
		for k, v := range votes {
			if v >= 2 && x.Keys[i][k] {
				corroborated += v - 1
			}
		}
		score := 100*coverage*coverage + 20*math.Log1p(float64(corroborated)) + e.Quality/e.Weight + 4*bm/(1+bm) + identity
		result[i] = math.Floor(score*2) / 2
	}
	return result
}
func sortStrings(a []string) { slices.Sort(a) }
