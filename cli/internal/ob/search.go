package ob

import (
	"math"
	"math/bits"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

var stopwords = wordSet("a an and are as at by for from in into is it its of on or the to with 的 与 和 及", false)
var grammar = wordSet("how what which who whose where when why do does did can could would should will shall have has had be been being am was were this that these those i we you they he she it our your their its some any please find show me books book about across between among through", true)

func wordSet(s string, stemming bool) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(s) {
		if stemming {
			w = stem(w)
		}
		m[w] = true
	}
	return m
}

var suffixes = [][2]string{{"ational", "ate"}, {"ations", ""}, {"ation", ""}, {"ities", ""}, {"ity", ""}, {"ings", ""}, {"ing", ""}, {"ies", "y"}, {"ers", ""}, {"er", ""}, {"ed", ""}, {"es", ""}, {"s", ""}, {"al", ""}}
var digit = regexp.MustCompile(`[0-9]`)
var esSuffix = regexp.MustCompile(`(?:ch|sh|x|ss|zz)es$`)
var titleSeparator = regexp.MustCompile(`[:：]`)

func stem(w string) string {
	if len([]rune(w)) < 4 || digit.MatchString(w) {
		return w
	}
	for _, s := range suffixes {
		if strings.HasSuffix(w, s[0]) && len([]rune(w))-len(s[0]) >= 3 {
			if s[0] == "s" && (strings.HasSuffix(w, "ss") || strings.HasSuffix(w, "us") || strings.HasSuffix(w, "is")) {
				return w
			}
			if s[0] == "es" && !esSuffix.MatchString(w) {
				return w[:len(w)-1]
			}
			return strings.TrimSuffix(w, s[0]) + s[1]
		}
	}
	return w
}
func Tokens(s string, query bool) []string {
	out := []string{}
	r := []rune(fold(s, false))
	for i := 0; i < len(r); {
		if !unicode.IsLetter(r[i]) && !unicode.IsNumber(r[i]) {
			i++
			continue
		}
		j := i + 1
		han := unicode.Is(unicode.Han, r[i])
		for j < len(r) && ((han && unicode.Is(unicode.Han, r[j])) || (!han && (unicode.IsLetter(r[j]) || unicode.IsNumber(r[j])))) {
			j++
		}
		run := string(r[i:j])
		if hasHan(run) {
			if j-i == 1 || !query {
				for _, c := range r[i:j] {
					out = append(out, string(c))
				}
			}
			for k := i; k+1 < j; k++ {
				out = append(out, string(r[k:k+2]))
			}
		} else if !stopwords[run] {
			out = append(out, stem(run))
		}
		i = j
	}
	return out
}
func queryTokens(q string) []string {
	out := []string{}
	for _, w := range Tokens(q, true) {
		if !grammar[w] {
			out = appendUnique(out, w)
		}
	}
	return out
}
func hasHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// The bounded dataset cannot exceed uint32 document/token counts. Keeping these
// triples at 12 bytes halves posting storage on the supported 64-bit targets.
type Posting struct{ Doc, Field, TF uint32 }
type SearchIndex struct {
	Postings   map[string][]Posting
	DF         map[string]int
	Vocabulary []string
	Lengths    [4][]float64
	Average    [4]float64
	Titles     [][2]string
	Canonical  map[string]map[string]bool
}

