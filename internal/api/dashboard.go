package api

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"server-manager/internal/sshclient"
)

// handleDashboardStats returns headline counters plus server online status.
// Online check is a cheap TCP ping done concurrently so 10+ servers
// still answer in ~2s.
func (a *API) handleDashboardStats(w http.ResponseWriter, r *http.Request) {
	var (
		servers  int
		enabled  int
		websites int
		commands int
	)
	_ = a.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(enabled),0) FROM servers`).Scan(&servers, &enabled)
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM websites`).Scan(&websites)
	_ = a.db.QueryRow(`SELECT COUNT(*) FROM commands`).Scan(&commands)

	type pingRow struct {
		id   int64
		host string
		port int
	}
	rows, err := a.db.Query(`SELECT id, host, port FROM servers WHERE enabled=1`)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	var toPing []pingRow
	for rows.Next() {
		var p pingRow
		if err := rows.Scan(&p.id, &p.host, &p.port); err == nil {
			toPing = append(toPing, p)
		}
	}
	rows.Close()

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		online int
	)
	results := make(map[int64]string, len(toPing))
	for _, p := range toPing {
		wg.Add(1)
		go func(p pingRow) {
			defer wg.Done()
			ok := sshclient.Ping(p.host, p.port)
			status := "offline"
			if ok {
				status = "online"
			}
			mu.Lock()
			results[p.id] = status
			if ok {
				online++
			}
			mu.Unlock()
			_, _ = a.db.Exec(`UPDATE servers SET last_status=?, last_check_at=CURRENT_TIMESTAMP WHERE id=?`, status, p.id)
		}(p)
	}
	wg.Wait()

	entries, err := a.activity.List(10)
	if err != nil {
		entries = nil
	}
	writeData(w, r, http.StatusOK, map[string]any{
		"servers":         servers,
		"servers_enabled": enabled,
		"servers_online":  online,
		"websites":        websites,
		"commands":        commands,
		"server_status":   results,
		"recent_activity": entries,
	})
}

func (a *API) handleActivityList(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	entries, err := a.activity.List(limit)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	writeData(w, r, http.StatusOK, entries)
}

var _ = time.Now
