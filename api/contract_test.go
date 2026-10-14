package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type operation struct {
	OperationID string
	Parameters  []struct {
		Name     string
		In       string
		Required bool
	}
	Responses map[string]json.RawMessage
}
type document struct {
	OpenAPI    string
	Paths      map[string]map[string]operation
	Components struct{ Schemas map[string]json.RawMessage }
}

func TestContractStructureAndRoutes(t *testing.T) {
	var doc document
	if err := json.Unmarshal(Contract, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OpenAPI != "3.0.3" {
		t.Fatal("expected OpenAPI 3.0.3")
	}
	ids := map[string]bool{}
	routes := map[string]bool{}
	bindings := map[string]string{}
	mux := http.NewServeMux()
	for path, item := range doc.Paths {
		for method, op := range item {
			if op.OperationID == "" || ids[op.OperationID] {
				t.Fatalf("missing/duplicate operation ID %q", op.OperationID)
			}
			ids[op.OperationID] = true
			if len(op.Responses) == 0 {
				t.Fatalf("%s has no responses", op.OperationID)
			}
			parameters := map[string]bool{}
			for _, p := range op.Parameters {
				if p.In == "path" && p.Required {
					parameters[p.Name] = true
				}
			}
			for _, segment := range strings.Split(path, "/") {
				if strings.HasPrefix(segment, "{") && !parameters[strings.Trim(segment, "{}")] {
					t.Fatalf("%s lacks required path parameter %s", op.OperationID, segment)
				}
			}
			pattern := strings.ToUpper(method) + " " + path
			// Actual ServeMux semantics allow details over the file wildcard.
			mux.HandleFunc(pattern, func(http.ResponseWriter, *http.Request) {})
			routes[pattern] = true
			bindings[op.OperationID] = pattern
		}
	}
	// Structural coverage only; handlers retain semantic validation.
	var raw any
	if err := json.Unmarshal(Contract, &raw); err != nil {
		t.Fatal(err)
	}
	var refs func(any)
	refs = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				const prefix = "#/components/schemas/"
				if !strings.HasPrefix(ref, prefix) {
					t.Fatalf("unsupported reference %s", ref)
				}
				if _, ok := doc.Components.Schemas[strings.TrimPrefix(ref, prefix)]; !ok {
					t.Fatalf("unresolved reference %s", ref)
				}
			}
			if _, ok := v["operationId"]; ok {
				policy := v["x-explorer-execution"]
				if policy != "read" && policy != "explicit" && policy != "disabled" {
					t.Fatalf("missing execution policy: %v", v["operationId"])
				}
			}
			for _, child := range v {
				refs(child)
			}
		case []any:
			for _, child := range v {
				refs(child)
			}
		}
	}
	refs(raw)
	files, err := filepath.Glob("../internal/companion/*.go")
	if err != nil {
		t.Fatal(err)
	}
	resources := map[string]bool{"GET /ui/api.js": true, "GET /": true, "GET /settings": true, "GET /dates": true, "GET /holidays": true, "GET /games": true, "GET /licenses/unicode": true, "GET /resource/svg/cross.svg": true, "GET /ui/theme.css": true, "GET /ui/shell.js": true}
	for _, filename := range files {
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(source, func(n ast.Node) bool {
			// Normal listener binds by operation ID; resource maps stay separate.
			if pair, ok := n.(*ast.KeyValueExpr); ok {
				if literal, ok := pair.Key.(*ast.BasicLit); ok && literal.Kind == token.STRING {
					key, _ := strconv.Unquote(literal.Value)
					if pattern, ok := bindings[key]; ok {
						delete(routes, pattern)
					}
					if resources[key] {
						delete(resources, key)
					}
				}
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "HandleFunc" {
				return true
			}
			literal, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				t.Error("route pattern must be explicit in baseline")
				return true
			}
			pattern, err := strconv.Unquote(literal.Value)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(filename) == "connection.go" {
				recovery := map[string]bool{"GET /ui/api.js": true, "GET /settings": true, "GET /api/settings": true, "POST /api/settings": true}
				if !recovery[pattern] {
					t.Errorf("unexpected recovery route %s", pattern)
				}
				return true
			}
			if strings.Contains(pattern, " /api/") {
				t.Errorf("API route bypasses operation registry: %s", pattern)
			} else {
				if !resources[pattern] {
					t.Errorf("unclassified resource route %s", pattern)
				}
				delete(resources, pattern)
			}
			return true
		})
	}
	for pattern := range routes {
		t.Errorf("contract route has no server binding: %s", pattern)
	}
	for pattern := range resources {
		t.Errorf("resource inventory route has no binding: %s", pattern)
	}
}
