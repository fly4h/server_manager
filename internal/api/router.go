package api

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"io/fs"

	"server-manager/internal/activity"
	"server-manager/internal/auth"
	"server-manager/internal/config"
	"server-manager/internal/credential"
	"server-manager/internal/database"
	"server-manager/internal/dockerapi"
	web "server-manager/web"
)

const sessionCookie = "sm_session"

// API bundles all handler dependencies.
type API struct {
	cfg      *config.Config
	db       *database.DB
	sessions *auth.SessionStore
	creds    *credential.Store
	activity *activity.Logger
	docker   *dockerapi.Client
}

func New(cfg *config.Config, db *database.DB, sessions *auth.SessionStore, creds *credential.Store, act *activity.Logger, docker *dockerapi.Client) *API {
	return &API{cfg: cfg, db: db, sessions: sessions, creds: creds, activity: act, docker: docker}
}

// Router builds the full HTTP routing tree.
func (a *API) Router() http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.Recoverer)
	r.Use(middleware.RealIP)
	r.Use(a.requestLogger)
	if !a.cfg.IsProd() {
		r.Use(a.corsDev)
	}
	// CSRF mitigation: state-changing calls must come from our own origin.
	r.Use(a.originCheck)

	r.Route("/api", func(r chi.Router) {
		// auth
		r.Post("/auth/login", a.handleLogin)
		r.Post("/auth/logout", a.handleLogout)
		r.Get("/auth/me", a.requireAuth(a.handleMe))
		r.Put("/auth/password", a.requireAuth(a.handleChangePassword))

		// dashboard & activity
		r.Get("/dashboard/stats", a.requireAuth(a.handleDashboardStats))
		r.Get("/activity", a.requireAuth(a.handleActivityList))

		// system
		r.Get("/system/info", a.requireAuth(a.handleSystemInfo))

		// servers + terminal
		r.Get("/servers", a.requireAuth(a.handleServerList))
		r.Post("/servers", a.requireAuth(a.handleServerCreate))
		r.Get("/servers/{id}", a.requireAuth(a.handleServerGet))
		r.Put("/servers/{id}", a.requireAuth(a.handleServerUpdate))
		r.Delete("/servers/{id}", a.requireAuth(a.handleServerDelete))
		r.Post("/servers/{id}/test", a.requireAuth(a.handleServerTest))
		r.Get("/servers/{id}/terminal", a.requireAuthWS(a.handleTerminal))

		// commands
		r.Get("/commands", a.requireAuth(a.handleCommandList))
		r.Post("/commands", a.requireAuth(a.handleCommandCreate))
		r.Put("/commands/{id}", a.requireAuth(a.handleCommandUpdate))
		r.Delete("/commands/{id}", a.requireAuth(a.handleCommandDelete))
		r.Post("/commands/{id}/render", a.requireAuth(a.handleCommandRender))
		r.Post("/servers/{serverId}/commands/{commandId}/execute", a.requireAuth(a.handleCommandExecute))

		// websites
		r.Get("/websites", a.requireAuth(a.handleWebsiteList))
		r.Post("/websites", a.requireAuth(a.handleWebsiteCreate))
		r.Get("/websites/{id}", a.requireAuth(a.handleWebsiteGet))
		r.Put("/websites/{id}", a.requireAuth(a.handleWebsiteUpdate))
		r.Delete("/websites/{id}", a.requireAuth(a.handleWebsiteDelete))

		// credentials
		r.Get("/credentials", a.requireAuth(a.handleCredentialList))
		r.Post("/credentials", a.requireAuth(a.handleCredentialCreate))
		r.Put("/credentials/{id}", a.requireAuth(a.handleCredentialUpdate))
		r.Delete("/credentials/{id}", a.requireAuth(a.handleCredentialDelete))

		// docker
		r.Get("/docker/status", a.requireAuth(a.handleDockerStatus))
		r.Get("/docker/containers", a.requireAuth(a.handleDockerContainers))
		r.Get("/docker/images", a.requireAuth(a.handleDockerImages))
		r.Get("/docker/volumes", a.requireAuth(a.handleDockerVolumes))
		r.Get("/docker/networks", a.requireAuth(a.handleDockerNetworks))
		r.Post("/docker/containers/{id}/start", a.requireAuth(a.dockerContainerAction("start")))
		r.Post("/docker/containers/{id}/stop", a.requireAuth(a.dockerContainerAction("stop")))
		r.Post("/docker/containers/{id}/restart", a.requireAuth(a.dockerContainerAction("restart")))
		r.Delete("/docker/containers/{id}", a.requireAuth(a.handleDockerContainerRemove))
		r.Get("/docker/containers/{id}/logs", a.requireAuth(a.handleDockerContainerLogs))
		r.Get("/docker/containers/{id}/inspect", a.requireAuth(a.handleDockerContainerInspect))
	})

	// SPA static assets from the embedded frontend build.
	dist, err := fs.Sub(web.DistFS, "dist")
	if err == nil {
		fileServer := http.FileServerFS(dist)
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
			path := strings.TrimPrefix(req.URL.Path, "/")
			if path != "" {
				if _, err := fs.Stat(dist, path); err == nil {
					fileServer.ServeHTTP(w, req)
					return
				}
			}
			// SPA fallback for client-side routes.
			index, err2 := fs.ReadFile(dist, "index.html")
			if err2 != nil {
				http.NotFound(w, req)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(index)
		})
	}
	return r
}

