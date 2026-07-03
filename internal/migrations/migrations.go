// Package migrations embeds the goose SQL migrations so the single binary
// can migrate its own database (ADR-0001: one artifact + Postgres).
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
