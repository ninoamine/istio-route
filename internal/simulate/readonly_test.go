package simulate

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenCalls are write verbs, watches, and filesystem mutations that the simulation domain and live reader must never invoke.
var forbiddenCalls = map[string]bool{
	"Create":           true,
	"Update":           true,
	"Patch":            true,
	"Delete":           true,
	"DeleteCollection": true,
	"Watch":            true,
	"WriteFile":        true,
	"Rename":           true,
	"Remove":           true,
}

// forbiddenQualified covers package-qualified filesystem calls whose selector name alone is too generic.
var forbiddenQualified = map[string]bool{
	"os.WriteFile": true,
	"os.Create":    true,
	"os.Rename":    true,
	"os.Remove":    true,
}

// forbiddenIdentifiers are workload and backend lookups that simulation must not perform.
var forbiddenIdentifiers = map[string]bool{
	"GetPod":             true,
	"ListPods":           true,
	"ListEndpointSlices": true,
	"GetService":         true,
	"ListServices":       true,
}

func TestReadOnlyDomainHasNoWriteOrWorkloadCalls(t *testing.T) {
	files := readonlyGuardFiles(t)
	if len(files) == 0 {
		t.Fatal("no non-test Go files found for the read-only guard")
	}
	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.SelectorExpr:
				name := n.Sel.Name
				if forbiddenCalls[name] {
					t.Errorf("%s: forbidden call %s at %s", path, name, fset.Position(n.Pos()))
				}
				if qualifier, ok := n.X.(*ast.Ident); ok && forbiddenQualified[qualifier.Name+"."+name] {
					t.Errorf("%s: forbidden call %s.%s at %s", path, qualifier.Name, name, fset.Position(n.Pos()))
				}
			case *ast.Ident:
				if forbiddenIdentifiers[n.Name] {
					t.Errorf("%s: forbidden identifier %s at %s", path, n.Name, fset.Position(n.Pos()))
				}
			}
			return true
		})
	}
}

// readonlyGuardFiles returns the non-test Go files in internal/simulate plus the live waypoint reader.
func readonlyGuardFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	var files []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, filepath.Join(".", name))
	}
	return append(files, filepath.Join("..", "source", "waypoint.go"))
}
