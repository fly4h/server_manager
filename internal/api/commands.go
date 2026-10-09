package api

import (
	"net/http"

	"server-manager/internal/commandexec"
	"server-manager/internal/sshclient"
)

// commandRow maps one row of the commands table.
type commandRow struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Command     string `json:"command"`
	DangerLevel string `json:"danger_level"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

const commandSelect = `SELECT id, name, COALESCE(description,''), command, danger_level, enabled, created_at, updated_at FROM commands`

func (a *API) handleCommandList(w http.ResponseWriter, r *http.Request) {
	rows, err := a.db.Query(commandSelect + ` ORDER BY id`)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	defer rows.Close()
	var list []*commandRow
	for rows.Next() {
		var c commandRow
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Command, &c.DangerLevel, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
			return
		}
		list = append(list, &c)
	}
	writeData(w, r, http.StatusOK, list)
}

type commandInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Command     string `json:"command"`
	DangerLevel string `json:"danger_level"`
	Enabled     *bool  `json:"enabled"`
}

func (in *commandInput) validate() (code, msg string) {
	if in.Name == "" || len(in.Name) > 100 {
		return "INVALID_NAME", "name is required (max 100 chars)"
	}
	if in.Command == "" || len(in.Command) > 2000 {
		return "INVALID_COMMAND", "command is required (max 2000 chars)"
	}
	switch in.DangerLevel {
	case "low", "medium", "high", "critical":
	default:
		return "INVALID_DANGER", "danger_level must be low/medium/high/critical"
	}
	return "", ""
}

func (a *API) handleCommandCreate(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	var in commandInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
		return
	}
	if code, msg := in.validate(); code != "" {
		writeErr(w, r, http.StatusBadRequest, code, msg, nil)
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	res, err := a.db.Exec(
		`INSERT INTO commands (name, description, command, danger_level, enabled) VALUES (?,?,?,?,?)`,
		in.Name, in.Description, in.Command, in.DangerLevel, enabled)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	id, _ := res.LastInsertId()
	a.activity.Log(s.UserID, s.Username, "command_create", "command", strconvI64(id), map[string]string{"name": in.Name})
	writeData(w, r, http.StatusCreated, map[string]int64{"id": id})
}

func (a *API) handleCommandUpdate(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid command id", nil)
		return
	}
	var in commandInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
		return
	}
	if code, msg := in.validate(); code != "" {
		writeErr(w, r, http.StatusBadRequest, code, msg, nil)
		return
	}
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	_, err = a.db.Exec(
		`UPDATE commands SET name=?, description=?, command=?, danger_level=?, enabled=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		in.Name, in.Description, in.Command, in.DangerLevel, enabled, id)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	a.activity.Log(s.UserID, s.Username, "command_update", "command", strconvI64(id), map[string]string{"name": in.Name})
	writeData(w, r, http.StatusOK, map[string]string{"ok": "updated"})
}

func (a *API) handleCommandDelete(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid command id", nil)
		return
	}
	_, err = a.db.Exec(`DELETE FROM commands WHERE id=?`, id)
	if err != nil {
		writeErr(w, r, http.StatusInternalServerError, "DB_ERROR", err.Error(), nil)
		return
	}
	a.activity.Log(s.UserID, s.Username, "command_delete", "command", strconvI64(id), nil)
	writeData(w, r, http.StatusOK, map[string]string{"ok": "deleted"})
}

// handleCommandRender previews a template with params — used by the UI
// before executing dangerous commands.
func (a *API) handleCommandRender(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid command id", nil)
		return
	}
	var req struct {
		Params map[string]string `json:"params"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_REQUEST", "invalid json", nil)
		return
	}
	var cmd string
	var storedLevel string
	if err := a.db.QueryRow(`SELECT command, danger_level FROM commands WHERE id=?`, id).
		Scan(&cmd, &storedLevel); err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "command not found", nil)
		return
	}
	rendered, err := commandexec.Render(cmd, req.Params)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "INVALID_PARAMS", err.Error(), nil)
		return
	}
	// Same effective level execute() computes, so preview and confirmation agree.
	level := commandexec.DangerLevel(storedLevel, rendered)
	writeData(w, r, http.StatusOK, map[string]any{
		"rendered":     rendered,
		"danger_level": level,
		"need_confirm": commandexec.NeedsConfirm(level),
	})
}

// handleCommandExecute runs a saved command on a server via SSH.
// High/critical commands require confirm=true (second confirmation).
func (a *API) handleCommandExecute(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	serverID, err := pathID(r, "serverId")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid server id", nil)
		return
	}
	commandID, err := pathID(r, "commandId")
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "BAD_ID", "invalid command id", nil)
		return
	}

	var req struct {
		Params  map[string]string `json:"params"`
		Confirm bool              `json:"confirm"`
	}
	if err := decodeJSON(r, &req); err != nil {
		req.Params = nil // allow empty body
	}

	var cmd commandRow
	if err := a.db.QueryRow(commandSelect+` WHERE id=?`, commandID).
		Scan(&cmd.ID, &cmd.Name, &cmd.Description, &cmd.Command, &cmd.DangerLevel, &cmd.Enabled, &cmd.CreatedAt, &cmd.UpdatedAt); err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "command not found", nil)
		return
	}
	if !cmd.Enabled {
		writeErr(w, r, http.StatusBadRequest, "COMMAND_DISABLED", "command is disabled", nil)
		return
	}

	row, err := a.scanServer(func(dest ...any) error {
		return a.db.QueryRow(serverSelect+` WHERE s.id=?`, serverID).Scan(dest...)
	})
	if err != nil {
		writeErr(w, r, http.StatusNotFound, "NOT_FOUND", "server not found", nil)
		return
	}
	secret, err := a.serverCredential(*row)
	if err != nil || secret == "" {
		writeErr(w, r, http.StatusBadRequest, "NO_CREDENTIAL", "server has no usable credential", nil)
		return
	}

	rendered, err := commandexec.Render(cmd.Command, req.Params)
	if err != nil {
		writeErr(w, r, http.StatusBadRequest, "INVALID_PARAMS", err.Error(), nil)
		return
	}

	level := commandexec.DangerLevel(cmd.DangerLevel, rendered)
	if commandexec.NeedsConfirm(level) && !req.Confirm {
		// Do NOT execute — ask the client to confirm with the rendered command shown.
		writeErr(w, r, http.StatusConflict, "CONFIRMATION_REQUIRED",
			"this command is dangerous ("+level+"), confirm to execute",
			map[string]any{"danger_level": level, "command": rendered})
		return
	}

	ctx, cancel := contextWithTimeout(r)
	defer cancel()
	output, err := sshclient.Run(ctx, sshclient.Target{
		ID: row.ID, Name: row.Name, Host: row.Host, Port: row.Port,
		Username: row.Username, AuthType: row.AuthType, Secret: secret,
	}, rendered)

	a.activity.Log(s.UserID, s.Username, "command_execute", "server", strconvI64(serverID), map[string]string{
		"server": row.Name, "command": cmd.Name, "danger": level,
	})
	writeData(w, r, http.StatusOK, map[string]any{
		"command": rendered,
		"danger_level": level,
		"output":     output,
		"error":      errString(err),
	})
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
