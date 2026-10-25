package ob

// These are accepted public title/name variants, not inferred translations or
// concept expansions. Works without variants retain their existing search input.
func workTitles(w Work) []string {
	out := []string{w.Title}
	if w.TitleRomanized != "" {
		out = appendUnique(out, w.TitleRomanized)
	}
	if w.TitleEnglish != nil && w.TitleEnglish.Value != "" {
		out = appendUnique(out, w.TitleEnglish.Value)
	}
	return out
}
func (x *Index) exactTitle(i int, q string) bool {
	for _, title := range x.TitleVariants[i] {
		if title == q {
			return true
		}
	}
	return false
}
