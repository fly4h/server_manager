package api

import (
	"encoding/json"
	"net/http"
	"reflect"

	"server-manager/internal/auth"
)

// Envelope follows the PRD error contract:
// success -> {"success":true,"data":...}
// failure -> {"success":false,"error":{"code","message"}}
type Envelope struct {
	Success bool        `json:"success"`
	Data    any         `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// writeData responds with the standard success envelope.
// Nil slices/maps are normalised to empty collections so list endpoints always
// return [] rather than null — the frontend can iterate without special cases.
func writeData(w http.ResponseWriter, r *http.Request, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Success: true, Data: normalizeEmpty(data)})
}

// normalizeEmpty turns nil slices and maps into empty non-nil collections.
func normalizeEmpty(v any) any {
	if v == nil {
		return v
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice:
		if rv.IsNil() {
			return reflect.MakeSlice(rv.Type(), 0, 0).Interface()
		}
	case reflect.Map:
		if rv.IsNil() {
			return reflect.MakeMap(rv.Type()).Interface()
		}
	}
	return v
}

func writeErr(w http.ResponseWriter, r *http.Request, status int, code, message string, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Envelope{Success: false, Error: &ErrorBody{Code: code, Message: message, Data: data}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// sessionFrom extracts the current session; nil if unauthenticated.
func sessionFrom(a *API, r *http.Request) *auth.Session {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil
	}
	return a.sessions.Get(c.Value)
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	return dec.Decode(v)
}
