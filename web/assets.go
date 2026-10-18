// Package webfs menanam template HTML dan aset statis ke dalam binary.
package webfs

import "embed"

// FS berisi folder templates/ dan static/.
//
//go:embed templates static
var FS embed.FS
