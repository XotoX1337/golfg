package i18n

import "embed"

// localesFS holds the per-language TOML message catalogs, embedded so the app
// still ships as a single binary.
//
//go:embed locales/*.toml
var localesFS embed.FS
