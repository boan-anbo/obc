// Package searchnorm is the text normalisation shared with the site. The rules are in
// docs/search-normalisation.md on the site, the tables in data/normalise.v1.json. Both
// implementations must pass data/vectors.v1.json in full.
package searchnorm

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

//go:embed data/normalise.v1.json
var tableJSON []byte

// TableSHA256 is the digest of the embedded table. A new table is new search semantics.
var TableSHA256 = func() string { h := sha256.Sum256(tableJSON); return hex.EncodeToString(h[:]) }()

var (
	foldMap   = map[rune]rune{}
	readings  = map[rune][]string{}
	syllables = map[string]bool{}
	longest   int
	lowerer   = cases.Lower(language.Und)
	pinyinish = regexp.MustCompile(`^[a-z]+[1-5]?$`)
)

func init() {
	var t struct {
		Fold struct{ From, To string } `json:"fold"`
	}
	if e := json.Unmarshal(tableJSON, &t); e != nil {
		panic(e)
	}
	from, to := []rune(t.Fold.From), []rune(t.Fold.To)
	if len(from) != len(to) {
		panic("searchnorm: fold strings differ in length")
	}
	for i, c := range from {
		foldMap[c] = to[i]
	}
	if e := loadPinyin(); e != nil {
		panic(e)
	}
}

// loadPinyin keeps the file's key order, which is the order of a character's readings.
func loadPinyin() error {
	var outer map[string]json.RawMessage
	if e := json.Unmarshal(tableJSON, &outer); e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(outer["pinyin"]))
	if _, e := d.Token(); e != nil {
		return e
	}
	for d.More() {
		key, e := d.Token()
		if e != nil {
			return e
		}
		var chars string
		if e = d.Decode(&chars); e != nil {
			return e
		}
		syllable, ok := key.(string)
		if !ok {
			return fmt.Errorf("searchnorm: pinyin key %v", key)
		}
		syllables[syllable] = true
		longest = max(longest, len(syllable))
		for _, c := range chars {
			readings[c] = append(readings[c], syllable)
		}
	}
	return nil
}

