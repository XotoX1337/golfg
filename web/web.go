// Package web embeds the server-rendered templates and built static assets so
// the application ships as a single binary.
package web

import "embed"

// Templates holds the html/template files (rooted at "templates/").
//
//go:embed templates
var Templates embed.FS

// Static holds the built/served assets (rooted at "static/"). Edit sources under
// assets/ and run `make esbuild` to regenerate static/css/app.min.css.
//
//go:embed all:static
var Static embed.FS
