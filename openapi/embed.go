// Package openapi embeds the API contract so the server can publish it.
package openapi

import _ "embed"

//go:embed openapi.yaml
var Spec []byte
