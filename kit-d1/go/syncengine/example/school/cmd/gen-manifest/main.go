// Command gen-manifest renders the project's sync.schema.json from the Go
// registry — the single source of truth the worker template consumes.
//
// Usage (from the kit repo root):
//
//	go run ./go/syncengine/example/school/cmd/gen-manifest -namespace school-tracker -o worker/instances/school-tracker/sync.schema.json
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Keimer92/Compass-Go/kit-d1/go/syncengine/example/school"
)

func main() {
	namespace := flag.String("namespace", "school-tracker", "project namespace stamped into the manifest")
	out := flag.String("o", "", "output file (default: stdout)")
	flag.Parse()

	raw, err := schoolschema.Must().ManifestJSON(*namespace)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	raw = append(raw, '\n')

	if *out == "" {
		os.Stdout.Write(raw)
		return
	}
	if err := os.WriteFile(*out, raw, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stderr, "wrote", *out)
}
