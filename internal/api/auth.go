package api

import (
	"net"
	"net/http"
	"time"

	"server-manager/internal/auth"
	"server-manager/internal/database"
	"server-manager/internal/dockerapi"
)

// handleLogin authenticates a user and sets the session cookie.
func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	if loginBlocked(ip) {
		writeErr(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "too many failed attempts, wait 15s", nil)
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Username == "" || req.Password == "" {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "username and password required", nil)
		return
	}

	var (
		id           int64
		passwordHash string
		role         string
	)
	err := a.db.QueryRow(`SELECT id, password_hash, role FROM users WHERE username=?`, req.Username).
		Scan(&id, &passwordHash, &role)
	if err != nil {
		loginFailed(ip)
		writeErr(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username or password", nil)
		return
	}
	ok, err := auth.VerifyPassword(req.Password, passwordHash)
	if err != nil || !ok {
		loginFailed(ip)
		writeErr(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid username or password", nil)
		return
	}
	loginSucceeded(ip)

	token := a.sessions.Create(id, req.Username, role)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.cfg.IsProd(),
		SameSite: http.SameSiteLaxMode,
	})
	a.activity.Log(id, req.Username, "login", "auth", "", nil)
	writeData(w, r, http.StatusOK, map[string]any{"username": req.Username, "role": role})
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeData(w, r, http.StatusOK, map[string]string{"ok": "logged out"})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	writeData(w, r, http.StatusOK, map[string]any{
		"username":   s.Username,
		"role":       s.Role,
		"expires_at": s.ExpiresAt.Format(time.RFC3339),
	})
}

func (a *API) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid request body", nil)
		return
	}
	if len(req.NewPassword) < 8 {
		writeErr(w, r, http.StatusBadRequest, "WEAK_PASSWORD", "new password must be at least 8 characters", nil)
		return
	}
	var hash string
	err := a.db.QueryRow(`SELECT password_hash FROM users WHERE id=?`, s.UserID).Scan(&hash)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	ok, err := auth.VerifyPassword(req.OldPassword, hash)
	if err != nil || !ok {
		writeErr(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "old password is wrong", nil)
		return
	}
	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "HASH_ERROR", err.Error(), nil)
		return
	}
	if _, err := a.db.Exec(`UPDATE users SET password_hash=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, newHash, s.UserID); err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	a.activity.Log(s.UserID, s.Username, "change_password", "user", "", nil)
	writeData(w, r, http.StatusOK, map[string]string{"ok": "password changed"})
}

// handleSystemInfo reports build/runtime facts for the Settings page.
func (a *API) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	dockerOK := false
	if a.docker != nil {
		ctx, cancel := contextWithTimeout(r)
		defer cancel()
		dockerOK = a.docker.Ping(ctx) == nil
	}
	var dbSize int64
	var pageCount, pageSize int64
	if err := a.db.QueryRow(`PRAGMA page_count`).Scan(&pageCount); err == nil {
		if err := a.db.QueryRow(`PRAGMA page_size`).Scan(&pageSize); err == nil {
			dbSize = pageCount * pageSize
		}
	}
	writeData(w, r, http.StatusOK, map[string]any{
		"version":     "1.0.0",
		"go_version":  goVersion,
		"env":         a.cfg.Env,
		"db_size":     dbSize,
		"docker_ok":   dockerOK,
		"docker_host": dockerHostDisplay(a.cfg.DockerHost),
	})
}

func dockerHostDisplay(h string) string {
	switch {
	case h == "":
		return "unix:///var/run/docker.sock"
	default:
		return h
	}
}

// EnsureAPIUsed keeps database/dockerapi imports referenced even if future edits
// remove direct usage — cheap compile-time insurance for a single-binary build.
var (
	_ = database.Open
	_ = dockerapi.New
)