// Normalise applies the five rules in order. Display always keeps the original text.
func Normalise(s string) string {
	var b strings.Builder
	for _, r := range norm.NFKD.String(s) {
		if r < 0x300 || r > 0x36f {
			b.WriteRune(r)
		}
	}
	base := lowerer.String(norm.NFKC.String(b.String()))
	b.Reset()
	for _, r := range base {
		if to, ok := foldMap[r]; ok {
			r = to
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Literal is Normalise with each run of other characters reduced to one space.
func Literal(s string) string {
	var b strings.Builder
	space := false
	for _, r := range Normalise(s) {
		if unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsNumber(r) {
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = false
			b.WriteRune(r)
		} else {
			space = true
		}
	}
	return b.String()
}

// IsHan says whether r is of Han script.
func IsHan(r rune) bool { return unicode.Is(unicode.Han, r) }

func isKana(r rune) bool { return unicode.In(r, unicode.Hiragana, unicode.Katakana) }

func hanRuns(s string) [][]rune {
	out := [][]rune{}
	var run []rune
	for _, r := range Normalise(s) {
		if IsHan(r) {
			run = append(run, r)
		} else if run != nil {
			out, run = append(out, run), nil
		}
	}
	if run != nil {
		out = append(out, run)
	}
	return out
}

// HanTokens gives the Han tokens of each run. An index keeps every character and pair;
// a query keeps a lone character, or the pairs of a longer run.
func HanTokens(s string, query bool) []string {
	out := []string{}
	for _, run := range hanRuns(s) {
		if !query || len(run) == 1 {
			for _, c := range run {
				out = append(out, string(c))
			}
		}
		for i := 0; i+1 < len(run); i++ {
			out = append(out, string(run[i:i+2]))
		}
	}
	return out
}

// Romanise gives each character's readings, runs apart: "gao hua | zeng/ceng".
func Romanise(s string) string {
	runs := []string{}
	for _, run := range hanRuns(s) {
		tokens := make([]string, 0, len(run))
		for _, c := range run {
			if list := readings[c]; len(list) > 0 {
				tokens = append(tokens, strings.Join(list, "/"))
			} else {
				tokens = append(tokens, "?")
			}
		}
		runs = append(runs, strings.Join(tokens, " "))
	}
	return strings.Join(runs, " | ")
}

// RomaniseRecord gives [title, creators] in pinyin. Japanese records have none: their
// kanji are not read in pinyin.
func RomaniseRecord(title, language string, creators []string) [2]string {
	if language == "ja" || strings.ContainsFunc(title, isKana) {
		return [2]string{}
	}
	names := []string{}
	for _, c := range creators {
		if r := Romanise(c); r != "" {
			names = append(names, r)
		}
	}
	return [2]string{Romanise(title), strings.Join(names, " | ")}
}

func segmentations(word string) [][]string {
	found := [][]string{}
	var walk func(int, []string)
	walk = func(at int, path []string) {
		if len(found) >= 4 {
			return
		}
		if at == len(word) {
			found = append(found, path)
			return
		}
		for end := min(len(word), at+longest); end > at; end-- {
			part := word[at:end]
			// A bare vowel is a syllable only as a whole word: gao is not ga-o.
			if syllables[part] && (len(part) > 1 || part == word) {
				walk(end, append(append([]string{}, path...), part))
			}
		}
	}
	walk(0, []string{})
	return found
}

// PinyinQueries gives the syllable sequences a Latin query may spell, or nothing unless
// all of it is pinyin.
func PinyinQueries(query string) [][]string {
	words := strings.Fields(Literal(query))
	if len(words) == 0 {
		return [][]string{}
	}
	out := [][]string{{}}
	for _, w := range words {
		if !pinyinish.MatchString(w) {
			return [][]string{}
		}
	}
	for _, w := range words {
		bare := strings.ReplaceAll(strings.TrimRight(w, "12345"), "v", "u")
		parts := segmentations(bare)
		if len(parts) == 0 {
			return [][]string{}
		}
		next := [][]string{}
		for _, head := range out {
			for _, part := range parts {
				next = append(next, append(append([]string{}, head...), part...))
			}
		}
		out = next[:min(len(next), 8)]
	}
	kept := [][]string{}
	for _, s := range out {
		if len(s) >= 2 {
			kept = append(kept, s)
		}
	}
	return kept
}

// NameReadings gives the possible romanised names of a Han name of 2 to 4 characters.
func NameReadings(query string) []string {
	name := []rune(Literal(query))
	if len(name) < 2 || len(name) > 4 {
		return []string{}
	}
	out := []string{""}
	for _, c := range name {
		list := readings[c]
		if !IsHan(c) || len(list) == 0 {
			return []string{}
		}
		next := []string{}
		for _, head := range out {
			for _, r := range list {
				next = append(next, strings.TrimSpace(head+" "+r))
			}
		}
		out = next[:min(len(next), 16)]
	}
	return out
}

func sequences(text string) [][][]string {
	out := [][][]string{}
	for _, run := range strings.Split(text, " | ") {
		if run == "" {
			continue
		}
		tokens := [][]string{}
		for _, token := range strings.Split(run, " ") {
			tokens = append(tokens, strings.Split(token, "/"))
		}
		out = append(out, tokens)
	}
	return out
}

func matchAt(run [][]string, from int, wanted []string) bool {
	if from+len(wanted) > len(run) {
		return false
	}
	for i, s := range wanted {
		if !contains(run[from+i], s) {
			return false
		}
	}
	return true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// TitleMatch says whether a sequence occurs unbroken in one run of a romanised title.
func TitleMatch(title string, wanted []string) bool {
	for _, run := range sequences(title) {
		for i := range run {
			if matchAt(run, i, wanted) {
				return true
			}
		}
	}
	return false
}

// NameMatch says whether a sequence covers one whole run, forwards or reversed.
func NameMatch(names string, wanted []string) bool {
	reversed := make([]string, len(wanted))
	for i, s := range wanted {
		reversed[len(wanted)-1-i] = s
	}
	for _, run := range sequences(names) {
		if len(run) == len(wanted) && (matchAt(run, 0, wanted) || matchAt(run, 0, reversed)) {
			return true
		}
	}
	return false
}
