package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	contract "github.com/Garruvex/zzz-wallpaper-companion/api"
)

func TestGeneratedBindingsMatchContract(t *testing.T) {
	if fmt.Sprintf("%x", sha256.Sum256(bytes.ReplaceAll(contract.Contract, []byte("\r\n"), []byte("\n")))) != ContractDigest {
		t.Fatal("contract digest drift; regenerate bindings")
	}
	var doc struct {
		Paths map[string]map[string]struct {
			ID    string `json:"operationId"`
			Alias string `json:"x-alias-of"`
		}
	}
	if err := json.Unmarshal(contract.Contract, &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]Operation{}
	for path, item := range doc.Paths {
		for method, op := range item {
			want[op.ID] = Operation{op.ID, strings.ToUpper(method), path, op.Alias}
		}
	}
	for _, op := range Operations() {
		if want[op.ID] != op {
			t.Fatalf("generated operation drift: %+v", op)
		}
		delete(want, op.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing generated operations: %v", want)
	}
}

func TestRegistryRejectsIncompleteOrConflictingBindings(t *testing.T) {
	handler := func(http.ResponseWriter, *http.Request) {}
	base := Operation{ID: "test.get", Method: "GET", Path: "/api/{id}"}
	for _, tc := range []struct {
		name      string
		ops       []Operation
		handlers  map[string]http.HandlerFunc
		resources map[string]http.Handler
		message   string
	}{
		{"missing", []Operation{base}, nil, nil, "missing handler"},
		{"nil", []Operation{base}, map[string]http.HandlerFunc{base.ID: nil}, nil, "missing handler"},
		{"extra", nil, map[string]http.HandlerFunc{"typo": handler}, nil, "undocumented handlers"},
		{"duplicate ID", []Operation{base, base}, map[string]http.HandlerFunc{base.ID: handler}, nil, "duplicate operation ID"},
		{"equivalent wildcard", []Operation{base, {ID: "test.other", Method: "GET", Path: "/api/{name}"}}, map[string]http.HandlerFunc{base.ID: handler, "test.other": handler}, nil, "conflicts"},
		{"ambiguous", []Operation{{ID: "a", Method: "GET", Path: "/api/{id}/details"}, {ID: "b", Method: "GET", Path: "/api/current/{file}"}}, map[string]http.HandlerFunc{"a": handler, "b": handler}, nil, "conflicts"},
		{"resource collision", []Operation{base}, map[string]http.HandlerFunc{base.ID: handler}, map[string]http.Handler{"GET /api/{name}": http.HandlerFunc(handler)}, "conflicts"},
		{"bad pattern", []Operation{{ID: "a", Method: "GET", Path: "/api/{"}}, map[string]http.HandlerFunc{"a": handler}, nil, "route GET /api/{ (a)"},
		{"alias missing", []Operation{{ID: "a", Method: "POST", Path: "/alias", AliasOf: "absent"}}, map[string]http.HandlerFunc{"a": handler}, nil, "invalid alias"},
		{"alias cycle", []Operation{{ID: "a", Method: "POST", Path: "/a", AliasOf: "b"}, {ID: "b", Method: "POST", Path: "/b", AliasOf: "a"}}, map[string]http.HandlerFunc{"a": handler, "b": handler}, nil, "alias cycle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux, err := Build(tc.ops, tc.handlers, tc.resources)
			if err == nil || mux != nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("got mux=%v err=%v, want %s", mux, err, tc.message)
			}
		})
	}
}

func TestRegistryPreservesSpecificityMethodsAndAliases(t *testing.T) {
	ops := []Operation{{ID: "file", Method: "GET", Path: "/api/carts/{id}/{file}"}, {ID: "details", Method: "GET", Path: "/api/carts/{id}/details"}, {ID: "renew", Method: "POST", Path: "/api/renew"}, {ID: "legacy", Method: "POST", Path: "/api/legacy", AliasOf: "renew"}}
	handlers := map[string]http.HandlerFunc{}
	for _, op := range ops {
		id := op.ID
		handlers[id] = func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, id+":"+r.PathValue("id")) }
	}
	mux, err := Build(ops, handlers, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{{"GET", "/api/carts/abc/details", "details:abc", 200}, {"HEAD", "/api/carts/abc/rom", "file:abc", 200}, {"POST", "/api/legacy", "legacy:", 200}, {"GET", "/api/renew", "", 405}} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status || tc.body != "" && w.Body.String() != tc.body {
			t.Fatalf("%s %s: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

func TestRegistryRejectsDuplicateResourceGroups(t *testing.T) {
	resources := map[string]http.Handler{"GET /docs": http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})}
	if _, err := Build(nil, nil, resources, resources); err == nil || !strings.Contains(err.Error(), "duplicate resource route") {
		t.Fatalf("%v", err)
	}
}