func (x *Index) buildSearch() {
	s := &SearchIndex{Postings: map[string][]Posting{}, DF: map[string]int{}, Canonical: map[string]map[string]bool{}}
	for f := 0; f < 4; f++ {
		s.Lengths[f] = make([]float64, len(x.Catalog.Works))
	}
	// Every name/alias is separated by " ; " in the reference terms field.
	// That separator is a token boundary, so immutable term tokens can be reused
	// without changing field TF, length, document frequency or posting order.
	termTokens := map[string][]string{}
	for key, t := range x.Terms {
		names := append([]string{t.En, t.Zh, t.Zht}, t.Aliases...)
		termTokens[key] = Tokens(strings.Join(names, " ; "), false)
	}
	for i, w := range x.Catalog.Works {
		seen := map[string]bool{}
		terms := []string{}
		for _, d := range Dimensions {
			for _, id := range w.Labels[d] {
				terms = append(terms, termTokens[d+":"+id]...)
			}
		}
		year := ""
		if w.Year != nil {
			year = strconv.Itoa(*w.Year)
		}
		docs := []string{strings.Join(workTitles(w), " ; "), "", "", w.Venue + " " + year}
		for f, v := range docs {
			counts := map[string]int{}
			tokens := terms
			if f != 2 {
				tokens = Tokens(v, false)
			}
			for _, tok := range tokens {
				counts[tok]++
				s.Lengths[f][i]++
			}
			for tok, n := range counts {
				s.Postings[tok] = append(s.Postings[tok], Posting{uint32(i), uint32(f), uint32(n)})
				if !seen[tok] {
					s.DF[tok]++
					seen[tok] = true
				}
			}
		}
		main := titleSeparator.Split(w.Title, 2)[0]
		s.Titles = append(s.Titles, [2]string{strings.Join(Tokens(w.Title, true), " "), strings.Join(Tokens(main, true), " ")})
	}
	// Creator/venue postings influence IDF and lengths even though content admission excludes them.
	for i, w := range x.Catalog.Works {
		names := []string{}
		for _, cr := range w.Creators {
			names = appendUnique(names, cr.Name)
			if cr.Romanized != "" {
				names = appendUnique(names, cr.Romanized)
			}
			for _, g := range x.Bundle.Retrieval.Authors {
				matched := false
				for _, m := range g.Members {
					if m.Work == w.ID && m.Name == cr.Name && m.Role == cr.Role {
						matched = true
						break
					}
				}
				if matched {
					names = appendUnique(names, g.Name)
					for _, m := range g.Members {
						names = appendUnique(names, m.Name)
					}
				}
			}
		}
		counts := map[string]int{}
		ts := Tokens(strings.Join(names, " ; "), false)
		s.Lengths[1][i] = float64(len(ts))
		for _, t := range ts {
			counts[t]++
		}
		for tok, n := range counts {
			already := false
			for _, p := range s.Postings[tok] {
				if int(p.Doc) == i {
					already = true
					break
				}
			}
			if !already {
				s.DF[tok]++
			}
			s.Postings[tok] = append(s.Postings[tok], Posting{uint32(i), 1, uint32(n)})
		}
	}
	for tok := range s.Postings {
		s.Vocabulary = append(s.Vocabulary, tok)
	}
	sort.Strings(s.Vocabulary)
	for f := 0; f < 4; f++ {
		sum := 0.
		for _, n := range s.Lengths[f] {
			sum += n
		}
		s.Average[f] = math.Max(1, sum/float64(max(1, len(x.Catalog.Works))))
	}
	for _, d := range Dimensions {
		for _, t := range x.Catalog.Taxonomy[d] {
			key := d + ":" + t.ID
			for _, tok := range Tokens(strings.Join([]string{t.En, t.Zh, t.Zht, t.ID}, " "), false) {
				if s.Canonical[tok] == nil {
					s.Canonical[tok] = map[string]bool{}
				}
				s.Canonical[tok][key] = true
			}
		}
	}
	x.Search = s
}

