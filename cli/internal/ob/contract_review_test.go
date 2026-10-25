package ob

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewRealMappingFix(t *testing.T) {
	dir := os.Getenv("OB_RELEASE_PACKAGE")
	if dir == "" {
		t.Skip("OB_RELEASE_PACKAGE required for actual release proof")
	}
	data, e := os.ReadFile(filepath.Join(dir, "catalog.json.gz"))
	if e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(dir, "cli-manifest.json"))
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if e = Decode(raw, &m); e != nil {
		t.Fatal(e)
	}
	if _, e = m.ValidateBundle(data); e != nil {
		t.Fatal("actual release rejected", e)
	}
	m.Release.Overlay.MappingSHA256 = strings.Repeat("0", 64)
	_, e = m.ValidateBundle(data)
	if e == nil {
		t.Fatal("contradiction accepted")
	}
}
