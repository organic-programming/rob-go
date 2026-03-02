package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormat(t *testing.T) {
	input := "package main\nfunc main(){}\n"
	formatted, changed, err := Format(input, "main.go")
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}
	if !changed {
		t.Fatal("expected formatting to change source")
	}
	if !strings.Contains(formatted, "func main() {}") {
		t.Fatalf("formatted=%q", formatted)
	}
}

func TestFormatInvalid(t *testing.T) {
	_, _, err := Format("package main\nfunc main({}\n", "bad.go")
	if err == nil {
		t.Fatal("expected syntax error")
	}
}

func TestParse(t *testing.T) {
	src := `package sample

import "fmt"

// Greeter says hello.
type Greeter struct {
	Name string
}

var count int
const Pi = 3.14

func Hello(name string) string {
	return fmt.Sprintf("hi %s", name)
}
`
	result, err := Parse(src, "sample.go", true)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if result.PackageName != "sample" {
		t.Fatalf("package=%q, want sample", result.PackageName)
	}
	if len(result.Imports) != 1 || result.Imports[0] != "fmt" {
		t.Fatalf("imports=%v", result.Imports)
	}

	kinds := map[string]bool{}
	for _, d := range result.Declarations {
		kinds[d.Kind] = true
	}
	if !kinds["type"] || !kinds["var"] || !kinds["const"] || !kinds["func"] {
		t.Fatalf("declarations missing expected kinds: %+v", result.Declarations)
	}
}

func TestParseExported(t *testing.T) {
	src := `package p

func Exported() {}
func unexported() {}
`
	result, err := Parse(src, "p.go", false)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if len(result.Declarations) != 2 {
		t.Fatalf("declarations=%d, want 2", len(result.Declarations))
	}
	if !result.Declarations[0].Exported {
		t.Fatalf("first declaration should be exported: %+v", result.Declarations[0])
	}
	if result.Declarations[1].Exported {
		t.Fatalf("second declaration should be unexported: %+v", result.Declarations[1])
	}
}

func TestTypeCheckValid(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod":  "module example.com/valid\n\ngo 1.24.0\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})

	result, err := TypeCheck([]string{"./..."}, dir, nil)
	if err != nil {
		t.Fatalf("TypeCheck error: %v", err)
	}
	if !result.OK {
		t.Fatalf("expected OK, diagnostics=%+v", result.Diagnostics)
	}
}

func TestTypeCheckErrors(t *testing.T) {
	dir := writeModule(t, map[string]string{
		"go.mod": "module example.com/invalid\n\ngo 1.24.0\n",
		"bad.go": "package invalid\n\nfunc f() { var x int = \"nope\"; _ = x }\n",
	})

	result, err := TypeCheck([]string{"./..."}, dir, nil)
	if err != nil {
		t.Fatalf("TypeCheck error: %v", err)
	}
	if result.OK {
		t.Fatalf("expected errors, got OK with packages=%+v", result.Packages)
	}
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected diagnostics")
	}
}

func TestLoadPackages(t *testing.T) {
	pkgs, err := LoadPackages([]string{"fmt"}, ".", nil, false)
	if err != nil {
		t.Fatalf("LoadPackages error: %v", err)
	}
	if len(pkgs) == 0 {
		t.Fatal("expected at least one package")
	}
	if len(pkgs[0].GoFiles) == 0 {
		t.Fatalf("expected go files in package %+v", pkgs[0])
	}
}

func TestDoc(t *testing.T) {
	result, err := Doc("fmt", ".")
	if err != nil {
		t.Fatalf("Doc error: %v", err)
	}
	if result.PackageName != "fmt" {
		t.Fatalf("package=%q, want fmt", result.PackageName)
	}
	if strings.TrimSpace(result.PackageDoc) == "" {
		t.Fatal("expected package doc")
	}
}

func writeModule(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return dir
}
