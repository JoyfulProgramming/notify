// Package rulestore is the SQLite-backed rule-store adapter shared by the
// rule-api and filter-service (plan section 9, decision #6: shared file,
// same process in this single-binary build).
package rulestore

import (
	"database/sql"

	"notify/pkg/contracts"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS rules (
	id             TEXT PRIMARY KEY,
	user_id        TEXT NOT NULL,
	source_app     TEXT NOT NULL DEFAULT '',
	source_account TEXT NOT NULL DEFAULT '',
	title          TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_rules_user_id ON rules(user_id);
`

type Store struct {
	db *sql.DB
}

// Open opens (and migrates) the SQLite file at path. ":memory:" is valid for tests.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// modernc.org/sqlite has no real connection pooling story for a single file;
	// keep one connection so concurrent CRUD + filter reads don't hit SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Create(r contracts.Rule) error {
	_, err := s.db.Exec(
		`INSERT INTO rules (id, user_id, source_app, source_account, title) VALUES (?, ?, ?, ?, ?)`,
		r.ID, r.UserID, r.SourceApp, r.SourceAccount, r.Title,
	)
	return err
}

func (s *Store) List(userID string) ([]contracts.Rule, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, source_app, source_account, title FROM rules WHERE user_id = ? ORDER BY id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []contracts.Rule
	for rows.Next() {
		var r contracts.Rule
		if err := rows.Scan(&r.ID, &r.UserID, &r.SourceApp, &r.SourceAccount, &r.Title); err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// Delete reports whether a rule owned by userID with the given id existed.
func (s *Store) Delete(userID, id string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM rules WHERE user_id = ? AND id = ?`, userID, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