// lexical returns nil when only grammar remains, and an empty map for unknown content.
func (x *Index) lexical(q string) (map[int]float64, []string) {
	words := queryTokens(q)
	if len(words) == 0 {
		return nil, nil
	}
	if x.Search == nil {
		x.buildSearch()
	}
	s := x.Search
	scores := map[int]float64{}
	matched := map[int]uint32{}
	concepts := []string{}
	last := ""
	if strings.TrimRightFunc(q, unicode.IsSpace) == q && !hasHan(words[len(words)-1]) {
		last = words[len(words)-1]
	}
	idf := func(t string) float64 {
		df := float64(s.DF[t])
		return math.Log(1 + (float64(len(x.Catalog.Works))-df+.5)/(df+.5))
	}
	for wi, word := range words {
		variants := []string{word}
		factors := map[string]float64{word: 1}
		if word == stem("computing") {
			for _, v := range []string{stem("typewriter"), stem("keyboard")} {
				variants = append(variants, v)
				factors[v] = .5
			}
		}
		if word == last && len([]rune(word)) >= 3 {
			at := sort.SearchStrings(s.Vocabulary, word)
			for n := 0; at < len(s.Vocabulary) && strings.HasPrefix(s.Vocabulary[at], word) && n < 200; at, n = at+1, n+1 {
				v := s.Vocabulary[at]
				if factors[v] == 0 {
					variants = append(variants, v)
					factors[v] = .5
				}
			}
		}
		cap := math.Inf(1)
		if s.DF[word] > 0 {
			cap = idf(word)
		}
		best := map[int]float64{}
		for _, tok := range variants {
			for key := range s.Canonical[tok] {
				concepts = appendUnique(concepts, key)
			}
			for _, p := range s.Postings[tok] {
				if p.Field != 0 && p.Field != 2 {
					continue
				}
				if p.Field == 2 {
					yes := false
					for key := range s.Canonical[tok] {
						if x.Keys[p.Doc][key] {
							yes = true
							break
						}
					}
					if !yes {
						continue
					}
				}
				tf := float64(p.TF)
				n := tf * 2.2 / (tf + 1.2*(.25+.75*s.Lengths[p.Field][p.Doc]/s.Average[p.Field]))
				weight := 1.
				if p.Field == 0 {
					weight = 3
				}
				best[int(p.Doc)] += factors[tok] * math.Min(idf(tok), cap) * weight * n
			}
		}
		for doc, score := range best {
			scores[doc] += score
			matched[doc] |= 1 << min(wi, 30)
		}
	}
	phrase := strings.Join(words, " ")
	for doc, score := range scores {
		share := float64(bits.OnesCount32(matched[doc])) / float64(len(words))
		title := s.Titles[doc]
		boost := 1.
		if title[0] == phrase || title[1] == phrase {
			boost = 4
		} else if len(words) > 1 && strings.Contains(" "+title[0]+" ", " "+phrase+" ") {
			boost = 2.5
		}
		for _, alias := range workTitles(x.Catalog.Works[doc])[1:] {
			full := strings.Join(Tokens(alias, true), " ")
			main := strings.Join(Tokens(titleSeparator.Split(alias, 2)[0], true), " ")
			if full == phrase || main == phrase {
				boost = 4
			} else if boost < 2.5 && len(words) > 1 && strings.Contains(" "+full+" ", " "+phrase+" ") {
				boost = 2.5
			}
		}
		scores[doc] = score * share * share * boost
	}
	return scores, concepts
}
func put(m map[int]float64, i int, q float64) { m[i] = math.Max(m[i], q) }
func occurrences(input, phrase string) []int {
	out := []int{}
	r, p := []rune(input), []rune(phrase)
	for i := 0; i+len(p) <= len(r); i++ {
		if !slices.Equal(r[i:i+len(p)], p) {
			continue
		}
		var left, right rune
		if i > 0 {
			left = r[i-1]
		}
		if i+len(p) < len(r) {
			right = r[i+len(p)]
		}
		latin := func(c rune) bool { return (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') }
		alnum := func(c rune) bool { return unicode.IsLetter(c) || unicode.IsNumber(c) }
		if (latin(p[0]) && alnum(left)) || (latin(p[len(p)-1]) && alnum(right)) {
			continue
		}
		if len(p) == 1 && hasHan(phrase) && (unicode.Is(unicode.Han, left) || unicode.Is(unicode.Han, right)) {
			continue
		}
		out = append(out, i)
	}
	return out
}
func (x *Index) names(v string, short bool) map[int]float64 {
	m := map[int]float64{}
	for name, docs := range x.Names {
		if !strings.Contains(name, v) {
			continue
		}
		whole := len(occurrences(name, v)) > 0
		if short && !hasHan(v) && !whole {
			continue
		}
		q := 3.
		if name == v {
			q = 5
		} else if whole {
			q = 3.5
		}
		for i := range docs {
			put(m, i, q)
		}
	}
	return m
}
func (x *Index) literalMatches(v string, m map[int]float64) map[int]float64 {
	if m == nil {
		m = map[int]float64{}
	}
	for i, titles := range x.TitleVariants {
		for _, title := range titles {
			if len(occurrences(title, v)) > 0 {
				put(m, i, 2)
			}
		}
	}
	return m
}
