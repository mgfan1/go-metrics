// Package migrations встраивает SQL-миграции схемы метрик в бинарник.
package migrations

import "embed"

// FS содержит файлы миграций для golang-migrate.
//
//go:embed *.sql
var FS embed.FS
