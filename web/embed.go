// Package web embeds the production frontend build.
// Rebuild with `npm run build` in web/ — the output lands in web/dist.
package web

import "embed"

// DistFS holds the built React app. A placeholder index.html exists so the
// project compiles before the first frontend build; `npm run build` replaces it.
//
//go:embed all:dist
var DistFS embed.FS
