// Package main provides a throwaway SSH server used to smoke-test the
// terminal/command paths without a real remote box.
//
//	go run ./tools/fake-ssh serve            # listens on 127.0.0.1:2222 (user/pass: test/test)
//	go run ./tools/fake-ssh ws <ws-url>      # opens a WebSocket, prints received messages
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: fake-ssh serve | ws <url>")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		serve()
	case "ws":
		if len(os.Args) < 3 {
			log.Fatal("ws requires a url argument")
		}
		wsClient(os.Args[2])
	default:
		fmt.Println("unknown mode", os.Args[1])
		os.Exit(2)
	}
}

func serve() {
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "test" && string(pass) == "test" {
				return nil, nil
			}
			return nil, fmt.Errorf("auth denied")
		},
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		log.Fatal(err)
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:2222")
	if err != nil {
		log.Fatal(err)
	}
	log.Println("fake ssh listening on 127.0.0.1:2222 (test/test)")

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleConn(conn, cfg)
	}
}

func handleConn(conn net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sc.Close()
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "only session supported")
			continue
		}
		ch, chReqs, err := newChan.Accept()
		if err != nil {
			continue
		}
		go handleSession(ch, chReqs)
	}
}

func handleSession(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()

	var (
		isPTY bool
		cmd   string
	)

	for req := range reqs {
		switch req.Type {
		case "pty-req":
			isPTY = true
			_ = req.Reply(true, nil)
		case "window-change":
			_ = req.Reply(true, nil)
		case "shell":
			_ = req.Reply(true, nil)
			// Keep draining control requests in the background while serving
			// the interactive loop inline — returning here would close the channel.
			go func() {
				for pk := range reqs {
					if pk.Type == "pty-req" || pk.Type == "window-change" {
						_ = pk.Reply(true, nil)
						continue
					}
					_ = pk.Reply(false, nil)
				}
			}()
			echoLoop(ch, isPTY)
			return
		case "exec":
			// payload: 4-byte length prefix + command string
			if len(req.Payload) > 4 {
				cmd = string(req.Payload[4:])
			}
			_ = req.Reply(true, nil)
			fmt.Fprintf(ch, "fake-ssh exec: %s\n", cmd)
			_, _ = ch.SendRequest("exit-status", false, exitStatus(0))
			return
		default:
			_ = req.Reply(false, nil)
		}
	}
}

func echoLoop(ch ssh.Channel, isPTY bool) {
	if isPTY {
		fmt.Fprintf(ch, "fake-ssh shell ready\r\n$ ")
	} else {
		fmt.Fprintf(ch, "fake-ssh shell ready\n$ ")
	}
	buf := make([]byte, 1024)
	for {
		n, err := ch.Read(buf)
		if n > 0 {
			fmt.Fprintf(ch, "echo:%s", string(buf[:n]))
		}
		if err != nil {
			return
		}
	}
}

func exitStatus(code uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, code)
	return b
}

// wsClient connects to the terminal WebSocket endpoint and prints what it gets.
func wsClient(rawURL string) {
	header := http.Header{}
	if c := os.Getenv("SM_COOKIE"); c != "" {
		header.Set("Cookie", "sm_session="+c)
	}
	conn, resp, err := websocket.DefaultDialer.Dial(rawURL, header)
	if err != nil {
		if resp != nil {
			log.Fatalf("dial: %v (http %d)", err, resp.StatusCode)
		}
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	fmt.Println("WS_CONNECTED")
	if err := conn.WriteJSON(map[string]any{"type": "resize", "cols": 80, "rows": 24}); err != nil {
		log.Fatal(err)
	}
	if err := conn.WriteJSON(map[string]any{"type": "input", "data": "hello\n"}); err != nil {
		log.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := 0; i < 4; i++ {
		_, data, err := conn.ReadMessage()
		if err != nil {
			fmt.Println("WS_READ_ERR", err)
			return
		}
		fmt.Printf("WS_MSG %q\n", string(data))
	}
	fmt.Println("WS_DONE")
}