// ---- middleware ----

func (a *API) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(a, r)
		if sess == nil {
			writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "login required", nil)
			return
		}
		ctx := context.WithValue(r.Context(), sessionKey{}, sess)
		next(w, r.WithContext(ctx))
	}
}

// requireAuthWS authenticates the WebSocket handshake via the session cookie.
func (a *API) requireAuthWS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(a, r)
		if sess == nil {
			writeErr(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "login required", nil)
			return
		}
		ctx := context.WithValue(r.Context(), sessionKey{}, sess)
		next(w, r.WithContext(ctx))
	}
}

type sessionKey struct{}

func currentSession(r *http.Request) *auth.Session {
	s, _ := r.Context().Value(sessionKey{}).(*auth.Session)
	return s
}

func (a *API) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			if r.URL.Path != "/api/auth/login" || ww.Status() >= 400 {
				// keep it quiet: skip successful health-ish polling
			}
			_ = start
		} else {
			next.ServeHTTP(w, r)
		}
	})
}

// originCheck blocks cross-site state-changing requests (CSRF defense).
func (a *API) originCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			origin := r.Header.Get("Origin")
			if origin != "" {
				host := r.Host
				if !sameOrigin(origin, host) {
					writeErr(w, r, http.StatusForbidden, "ORIGIN_REJECTED", "cross-origin request blocked", nil)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(origin, host string) bool {
	u := strings.TrimPrefix(strings.TrimPrefix(origin, "https://"), "http://")
	return strings.EqualFold(u, host)
}

func (a *API) corsDev(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- login rate limiting ----

var loginAttempts sync.Map // ip -> attempt state

func loginBlocked(ip string) bool {
	v, ok := loginAttempts.Load(ip)
	if !ok {
		return false
	}
	at := v.(*loginState)
	if at.failures >= 5 && time.Since(at.last) < 15*time.Second {
		return true
	}
	return false
}

func loginFailed(ip string) {
	now := time.Now()
	v, _ := loginAttempts.LoadOrStore(ip, &loginState{})
	at := v.(*loginState)
	if now.Sub(at.last) > time.Minute {
		at.failures = 0
	}
	at.failures++
	at.last = now
}

func loginSucceeded(ip string) { loginAttempts.Delete(ip) }

type loginState struct {
	failures int
	last     time.Time
}

// ---- helpers shared by handlers ----

func (a *API) serverCredential(srvRow serverRow) (string, error) {
	if srvRow.CredentialID == 0 {
		return "", nil
	}
	return a.creds.Resolve(srvRow.CredentialID)
}

var _ = sql.ErrNoRows
