package ob

import (
	"math"
	"strings"
	"unicode"

	"openbibliographies/ob/internal/searchnorm"
)

func romanisedRecord(w Work) [2]string {
	names := make([]string, 0, len(w.Creators))
	for _, c := range w.Creators {
		names = append(names, c.Name)
	}
	return searchnorm.RomaniseRecord(w.Title, w.Language, names)
}

// creatorNamed finds works whose creator is exactly this name, in either word order.
func (x *Index) creatorNamed(value string) map[int]bool {
	out := map[int]bool{}
	words := strings.Fields(value)
	for i, j := 0, len(words)-1; i < j; i, j = i+1, j-1 {
		words[i], words[j] = words[j], words[i]
	}
	for _, name := range []string{value, strings.Join(words, " ")} {
		for doc := range x.Names[name] {
			out[doc] = true
		}
	}
	return out
}

// romanisedHits maps a work to how far below the top it sits: a name match .5, a title 1.
func (x *Index) romanisedHits(query string) map[int]float64 {
	hits := map[int]float64{}
	for _, wanted := range searchnorm.PinyinQueries(query) {
		for doc := range x.creatorNamed(strings.Join(wanted, " ")) {
			hits[doc] = .5
		}
		for doc, record := range x.Romanised {
			if searchnorm.NameMatch(record[1], wanted) {
				hits[doc] = .5
			} else if _, seen := hits[doc]; !seen && searchnorm.TitleMatch(record[0], wanted) {
				hits[doc] = 1
			}
		}
	}
	for _, value := range searchnorm.NameReadings(query) {
		for doc := range x.creatorNamed(value) {
			hits[doc] = .5
		}
	}
	return hits
}

// Identity hits sit above the top of the results, titles first. A work that is both is a title hit.
const (
	titleHit   = -1.
	creatorHit = -.5
)

// identityHits are works whose title, main title or creator is exactly the query, ranked
// above the top. A name that is also a taxonomy term, or a title whose words read as
// clauses, must still find them: a person's own books come before books about the person.
func (x *Index) identityHits(query string) map[int]float64 {
	hits := map[int]float64{}
	phrase := literal(query)
	if phrase == "" {
		return hits
	}
	for doc := range x.Names[phrase] {
		hits[doc] = creatorHit
	}
	for doc := range x.Titles {
		if x.exactTitle(doc, phrase) || literal(titleSeparator.Split(x.Catalog.Works[doc].Title, 2)[0]) == phrase {
			hits[doc] = titleHit
		}
	}
	return hits
}

// A lone word the catalogue already knows (china, shanghai) stays English first.
func (x *Index) knownWord(query string) bool {
	words := strings.Fields(searchnorm.Normalise(query))
	if len(words) != 1 || strings.ContainsFunc(words[0], func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
		return false
	}
	if x.Search == nil {
		x.buildSearch()
	}
	for _, t := range Tokens(words[0], true) {
		if x.Search.DF[t] > 0 {
			return true
		}
	}
	return false
}

// withRomanised adds what romanisation finds without changing the literal results. Spelled
// in full, a sequence beats partial word matches, unless the catalogue holds those words
// literally as a name or phrase: then the literal record keeps first place.
func (x *Index) withRomanised(query string, main map[int]float64) map[int]float64 {
	hits := x.romanisedHits(query)
	identity := x.identityHits(query)
	for doc, drop := range identity {
		hits[doc] = drop
	}
	if len(hits) == 0 {
		return main
	}
	known := x.knownWord(query)
	phrase := literal(query)
	recorded := len(x.Names[phrase]) > 0
	for _, title := range x.Titles {
		if recorded {
			break
		}
		recorded = strings.Contains(" "+title+" ", " "+phrase+" ")
	}
	out := make(map[int]float64, len(main)+len(hits))
	top := 1.5
	for doc, score := range main {
		out[doc] = score
		top = math.Max(top, score)
	}
	if !recorded {
		top += 2
	}
	for doc, drop := range hits {
		score := top - drop
		if _, exact := identity[doc]; known && !exact {
			score = .5
		}
		out[doc] = math.Max(out[doc], score)
	}
	return out
}
