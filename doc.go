// Package sqlcollect collects database/sql rows into typed values using generic
// callbacks. It provides scalar scanning and reflection-based struct scanning
// with db tags, independent of the database driver.
//
// Rows and OneRow take ownership of a successfully queried
// *sql.Rows and close it on every return path. Callers must handle query errors
// before passing rows to a collector.
package sqlcollect
