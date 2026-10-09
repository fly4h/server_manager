package api

import (
	"net/http"
	"net/url"
)

// websiteRow maps one row of the websites table.
type websiteRow struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	URL          string  `json:"url"`
	Icon         string  `json:"icon"`
	Category     string  `json:"category"`
	AuthType     string  `json:"auth_type"`
	CredentialID int64   `json:"credential_id"`
	Credential   *string `json:"credential_name,omitempty"`
	Description  string  `json:"description"`
	Enabled      bool    `json:"enabled"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

const websiteSelect = `
SELECT w.id, w.name, w.url, COALESCE(w.icon,''), COALESCE(w.category,'general'), w.auth_type,
       COALESCE(w.credential_id,0), c.name, COALESCE(w.description,''), w.enabled, w.created_at, w.updated_at
FROM websites w LEFT JOIN credentials c ON c.id = w.credential_id`

func (a *API) handleWebsiteList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(websiteSelect + ` ORDER BY w.id`)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []*websiteRow
	for rows.Next() {
		var (
			wr   websiteRow
			cred sqlNullString
		)
		if err := rows.Scan(&wr.ID, &wr.Name, &wr.URL, &wr.Icon, &wr.Category, &wr.AuthType,
			&wr.CredentialID, &cred, &wr.Description, &wr.Enabled, &wr.CreatedAt, &wr.UpdatedAt); err != nil {
			writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
			return
		}
		if cred.Valid {
			wr.Credential = &cred.String
		}
		list = append(list, &wr)
	}
	writeData(w, r, http.StatusOK, list)
}

func (a *API) handleWebsiteGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid website id", nil)
		return
	}
	var (
		wr   websiteRow
		cred sqlNullString
	)
	err = a.db.QueryRow(websiteSelect+` WHERE w.id=?`, id).Scan(
		&wr.ID, &wr.Name, &wr.URL, &wr.Icon, &wr.Category, &wr.AuthType,
		&wr.CredentialID, &cred, &wr.Description, &wr.Enabled, &wr.CreatedAt, &wr.UpdatedAt)
	if err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "website not found", nil)
		return
	}
	if cred.Valid {
		wr.Credential = &cred.String
	}
	writeData(w, r, http.StatusOK, wr)
}

// websiteInput is the create/update payload. Secrets are write-only.
type websiteInput struct {
	Name        string  `json:"name"`
	URL         string  `json:"url"`
	Icon        string  `json:"icon"`
	Category    string  `json:"category"`
	AuthType    string  `json:"auth_type"`
	Credential  *string `json:"credential"` // password / token / cookie (write-only)
	Description string  `json:"description"`
	Enabled     *bool   `json:"enabled"`
}

var allowedWebsiteAuthTypes = map[string]bool{
	"none": true, "basic_auth": true, "form": true, "cookie": true, "token": true, "custom": true,
}

func (in *websiteInput) validate() (code, msg string) {
	if in.Name == "" || len(in.Name) > 100 {
		return "INVALID_NAME", "name is required (max 100 chars)"
	}
	u, err := url.Parse(in.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "INVALID_URL", "url must be an absolute http(s) URL"
	}
	if in.AuthType == "" {
		in.AuthType = "none"
	}
	if in.Category == "" {
		in.Category = "general"
	}
	if !allowedWebsiteAuthTypes[in.AuthType] {
		return "INVALID_AUTH_TYPE", "auth_type not supported"
	}
	if in.AuthType == "none" && in.Credential != nil && *in.Credential != "" {
		return "CREDENTIAL_NOT_ALLOWED", "auth_type none cannot carry a credential"
	}
	return "", ""
}

func (a *API) handleWebsiteCreate(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	var in websiteInput
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
		id, err := a.creds.Save(in.Name, "website_"+in.AuthType, *in.Credential)
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
		`INSERT INTO websites (name, url, icon, category, auth_type, credential_id, description, enabled)
		 VALUES (?,?,?,?,?,?,?,?)`,
		in.Name, in.URL, in.Icon, in.Category, in.AuthType, nullInt64(credID), in.Description, enabled)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	id, _ := res.LastInsertId()
	a.activity.Log(s.UserID, s.Username, "website_create", "website", strconvI64(id), map[string]string{"name": in.Name, "url": in.URL})
	writeData(w, r, http.StatusCreated, map[string]int64{"id": id})
}

func (a *API) handleWebsiteUpdate(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid website id", nil)
		return
	}
	var in websiteInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
		return
	}
	if code, msg := in.validate(); code != "" {
		writeErr(w, r, http.StatusBadRequest, code, msg, nil)
		return
	}
	var credID sqlNullInt64
	_ = a.db.QueryRow(`SELECT credential_id FROM websites WHERE id=?`, id).Scan(&credID)
	if in.Credential != nil && *in.Credential != "" {
		if credID.Valid {
			if err := a.creds.UpdateSecret(credID.Int64, *in.Credential); err != nil {
				writeErr(w, r, http.StatusInternalServerError, "ENCRYPT_ERROR", "failed to update credential", nil)
				return
			}
		} else {
			newID, err := a.creds.Save(in.Name, "website_"+in.AuthType, *in.Credential)
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
		`UPDATE websites SET name=?, url=?, icon=?, category=?, auth_type=?, credential_id=?, description=?, enabled=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		in.Name, in.URL, in.Icon, in.Category, in.AuthType, credID, in.Description, enabled, id)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	a.activity.Log(s.UserID, s.Username, "website_update", "website", strconvI64(id), map[string]string{"name": in.Name})
	writeData(w, r, http.StatusOK, map[string]string{"ok": "updated"})
}

func (a *API) handleWebsiteDelete(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid website id", nil)
		return
	}
	var credID sqlNullInt64
	_ = a.db.QueryRow(`SELECT credential_id FROM websites WHERE id=?`, id).Scan(&credID)
	_, err = a.db.Exec(`DELETE FROM websites WHERE id=?`, id)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	if credID.Valid {
		_ = a.creds.Delete(credID.Int64)
	}
	a.activity.Log(s.UserID, s.Username, "website_delete", "website", strconvI64(id), nil)
	writeData(w, r, http.StatusOK, map[string]string{"ok": "deleted"})
}
