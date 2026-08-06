package store

import "database/sql"

// UserRow is the persisted user record (including the bcrypt hash).
type UserRow struct {
	ID           string
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    string
}

// CountUsers returns the number of registered users (0 => first-run/setup mode).
func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	return n, err
}

// GetUserByUsername looks up a user by username (case-insensitive).
func (s *Store) GetUserByUsername(username string) (*UserRow, error) {
	var u UserRow
	err := s.db.QueryRow("SELECT id, username, password_hash, role, created_at FROM users WHERE lower(username) = lower(?)", username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// AddUser inserts a new user.
func (s *Store) AddUser(u UserRow) error {
	_, err := s.db.Exec("INSERT INTO users (id, username, password_hash, role, created_at) VALUES (?, ?, ?, ?, ?)",
		u.ID, u.Username, u.PasswordHash, u.Role, u.CreatedAt)
	return err
}
