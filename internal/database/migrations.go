package database

import "embed"

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationFiles lists migration scripts in application order.
var migrationFiles = []string{"migrations/001_init.sql"}

func loadMigrations() ([]string, error) {
	out := make([]string, 0, len(migrationFiles))
	for _, name := range migrationFiles {
		b, err := migrationsFS.ReadFile(name)
		if err != nil {
			return nil, err
		}
		out = append(out, string(b))
	}
	return out, nil
}
