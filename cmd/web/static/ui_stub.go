//go:build !webui

// Package static provides stub values when the binary is built without the webui tag.
// In this mode the server only serves API endpoints (no embedded SPA).
package static

import "embed"

// HasUI is false when compiled without -tags webui.
// The HTTP server will skip SPA static file serving and only expose /api/v1/* routes.
const HasUI = false

// Files is an empty filesystem placeholder.
var Files embed.FS
