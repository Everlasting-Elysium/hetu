// Package web embeds the built frontend assets (web/dist) so that `go build`
// produces a single binary serving both API and UI.
package web

import "embed"

// DistFS holds the Vite build output. The dist/ directory is populated by
// `cd web && npm run build`; a placeholder index.html is committed so that
// go:embed never fails on a clean checkout.
//
//go:embed all:dist
var DistFS embed.FS
