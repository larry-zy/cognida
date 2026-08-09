// Package migrations 以 embed.FS 承载版本化 SQL 迁移文件（golang-migrate 源，〔INF-4〕）。
// 迁移文件命名遵循 golang-migrate 约定：NNNNNN_描述.up.sql / NNNNNN_描述.down.sql。
// 迁移由 cmd/migrate-db 工具驱动执行，运行时（cmd/server）不做任何自动建表/改表。
package migrations

import "embed"

// FS 内嵌本目录下全部 .sql 迁移文件，供 golang-migrate 的 iofs 源读取。
//
//go:embed *.sql
var FS embed.FS
