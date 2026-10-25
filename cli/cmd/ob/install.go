package main

import (
	"encoding/json"
	"io"
	"openbibliographies/ob/internal/distribution"
	"openbibliographies/ob/internal/ob"
	"strings"
)

func installCommand(args []string, out io.Writer) error {
	options := map[string]string{}
	for i := 0; i < len(args); i++ {
		key := strings.TrimPrefix(args[i], "--")
		if key == args[i] {
			return ob.Invalid("install requires --package DIR --dir NEW_DIR")
		}
		value := ""
		if k := strings.IndexByte(key, '='); k >= 0 {
			value = key[k+1:]
			key = key[:k]
		} else {
			i++
			if i >= len(args) {
				return ob.Invalid("missing install option value")
			}
			value = args[i]
		}
		if key != "package" && key != "dir" {
			return ob.Invalid("unknown install option: " + key)
		}
		if _, ok := options[key]; ok {
			return ob.Invalid("repeated install option: " + key)
		}
		options[key] = value
	}
	result, e := distribution.Install(options["package"], options["dir"], version, commit)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(result)
}
