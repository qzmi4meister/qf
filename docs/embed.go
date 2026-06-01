// Package docs embeds the OpenAPI spec for serving via the CP REST API.
package docs

import _ "embed"

//go:embed openapi.yaml
var OpenAPISpec []byte
