package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOfflineCLI(t *testing.T) {
	data := filepath.Join("..", "..", "testdata", "fixture.json")
	cache := t.TempDir()
	cases := []struct {
		args []string
		code int
		json bool
	}{{[]string{"query", "--cache", cache}, 3, false}, {[]string{"query", "--dataset", data, "--q", "zzzzunfindable"}, 0, true}, {[]string{"query", "--dataset", data, "--q", "Barbieri-Low"}, 0, true}, {[]string{"status", "--dataset", data}, 0, true}, {[]string{"query", "--limit", "101", "--cache", cache}, 2, false}, {[]string{"query", "--q", "a", "--q", "b"}, 2, false}, {[]string{"show", "--dataset", data, "--id", "missing"}, 7, false}, {[]string{"version"}, 0, true}, {[]string{"update", "--cache", cache}, 2, false}, {[]string{"update", "--cache", cache, "--bundle", data}, 0, true}, {[]string{"query", "--cache", cache, "--author", "alice-example"}, 0, true}}
	for _, c := range cases {
		var out, diag bytes.Buffer
		code := run(c.args, &out, &diag)
		if code != c.code {
			t.Fatalf("%v: code %d expected %d: %s", c.args, code, c.code, diag.String())
		}
		if c.code != 0 && out.Len() != 0 {
			t.Fatal("diagnostic on stdout")
		}
		if c.code == 0 && diag.Len() != 0 {
			t.Fatal("successful diagnostic")
		}
		if c.json && !json.Valid(out.Bytes()) {
			t.Fatal("invalid JSON", out.String())
		}
	}
	var out, diag bytes.Buffer
	if code := run([]string{"query", "--dataset", data, "--output", "jsonl", "--limit", "2"}, &out, &diag); code != 0 {
		t.Fatal(code, diag.String())
	}
	for _, line := range bytes.Split(bytes.TrimSpace(out.Bytes()), []byte{'\n'}) {
		if !json.Valid(line) {
			t.Fatal("bad JSONL")
		}
	}
	files, _ := os.ReadDir(cache)
	if len(files) != 2 {
		t.Fatal("unexpected cache files", files)
	}
}

func TestExplicitDatasetWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("LocalAppData", "")
	var out, diag bytes.Buffer
	if code := run([]string{"query", "--dataset", "../../testdata/fixture.json", "--q", "Barbieri-Low"}, &out, &diag); code != 0 {
		t.Fatal(code, diag.String())
	}
}
