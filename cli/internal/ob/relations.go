package ob

import (
	"bytes"
	"fmt"
)

type Relation struct {
	Type string `json:"type"`
	Work string `json:"work"`
}
type Relations []Relation

func (r *Relations) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		return fmt.Errorf("relations must be a nonempty array")
	}
	var v []Relation
	if e := Decode(b, &v); e != nil {
		return e
	}
	if len(v) == 0 {
		return fmt.Errorf("relations must be nonempty")
	}
	for _, x := range v {
		if (x.Type != "hasVersion" && x.Type != "isVersionOf") || !validID.MatchString(x.Work) {
			return fmt.Errorf("invalid relation type/target")
		}
	}
	*r = v
	return nil
}
func validateWorkRelations(works []Work, ids map[string]int) error {
	for _, w := range works {
		if w.Relations != nil && len(w.Relations) == 0 {
			return fmt.Errorf("empty relations")
		}
		for _, r := range w.Relations {
			inverse := "isVersionOf"
			if r.Type == inverse {
				inverse = "hasVersion"
			} else if r.Type != "hasVersion" {
				return fmt.Errorf("invalid public relation type")
			}
			i, ok := ids[r.Work]
			if !ok {
				return fmt.Errorf("relation target missing")
			}
			found := false
			for _, back := range works[i].Relations {
				found = found || (back.Type == inverse && back.Work == w.ID)
			}
			if !found {
				return fmt.Errorf("reciprocal relation missing")
			}
		}
	}
	return nil
}
