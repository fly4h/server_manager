package api

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	"server-manager/internal/sshclient"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		// session cookie already authenticated the handshake; origin is
		// additionally checked by the originCheck middleware.
		return true
	},
}

// terminalMessage is the JSON protocol between xterm.js and the backend.
//
//	Client -> Server: {"type":"input","data":"ls -la\n"} | {"type":"resize","cols":120,"rows":40}
//	Server -> Client: {"type":"output","data":"..."} | {"type":"error","message":"..."} | {"type":"closed"}
type terminalMessage struct {
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
	Cols int    `json:"cols,omitempty"`
	Rows int    `json:"rows,omitempty"`
	Message string `json:"message,omitempty"`
}

// handleTerminal bridges a WebSocket to an SSH PTY session.
func (a *API) handleTerminal(w http.ResponseWriter, r *http.Request) {
	sess := currentSession(r)
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
	if err != nil || secret == "" {
		writeErr(w, r, http.StatusBadRequest, "NO_CREDENTIAL", "server has no usable credential", nil)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // upgrade already wrote the HTTP error
	}
	defer conn.Close()

	sshClient, sshSession, err := sshclient.NewSession(sshclient.Target{
		ID: row.ID, Name: row.Name, Host: row.Host, Port: row.Port,
		Username: row.Username, AuthType: row.AuthType, Secret: secret,
	})
	if err != nil {
		sendTerminalMsg(conn, terminalMessage{Type: "error", Message: "ssh connect failed: " + err.Error()})
		sendTerminalMsg(conn, terminalMessage{Type: "closed"})
		return
	}
	defer sshClient.Close()

	stdin, err := sshSession.StdinPipe()
	if err != nil {
		sendTerminalMsg(conn, terminalMessage{Type: "error", Message: err.Error()})
		return
	}
	stdout, err := sshSession.StdoutPipe()
	if err != nil {
		sendTerminalMsg(conn, terminalMessage{Type: "error", Message: err.Error()})
		return
	}
	stderr, err := sshSession.StderrPipe()
	if err != nil {
		sendTerminalMsg(conn, terminalMessage{Type: "error", Message: err.Error()})
		return
	}
	if err := sshSession.RequestPty("xterm-256color", 80, 24, sshTerminalModes()); err != nil {
		sendTerminalMsg(conn, terminalMessage{Type: "error", Message: "pty request failed: " + err.Error()})
		return
	}
	if err := sshSession.Shell(); err != nil {
		sendTerminalMsg(conn, terminalMessage{Type: "error", Message: "shell start failed: " + err.Error()})
		return
	}

	a.activity.Log(sess.UserID, sess.Username, "ssh_login", "server", strconvI64(row.ID), map[string]string{"server": row.Name})
	_, _ = a.db.Exec(`UPDATE servers SET last_status='online', last_check_at=CURRENT_TIMESTAMP WHERE id=?`, row.ID)

	var writeMu sync.Mutex // serialize writes to the WS conn

	// SSH output -> WebSocket
	done := make(chan struct{}, 3)
	pump := func(rd interface{ Read([]byte) (int, error) }) {
		buf := make([]byte, 4096)
		for {
			n, err := rd.Read(buf)
			if n > 0 {
				sendTerminalMsgLocked(&writeMu, conn, terminalMessage{Type: "output", Data: string(buf[:n])})
			}
			if err != nil {
				done <- struct{}{}
				return
			}
		}
	}
	go pump(stdout)
	go pump(stderr)

	// session exit -> notify frontend
	go func() {
		_ = sshSession.Wait()
		done <- struct{}{}
	}()

	// WebSocket -> SSH stdin
	go func() {
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				done <- struct{}{}
				return
			}
			var msg terminalMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "input":
				_, _ = stdin.Write([]byte(msg.Data))
			case "resize":
				if msg.Cols > 0 && msg.Rows > 0 {
					_ = sshSession.WindowChange(msg.Rows, msg.Cols)
				}
			}
		}
	}()

	<-done
	sendTerminalMsgLocked(&writeMu, conn, terminalMessage{Type: "closed"})
}

// sendTerminalMsg writes a pre-session message; called only on the connect
// error path before any goroutine starts, so sequential writes are safe.
func sendTerminalMsg(conn *websocket.Conn, msg terminalMessage) {
	_ = conn.WriteJSON(msg)
}

func sendTerminalMsgLocked(mu *sync.Mutex, conn *websocket.Conn, msg terminalMessage) {
	mu.Lock()
	defer mu.Unlock()
	_ = conn.WriteJSON(msg)
}

func sshTerminalModes() ssh.TerminalModes {
	return ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
}
