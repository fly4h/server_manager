package sshclient

import (
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"
)

// Target describes an SSH endpoint plus how to authenticate.
type Target struct {
	ID       int64
	Name     string
	Host     string
	Port     int
	Username string
	AuthType string // password | private_key
	Secret   string // password or PEM private key (decrypted in-memory only)
}

// Dial opens an SSH connection to the target.
func Dial(t Target) (*ssh.Client, error) {
	if t.Port == 0 {
		t.Port = 22
	}
	var authMethod ssh.AuthMethod
	switch t.AuthType {
	case "password":
		authMethod = ssh.Password(t.Secret)
	case "private_key":
		signer, err := ssh.ParsePrivateKey([]byte(t.Secret))
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		authMethod = ssh.PublicKeys(signer)
	default:
		return nil, fmt.Errorf("unsupported auth type %q", t.AuthType)
	}
	cfg := &ssh.ClientConfig{
		User:            t.Username,
		Auth:            []ssh.AuthMethod{authMethod},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // personal tool, TOFU is out of V1 scope
		Timeout:         10 * time.Second,
	}
	return ssh.Dial("tcp", net.JoinHostPort(t.Host, fmt.Sprintf("%d", t.Port)), cfg)
}

// Test verifies reachability + auth, then closes the connection.
func Test(t Target) error {
	client, err := Dial(t)
	if err != nil {
		return err
	}
	return client.Close()
}

// Ping does a cheap TCP dial to check the SSH port — used by the dashboard
// for fast online/offline status without full handshakes.
func Ping(host string, port int) bool {
	if port == 0 {
		port = 22
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprintf("%d", port)), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Run executes one command over a fresh connection and returns combined output.
func Run(ctx context.Context, t Target, command string) (string, error) {
	client, err := Dial(t)
	if err != nil {
		return "", fmt.Errorf("ssh connect: %w", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer sess.Close()

	type res struct {
		out []byte
		err error
	}
	done := make(chan res, 1)
	go func() {
		out, err := sess.CombinedOutput(command)
		done <- res{out, err}
	}()
	select {
	case <-ctx.Done():
		_ = sess.Signal(ssh.SIGKILL)
		return "", ctx.Err()
	case r := <-done:
		return string(r.out), r.err
	}
}

// NewSession is used by the terminal WebSocket handler, which manages the
// session lifecycle itself.
func NewSession(t Target) (*ssh.Client, *ssh.Session, error) {
	client, err := Dial(t)
	if err != nil {
		return nil, nil, err
	}
	sess, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, nil, err
	}
	return client, sess, nil
}
