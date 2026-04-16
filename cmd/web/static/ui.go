//go:build webui

// Package static embeds the compiled React frontend into the korsair-web binary.
// The dist/ directory is populated by "make build-frontend" before compilation.
package static

import "embed"

// HasUI is true when compiled with -tags webui (production build with embedded SPA).
const HasUI = true

//go:embed dist
var Files embed.FS
