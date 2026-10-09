package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"server-manager/internal/sshclient"
)

func contextWithTimeout(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 30*time.Second)
}

func pathID(r *http.Request, key string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, key), 10, 64)
}

// serverRow maps one row of the servers table.
type serverRow struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Host         string     `json:"host"`
	Port         int        `json:"port"`
	Username     string     `json:"username"`
	AuthType     string     `json:"auth_type"`
	CredentialID int64      `json:"credential_id"`
	Credential   *string    `json:"credential_name,omitempty"`
	Description  string     `json:"description"`
	Tags         string     `json:"tags"`
	Enabled      bool       `json:"enabled"`
	LastStatus   string     `json:"last_status"`
	LastCheckAt  *time.Time `json:"last_check_at"`
	CreatedAt    string     `json:"created_at"`
	UpdatedAt    string     `json:"updated_at"`
}

func (a *API) scanServer(scan func(dest ...any) error) (*serverRow, error) {
	var s serverRow
	var cred sqlNullString
	err := scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.Username, &s.AuthType, &s.CredentialID,
		&cred, &s.Description, &s.Tags, &s.Enabled, &s.LastStatus, &s.LastCheckAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if cred.Valid {
		s.Credential = &cred.String
	}
	return &s, nil
}

const serverSelect = `
SELECT s.id, s.name, s.host, s.port, s.username, s.auth_type, COALESCE(s.credential_id,0),
       c.name, s.description, s.tags, s.enabled, s.last_status, s.last_check_at, s.created_at, s.updated_at
FROM servers s LEFT JOIN credentials c ON c.id = s.credential_id`

func (a *API) handleServerList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(serverSelect + ` ORDER BY s.id`)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []*serverRow
	for rows.Next() {
		s, err := a.scanServer(rows.Scan)
		if err != nil {
			writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
			return
		}
		list = append(list, s)
	}
	writeData(w, r, http.StatusOK, list)
}

func (a *API) handleServerGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid server id", nil)
		return
	}
	s, err := a.scanServer(func(dest ...any) error {
		return a.db.QueryRow(serverSelect+` WHERE s.id=?`, id).Scan(dest...)
	})
	if err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "server not found", nil)
		return
	}
	writeData(w, r, http.StatusOK, s)
}

// serverInput is the create/update payload. Secrets are write-only.
type serverInput struct {
	Name        string  `json:"name"`
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	Username    string  `json:"username"`
	AuthType    string  `json:"auth_type"`
	Credential  *string `json:"credential"` // new password or private key (write-only)
	Description string  `json:"description"`
	Tags        string  `json:"tags"`
	Enabled     *bool   `json:"enabled"`
}

func (in *serverInput) validate() (code, msg string) {
	if len(in.Name) > 100 || in.Name == "" {
		return "INVALID_NAME", "name is required (max 100 chars)"
	}
	if in.Host == "" || len(in.Host) > 255 {
		return "INVALID_HOST", "host is required"
	}
	if in.Port < 0 || in.Port > 65535 {
		return "INVALID_PORT", "port out of range"
	}
	if in.Port == 0 {
		in.Port = 22
	}
	if in.Username == "" {
		in.Username = "root"
	}
	if in.AuthType != "password" && in.AuthType != "private_key" {
		return "INVALID_AUTH_TYPE", "auth_type must be password or private_key"
	}
	return "", ""
}

func (a *API) handleServerCreate(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	var in serverInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
		return
	}
	if code, msg := in.validate(); code != "" {
		writeErr(w, r, http.StatusBadRequest, code, msg, nil)
		return
	}

	var credID int64
	if in.Credential != nil && *in.Credential != "" {
		id, err := a.creds.Save(in.Name, "ssh_"+in.AuthType, *in.Credential)
		if err != nil {
			writeErr(w, r, http.StatusInternalServerError, "ENCRYPT_ERROR", "failed to encrypt credential", nil)
			return
		}
		credID = id
	}

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	res, err := a.db.Exec(
		`INSERT INTO servers (name, host, port, username, auth_type, credential_id, description, tags, enabled)
		 VALUES (?,?,?,?,?,?,?,?,?)`,
		in.Name, in.Host, in.Port, in.Username, in.AuthType, nullInt64(credID), in.Description, in.Tags, enabled)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	id, _ := res.LastInsertId()
	a.activity.Log(s.UserID, s.Username, "server_create", "server", strconvI64(id), map[string]string{"name": in.Name, "host": in.Host})
	writeData(w, r, http.StatusCreated, map[string]int64{"id": id})
}

