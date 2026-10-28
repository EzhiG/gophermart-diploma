// Package migrations содержит SQL-миграции схемы базы.
// Миграции встроены в бинарный файл и применяются при запуске сервиса.
package migrations

import "embed"

// FS - файлы миграций *.sql.
//
//go:embed *.sql
var FS embed.FS
