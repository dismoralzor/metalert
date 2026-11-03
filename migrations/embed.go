// Package migrations хранит SQL-миграции схемы БД и вшивает их в бинарник.
// //go:embed не умеет подниматься выше директории пакета, поэтому эмбеддер
// живёт рядом с *.sql, а не в internal/repository.
package migrations

import "embed"

// FS содержит вшитые в бинарник файлы SQL-миграций (*.sql).
//
//go:embed *.sql
var FS embed.FS
