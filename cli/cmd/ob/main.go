package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"openbibliographies/ob/internal/ob"
)

var version = "0.1.0"
var commit = "unknown"

const help = `ob — Open Bibliographies offline reader

  ob query [--q TEXT] [--author KEY] [--sort recommended|title|year-new|year-old]
           [--discipline IDs] [--region IDs] [--period IDs] [--subject IDs]
           [--topic IDs] [--format IDs] [--workType IDs] [--facets IDs]
           [--offset N] [--limit 1..100] [--snapshot SNAPSHOT] [--output json|jsonl]
  ob search ...                 alias for query
  ob show --id WORK_ID
  ob update --bundle FILE       acquire a data bundle locally
  ob update --source HTTPS_URL  acquire/update using a verified CLI manifest
  ob status
  ob version
  ob install --package DIR --dir NEW_DIR  install an offline package locally

Every data command accepts --dataset FILE (read only) or --cache DIR.
Queries and status never use the network. Default output is JSON; JSONL emits
one public work per line. Diagnostics go to stderr. See README for exit codes.
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, out, diag io.Writer) int {
	e := execute(args, out)
	if e == nil {
		return 0
	}
	var oe *ob.Error
	if errors.As(e, &oe) {
		fmt.Fprintln(diag, e)
		return oe.Exit
	}
	fmt.Fprintln(diag, "DATA_ERROR:", e)
	return 4
}
func execute(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, e := io.WriteString(out, help)
		return e
	}
	cmd := args[0]
	if cmd == "install" {
		return installCommand(args[1:], out)
	}
	if cmd == "version" || cmd == "--version" {
		if len(args) != 1 {
			return ob.Invalid("version accepts no options")
		}
		return json.NewEncoder(out).Encode(map[string]string{"version": version, "commit": commit, "readerVersion": ob.ReaderVersion, "bundleSchema": ob.BundleSchema, "querySchema": "openbibliographies.query.v1"})
	}
	if cmd != "query" && cmd != "search" && cmd != "show" && cmd != "status" && cmd != "update" {
		return ob.Invalid("Unknown command: " + cmd)
	}
	params := url.Values{}
	special := map[string]string{}
	for i := 1; i < len(args); i++ {
		if args[i] == "--help" {
			_, e := io.WriteString(out, help)
			return e
		}
		if !strings.HasPrefix(args[i], "--") {
			return ob.Invalid("Options require --name VALUE")
		}
		key := strings.TrimPrefix(args[i], "--")
		value := ""
		if at := strings.IndexByte(key, '='); at >= 0 {
			value = key[at+1:]
			key = key[:at]
		} else {
			i++
			if i >= len(args) {
				return ob.Invalid("Missing value for --" + key)
			}
			value = args[i]
		}
		if _, ok := special[key]; ok || len(params[key]) > 0 {
			return ob.Invalid("Repeated parameter: " + key)
		}
		switch key {
		case "dataset", "cache", "source", "bundle", "id":
			special[key] = value
		default:
			params[key] = []string{value}
		}
	}
	if cmd == "query" || cmd == "search" {
		if special["source"] != "" || special["bundle"] != "" || special["id"] != "" {
			return ob.Invalid("Unsupported query option")
		}
	} else if len(params) > 0 {
		return ob.Invalid("Query options are only accepted by query/search")
	}
	cache := special["cache"]
	allowedSpecial := map[string]bool{"cache": true, "dataset": true}
	if cmd == "show" {
		allowedSpecial["id"] = true
	}
	if cmd == "update" {
		allowedSpecial = map[string]bool{"cache": true, "source": true, "bundle": true}
	}
	for key := range special {
		if !allowedSpecial[key] {
			return ob.Invalid("Unsupported option for " + cmd + ": " + key)
		}
	}
	if cache == "" && (special["dataset"] == "" || cmd == "update") {
		var e error
		cache, e = ob.DefaultCache()
		if e != nil {
			return e
		}
	}
	if cmd == "update" {
		if special["dataset"] != "" || special["id"] != "" {
			return ob.Invalid("update uses --cache and --source/--bundle")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
		defer cancel()
		c, e := ob.Update(ctx, cache, special["source"], special["bundle"], nil)
		if e != nil {
			return e
		}
		return json.NewEncoder(out).Encode(c)
	}
	if special["source"] != "" || special["bundle"] != "" {
		return ob.Invalid("source/bundle require update")
	}
	var q ob.Query
	var e error
	if cmd == "query" || cmd == "search" {
		q, e = ob.Parse(params)
		if e != nil {
			return e
		}
	}
	path := special["dataset"]
	var current *ob.Current
	if path == "" {
		path, current, e = ob.CurrentData(cache)
		if e != nil {
			return e
		}
	}
	x, e := ob.Load(path)
	if errors.Is(e, os.ErrNotExist) {
		return &ob.Error{Code: "NO_DATA", Message: "Dataset file does not exist", Exit: 3}
	}
	if e != nil {
		return e
	}
	if current != nil {
		b, e := ob.ReadFile(path)
		if e != nil {
			return e
		}
		if int64(len(b)) != current.Manifest.Size || ob.Digest(b) != current.Manifest.SHA256 || x.Bundle.Snapshot != current.Manifest.Snapshot {
			return fmt.Errorf("cache integrity mismatch")
		}
	}
	switch cmd {
	case "status":
		if special["id"] != "" {
			return ob.Invalid("status accepts no id")
		}
		return json.NewEncoder(out).Encode(map[string]any{"schema": "openbibliographies.status.v1", "path": path, "snapshot": x.Bundle.Snapshot, "development": x.Bundle.Development, "source": x.Bundle.Source, "works": len(x.Catalog.Works), "generated": x.Catalog.Generated, "catalogSHA256": x.Bundle.Retrieval.CatalogSHA256, "authorVersion": x.Bundle.Retrieval.AuthorVersion, "semanticsSHA256": x.Bundle.Retrieval.SemanticsSHA256, "cache": current})
	case "show":
		id := special["id"]
		if id == "" {
			return ob.Invalid("show requires --id WORK_ID")
		}
		for _, w := range x.Catalog.Works {
			if w.ID == id {
				return json.NewEncoder(out).Encode(ob.Public(w))
			}
		}
		return &ob.Error{Code: "NOT_FOUND", Message: "Unknown work ID", Exit: 7}
	default:
		r, e := x.Execute(q)
		if e != nil {
			return e
		}
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		if q.Output == "jsonl" {
			for _, w := range r.Works {
				if e = enc.Encode(w); e != nil {
					return e
				}
			}
			return nil
		}
		return enc.Encode(r)
	}
}
