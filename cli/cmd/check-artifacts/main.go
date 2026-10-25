package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"openbibliographies/ob/internal/distribution"
	"os"
	"sort"
)

func main() {
	dir := flag.String("package", "dist/release", "offline package directory")
	flag.Parse()
	p, e := distribution.Open(*dir)
	if e == nil {
		var report distribution.Report
		report, e = p.Inspect()
		if e == nil {
			sort.Slice(report.Binaries, func(i, j int) bool { return report.Binaries[i].File < report.Binaries[j].File })
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			e = enc.Encode(report)
		}
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
