package credential

import (
	"database/sql"
	"time"
)

// Item is a stored credential. Secret is never serialized to API responses.
type Item struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store persists credentials with encrypted payloads.
type Store struct {
	db  *sql.DB
	mgr *Manager
}

func NewStore(db *sql.DB, mgr *Manager) *Store { return &Store{db: db, mgr: mgr} }

// Save encrypts the secret and inserts a new credential row.
func (s *Store) Save(name, typ, secret string) (int64, error) {
	enc, err := s.mgr.Encrypt([]byte(secret))
	if err != nil {
		return 0, err
	}
	res, err := s.db.Exec(
		`INSERT INTO credentials (name, type, encrypted_data) VALUES (?,?,?)`,
		name, typ, enc)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateSecret re-encrypts the secret for an existing credential.
func (s *Store) UpdateSecret(id int64, secret string) error {
	enc, err := s.mgr.Encrypt([]byte(secret))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE credentials SET encrypted_data=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, enc, id)
	return err
}

// UpdateMeta renames a credential without touching its secret.
func (s *Store) UpdateMeta(id int64, name, typ string) error {
	_, err := s.db.Exec(`UPDATE credentials SET name=?, type=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, typ, id)
	return err
}

// Resolve decrypts the secret of a credential.
func (s *Store) Resolve(id int64) (string, error) {
	var enc string
	err := s.db.QueryRow(`SELECT encrypted_data FROM credentials WHERE id=?`, id).Scan(&enc)
	if err != nil {
		return "", err
	}
	plain, err := s.mgr.Decrypt(enc)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// List returns metadata only — never the secret.
func (s *Store) List() ([]Item, error) {
	rows, err := s.db.Query(`SELECT id, name, type, created_at, updated_at FROM credentials ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.Type, &it.CreatedAt, &it.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

// Delete removes a credential. References from servers/websites are cleared
// by the ON DELETE SET NULL rule.
func (s *Store) Delete(id int64) error {
	_, err := s.db.Exec(`DELETE FROM credentials WHERE id=?`, id)
	return err
}
