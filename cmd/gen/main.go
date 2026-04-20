// Command gen scaffolds new feature modules under internal/modules/<name>/.
//
// Usage:
//
//	gen module <name> [--minimal] [--plural <form>]
//	gen verify-todo-drift
//
// Templates live under cmd/gen/templates/{full,minimal}/ and are embedded
// into the binary. The reference module lives at internal/modules/todo/;
// `gen verify-todo-drift` renders the full/ templates with Name=todo and
// compares byte-for-byte against internal/modules/todo/, so CI can fail
// if either drifts out of sync.
package main

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"
)

//go:embed all:templates
var templatesFS embed.FS

type data struct {
	Name       string
	NameLower  string
	NamePascal string
	NamePlural string
	ModulePath string
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "module":
		if err := runModule(os.Args[2:]); err != nil {
			fail(err)
		}
	case "verify-todo-drift":
		if err := runDriftCheck(); err != nil {
			fail(err)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func runModule(args []string) error {
	// Parse flags manually so callers can write either order:
	//   gen module users --minimal
	//   gen module --minimal users
	// (Go's flag.Parse stops at the first non-flag arg, so we can't rely on it here.)
	var (
		name    string
		minimal bool
		plural  string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--minimal", a == "-minimal":
			minimal = true
		case a == "--plural", a == "-plural":
			if i+1 >= len(args) {
				return fmt.Errorf("--plural requires a value")
			}
			i++
			plural = args[i]
		case strings.HasPrefix(a, "--plural="), strings.HasPrefix(a, "-plural="):
			_, plural, _ = strings.Cut(a, "=")
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag: %s", a)
		default:
			if name != "" {
				return fmt.Errorf("unexpected argument %q (already saw module name %q)", a, name)
			}
			name = a
		}
	}
	if name == "" {
		return fmt.Errorf("usage: gen module <name> [--minimal] [--plural <form>]")
	}
	if err := validateName(name); err != nil {
		return err
	}
	modulePath, err := readModulePath()
	if err != nil {
		return err
	}
	if plural == "" {
		plural = name + "s"
	}
	d := data{
		Name:       name,
		NameLower:  strings.ToLower(name),
		NamePascal: toPascal(name),
		NamePlural: plural,
		ModulePath: modulePath,
	}
	flavor := "full"
	if minimal {
		flavor = "minimal"
	}
	dest := filepath.Join("internal", "modules", d.NameLower)
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("%s already exists", dest)
	}
	if err := renderTree(flavor, dest, d); err != nil {
		return err
	}
	printSnippet(d, flavor)
	return nil
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("module name is required")
	}
	if !unicode.IsLetter(rune(name[0])) {
		return fmt.Errorf("name must start with a letter: %q", name)
	}
	for _, r := range name {
		if !(unicode.IsLower(r) || unicode.IsDigit(r) || r == '_') {
			return fmt.Errorf("name must be lowercase letters, digits, or underscores: %q", name)
		}
	}
	return nil
}

func toPascal(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

func readModulePath() (string, error) {
	f, err := os.Open("go.mod")
	if err != nil {
		return "", fmt.Errorf("read go.mod (run from project root): %w", err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if rest, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(rest), nil
		}
	}
	return "", fmt.Errorf("no module line found in go.mod")
}

func renderTree(flavor, dest string, d data) error {
	root := filepath.Join("templates", flavor)
	return fs.WalkDir(templatesFS, root, func(path string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = strings.ReplaceAll(rel, "__name__", d.NameLower)
		rel = strings.TrimSuffix(rel, ".tmpl")
		outPath := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		tmplBytes, err := templatesFS.ReadFile(path)
		if err != nil {
			return err
		}
		t, err := template.New(rel).Parse(string(tmplBytes))
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, d); err != nil {
			return fmt.Errorf("execute %s: %w", rel, err)
		}
		return os.WriteFile(outPath, buf.Bytes(), 0o644)
	})
}

func printSnippet(d data, flavor string) {
	var ctor, routes string
	if flavor == "minimal" {
		ctor = fmt.Sprintf("%sHandler := %s.New(v)", d.NameLower, d.NameLower)
		routes = fmt.Sprintf("%sHandler.RegisterQueryRoutes(api)", d.NameLower)
	} else {
		ctor = fmt.Sprintf("%sHandler := %s.New(db, v)", d.NameLower, d.NameLower)
		routes = fmt.Sprintf("%sHandler.RegisterQueryRoutes(api)\n       %sHandler.RegisterMutationRoutes(mutations)", d.NameLower, d.NameLower)
	}
	migrationNote := ""
	if flavor == "full" {
		migrationNote = fmt.Sprintf(`
  - Add a migration if needed:
      internal/platform/postgres/migrations/NNNN_%s.up.sql
      internal/platform/postgres/migrations/NNNN_%s.down.sql`, d.NamePlural, d.NamePlural)
	}
	fmt.Printf(`Module %q scaffolded (%s).

Add to internal/app/wire.go:

  1. Import (import block):
       "%s/internal/modules/%s"

  2. Construct (section "4. Modules"):
       %s

  3. Register routes (section "5. Router"):
       %s

Next steps:%s
  - make docs  # regenerate OpenAPI after annotating handlers
`, d.Name, flavor, d.ModulePath, d.NameLower, ctor, routes, migrationNote)
}

func runDriftCheck() error {
	tmp, err := os.MkdirTemp("", "gen-drift-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	modulePath, err := readModulePath()
	if err != nil {
		return err
	}
	d := data{
		Name:       "todo",
		NameLower:  "todo",
		NamePascal: "Todo",
		NamePlural: "todos",
		ModulePath: modulePath,
	}
	if err := renderTree("full", tmp, d); err != nil {
		return err
	}
	ref := filepath.Join("internal", "modules", "todo")
	diffs := 0
	err = filepath.WalkDir(tmp, func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(tmp, p)
		genBytes, _ := os.ReadFile(p)
		refBytes, refErr := os.ReadFile(filepath.Join(ref, rel))
		if refErr != nil {
			fmt.Fprintf(os.Stderr, "drift: generated %s is missing from internal/modules/todo/\n", rel)
			diffs++
			return nil
		}
		if !bytes.Equal(genBytes, refBytes) {
			fmt.Fprintf(os.Stderr, "drift: %s differs\n", rel)
			diffs++
		}
		return nil
	})
	if err != nil {
		return err
	}
	err = filepath.WalkDir(ref, func(p string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(ref, p)
		if _, statErr := os.Stat(filepath.Join(tmp, rel)); statErr != nil {
			fmt.Fprintf(os.Stderr, "drift: internal/modules/todo/%s is missing from templates\n", rel)
			diffs++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if diffs > 0 {
		return fmt.Errorf("drift detected: %d file(s); update cmd/gen/templates/full/ or internal/modules/todo/ to match", diffs)
	}
	fmt.Println("no drift")
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  gen module <name> [--minimal] [--plural <form>]
  gen verify-todo-drift`)
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "error: %v\n", err)
	os.Exit(1)
}
