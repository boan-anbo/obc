package searchnorm

import (
	"bufio"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Run by hand against output from the site's own functions: PARITY=file go test -run Parity.
func TestParityAgainstTheSite(t *testing.T) {
	path := os.Getenv("PARITY")
	if path == "" {
		t.Skip("PARITY not set")
	}
	f, e := os.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 1<<20), 1<<24)
	n, bad := 0, 0
	for scan.Scan() {
		var row []json.RawMessage
		if e := json.Unmarshal(scan.Bytes(), &row); e != nil {
			t.Fatal(e)
		}
		var s, a, b, c string
		var d [][]string
		var f []string
		for i, p := range []any{&s, &a, &b, &c, &d, &f} {
			if e := json.Unmarshal(row[i], p); e != nil {
				t.Fatal(e)
			}
		}
		n++
		if Normalise(s) != a || Literal(s) != b || Romanise(s) != c || !reflect.DeepEqual(PinyinQueries(s), d) || !reflect.DeepEqual(NameReadings(s), f) {
			bad++
			if bad <= 8 {
				t.Errorf("%q: go %q %q %q %v %v; site %q %q %q %v %v", s, Normalise(s), Literal(s), Romanise(s), PinyinQueries(s), NameReadings(s), a, b, c, d, f)
			}
		}
	}
	t.Logf("%d strings, %d differ", n, bad)
	if bad > 0 {
		t.Fail()
	}
}
