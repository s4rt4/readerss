package web

import "embed"

// Static contains the web/static directory for single-binary builds.
//
//go:embed static
var Static embed.FS
