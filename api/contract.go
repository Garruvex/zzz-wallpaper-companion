package api

import _ "embed"

// Contract is the canonical wire contract embedded directly for offline builds.
//
//go:embed openapi.json
var Contract []byte
