package config

import (
	"os"
	"path/filepath"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	Env           string // dev | prod
	Port          string
	DataDir       string
	DatabasePath  string
	EncryptionKey string // master key for AES-256-GCM (env or auto-generated in data dir)
	SessionSecret string
	AdminUser     string
	AdminPass     string
	DockerHost    string // e.g. unix:///var/run/docker.sock
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Load reads configuration from environment variables with sensible defaults.
func Load() *Config {
	dataDir := getenv("APP_DATA_DIR", "./data")
	return &Config{
		Env:           getenv("APP_ENV", "dev"),
		Port:          getenv("APP_PORT", "8080"),
		DataDir:       dataDir,
		DatabasePath:  getenv("APP_DATABASE", filepath.Join(dataDir, "app.db")),
		EncryptionKey: os.Getenv("APP_ENCRYPTION_KEY"),
		SessionSecret: os.Getenv("APP_SESSION_SECRET"),
		AdminUser:     getenv("APP_ADMIN_USERNAME", "admin"),
		AdminPass:     getenv("APP_ADMIN_PASSWORD", "admin123"),
		DockerHost:    getenv("DOCKER_HOST", "unix:///var/run/docker.sock"),
	}
}

// IsProd reports whether the app runs in production mode.
func (c *Config) IsProd() bool { return c.Env == "prod" }
