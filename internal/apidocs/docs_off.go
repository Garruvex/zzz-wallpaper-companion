//go:build !apidocs

package apidocs

import "net/http"

// Enabled reports whether this build serves the explorer. Builds without
// -tags apidocs (releases included) leave it and its Swagger assets out.
const Enabled = false

func Routes() map[string]http.Handler { return nil }

func Register(*http.ServeMux) {}
