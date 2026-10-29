// Package store persists watches and seen months in the local SQLite database
// (modernc.org/sqlite, sin CGO) at config.DataPath(). Two tables:
//
//   - watches: id, kind, target, sink, with_impact, created_at, with a
//     UNIQUE(kind, target) so a repeated `watch add` cannot duplicate.
//   - seen_months: watch_id, month (YYYY-MM), value (exact decimal text as
//     published by the Banco de España), seen_at and delivered_at, with a
//     UNIQUE(watch_id, month) so a repeated `watch check` cannot record the
//     same month twice (WATCH-01, decision 2). An undelivered row is a pending
//     retry, never a lost event (decision 5).
//
// The database is opened with the same discipline as the config: directory
// 0700, file 0600, and a contract error (internal/1) naming the path when it
// cannot be opened. Loan data lives in the config file, not here.
package store
