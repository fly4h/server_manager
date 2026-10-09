package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"server-manager/internal/activity"
	"server-manager/internal/api"
	"server-manager/internal/auth"
	"server-manager/internal/config"
	"server-manager/internal/credential"
	"server-manager/internal/database"
	"server-manager/internal/dockerapi"
)

func main() {
	cfg := config.Load()

	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := ensureAdminUser(db, cfg); err != nil {
		log.Fatalf("ensure admin user: %v", err)
	}

	sessions := auth.NewSessionStore()

	credMgr, err := credential.NewManager(cfg.EncryptionKey, cfg.DataDir)
	if err != nil {
		log.Fatalf("init encryption: %v", err)
	}
	if cfg.EncryptionKey == "" {
		log.Printf("APP_ENCRYPTION_KEY not set — using auto-generated key stored in %s (set the env var to control it)", cfg.DataDir)
	}
	credStore := credential.NewStore(db.DB, credMgr)

	actLog := activity.NewLogger(db.DB)

	docker, err := dockerapi.New(cfg.DockerHost)
	if err != nil {
		log.Printf("docker disabled: %v", err)
		docker = nil
	}

	handler := api.New(cfg, db, sessions, credStore, actLog, docker).Router()

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("server-manager v1.0.0 listening on :%s (env=%s)", cfg.Port, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

// ensureAdminUser creates the initial admin account from env vars
// when the users table is empty.
func ensureAdminUser(db *database.DB, cfg *config.Config) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := auth.HashPassword(cfg.AdminPass)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?,?, 'admin')`, cfg.AdminUser, hash)
	if err == nil {
		log.Printf("created initial admin user %q — change the password after first login", cfg.AdminUser)
	}
	return err
}
