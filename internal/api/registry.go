// Package api binds declared operations to existing feature services.
package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type Operation struct {
	ID, Method, Path, AliasOf string
}

// Build validates everything on a private mux before exposing any routes.
// Resources include pages and documentation; they share collision checks but
// are not API operations. GET retains ServeMux's existing HEAD behavior.
func Build(operations []Operation, handlers map[string]http.HandlerFunc, resourceGroups ...map[string]http.Handler) (*http.ServeMux, error) {
	known := make(map[string]Operation)
	for _, op := range operations {
		if op.ID == "" || op.Method == "" || !strings.HasPrefix(op.Path, "/") {
			return nil, fmt.Errorf("invalid operation declaration: %+v", op)
		}
		if _, exists := known[op.ID]; exists {
			return nil, fmt.Errorf("duplicate operation ID %q", op.ID)
		}
		known[op.ID] = op
		if handlers[op.ID] == nil {
			return nil, fmt.Errorf("missing handler for %s (%s %s)", op.ID, op.Method, op.Path)
		}
	}
	for _, op := range operations {
		if op.AliasOf != "" {
			if target, ok := known[op.AliasOf]; !ok || op.AliasOf == op.ID || target.Method != op.Method {
				return nil, fmt.Errorf("invalid alias %s -> %s", op.ID, op.AliasOf)
			}
			seen := map[string]bool{op.ID: true}
			for target := op.AliasOf; target != ""; target = known[target].AliasOf {
				if seen[target] {
					return nil, fmt.Errorf("alias cycle at %s", op.ID)
				}
				seen[target] = true
			}
		}
	}
	var extra []string
	for id := range handlers {
		if _, exists := known[id]; !exists {
			extra = append(extra, id)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return nil, fmt.Errorf("undocumented handlers: %s", strings.Join(extra, ", "))
	}
	mux := http.NewServeMux()
	for _, op := range operations {
		if err := bind(mux, op.Method+" "+op.Path, handlers[op.ID], op.ID); err != nil {
			return nil, err
		}
	}
	resources := map[string]http.Handler{}
	var patterns []string
	for _, group := range resourceGroups {
		for pattern, handler := range group {
			if _, exists := resources[pattern]; exists {
				return nil, fmt.Errorf("duplicate resource route: %s", pattern)
			}
			resources[pattern] = handler
			patterns = append(patterns, pattern)
		}
	}
	sort.Strings(patterns)
	for _, pattern := range patterns {
		if resources[pattern] == nil {
			return nil, fmt.Errorf("missing resource handler: %s", pattern)
		}
		if err := bind(mux, pattern, resources[pattern], "resource"); err != nil {
			return nil, err
		}
	}
	return mux, nil
}

func bind(mux *http.ServeMux, pattern string, handler http.Handler, owner string) (err error) {
	defer func() {
		if failure := recover(); failure != nil {
			err = fmt.Errorf("route %s (%s): %v", pattern, owner, failure)
		}
	}()
	mux.Handle(pattern, handler)
	return nil
}