func (a *API) handleServerUpdate(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid server id", nil)
		return
	}
	var in serverInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
		return
	}
	if code, msg := in.validate(); code != "" {
		writeErr(w, r, http.StatusBadRequest, code, msg, nil)
		return
	}

	var credID sqlNullInt64
	_ = a.db.QueryRow(`SELECT credential_id FROM servers WHERE id=?`, id).Scan(&credID)
	if in.Credential != nil && *in.Credential != "" {
		if credID.Valid {
			if err := a.creds.UpdateSecret(credID.Int64, *in.Credential); err != nil {
				writeErr(w, r, http.StatusInternalServerError, "ENCRYPT_ERROR", "failed to update credential", nil)
				return
			}
		} else {
			newID, err := a.creds.Save(in.Name, "ssh_"+in.AuthType, *in.Credential)
			if err != nil {
				writeErr(w, r, http.StatusInternalServerError, "ENCRYPT_ERROR", "failed to encrypt credential", nil)
				return
			}
			credID = sqlNullInt64Of(newID)
		}
	}

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	_, err = a.db.Exec(
		`UPDATE servers SET name=?, host=?, port=?, username=?, auth_type=?, credential_id=?, description=?, tags=?, enabled=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		in.Name, in.Host, in.Port, in.Username, in.AuthType, credID, in.Description, in.Tags, enabled, id)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	a.activity.Log(s.UserID, s.Username, "server_update", "server", strconvI64(id), map[string]string{"name": in.Name})
	writeData(w, r, http.StatusOK, map[string]string{"ok": "updated"})
}

func (a *API) handleServerDelete(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid server id", nil)
		return
	}
	var credID sqlNullInt64
	_ = a.db.QueryRow(`SELECT credential_id FROM servers WHERE id=?`, id).Scan(&credID)
	_, err = a.db.Exec(`DELETE FROM servers WHERE id=?`, id)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	if credID.Valid {
		_ = a.creds.Delete(credID.Int64)
	}
	a.activity.Log(s.UserID, s.Username, "server_delete", "server", strconvI64(id), nil)
	writeData(w, r, http.StatusOK, map[string]string{"ok": "deleted"})
}

// handleServerTest performs a full SSH handshake to validate the stored credential.
func (a *API) handleServerTest(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid server id", nil)
		return
	}
	row, err := a.scanServer(func(dest ...any) error {
		return a.db.QueryRow(serverSelect+` WHERE s.id=?`, id).Scan(dest...)
	})
	if err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "server not found", nil)
		return
	}
	secret, err := a.serverCredential(*row)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DECRYPT_ERROR", "cannot decrypt credential", nil)
		return
	}
	if secret == "" {
		writeErr(w, r, http.StatusBadRequest, "NO_CREDENTIAL", "server has no credential stored", nil)
		return
	}
	err = sshclient.Test(sshclient.Target{
		ID: row.ID, Name: row.Name, Host: row.Host, Port: row.Port,
		Username: row.Username, AuthType: row.AuthType, Secret: secret,
	})
	status := "online"
	if err != nil {
		status = "offline"
	}
	_, _ = a.db.Exec(`UPDATE servers SET last_status=?, last_check_at=CURRENT_TIMESTAMP WHERE id=?`, status, id)
	a.activity.Log(s.UserID, s.Username, "server_test", "server", strconvI64(id), map[string]string{"result": status})
	if err != nil {
		writeData(w, r, http.StatusOK, map[string]string{"status": status, "error": err.Error()})
		return
	}
	writeData(w, r, http.StatusOK, map[string]string{"status": status})
}
