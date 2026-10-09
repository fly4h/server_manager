package api

import (
	"net/http"
)

// handleCredentialList returns metadata only — secrets never leave the server.
func (a *API) handleCredentialList(w http.ResponseWriter, r *http.Request) {
	items, err := a.creds.List()
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	writeData(w, r, http.StatusOK, items)
}

type credentialInput struct {
	Name   string  `json:"name"`
	Type   string  `json:"type"`
	Secret *string `json:"secret"` // write-only
}

func (in *credentialInput) validate() (code, msg string) {
	if in.Name == "" || len(in.Name) > 100 {
		return "INVALID_NAME", "name is required (max 100 chars)"
	}
	if in.Type == "" || len(in.Type) > 50 {
		return "INVALID_TYPE", "type is required"
	}
	return "", ""
}

func (a *API) handleCredentialCreate(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	var in credentialInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
		return
	}
	if code, msg := in.validate(); code != "" {
		writeErr(w, r, http.StatusBadRequest, code, msg, nil)
		return
	}
	secret := ""
	if in.Secret != nil {
		secret = *in.Secret
	}
	id, err := a.creds.Save(in.Name, in.Type, secret)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	a.activity.Log(s.UserID, s.Username, "credential_create", "credential", strconvI64(id), map[string]string{"name": in.Name, "type": in.Type})
	writeData(w, r, http.StatusCreated, map[string]int64{"id": id})
}

func (a *API) handleCredentialUpdate(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid credential id", nil)
		return
	}
	var in credentialInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
		return
	}
	if code, msg := in.validate(); code != "" {
		writeErr(w, r, http.StatusBadRequest, code, msg, nil)
		return
	}
	if err := a.creds.UpdateMeta(id, in.Name, in.Type); err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	if in.Secret != nil && *in.Secret != "" {
		if err := a.creds.UpdateSecret(id, *in.Secret); err != nil {
			writeErr(w, r, http.StatusInternalServerError, "ENCRYPT_ERROR", "failed to update secret", nil)
			return
		}
	}
	a.activity.Log(s.UserID, s.Username, "credential_update", "credential", strconvI64(id), map[string]string{"name": in.Name})
	writeData(w, r, http.StatusOK, map[string]string{"ok": "updated"})
}

func (a *API) handleCredentialDelete(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid credential id", nil)
		return
	}
	if err := a.creds.Delete(id); err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	a.activity.Log(s.UserID, s.Username, "credential_delete", "credential", strconvI64(id), nil)
	writeData(w, r, http.StatusOK, map[string]string{"ok": "deleted"})
}
