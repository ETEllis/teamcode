package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"

	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/logging"

	"github.com/pressly/goose/v3"
)

func Connect() (*sql.DB, error) {
	dbPath, err := resolveDBPath()
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Verify connection
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Set pragmas for better performance
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA page_size = 4096;",
		"PRAGMA cache_size = -8000;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA synchronous = NORMAL;",
	}

	for _, pragma := range pragmas {
		if _, err = db.Exec(pragma); err != nil {
			logging.Error("Failed to set pragma", pragma, err)
		} else {
			logging.Debug("Set pragma", "pragma", pragma)
		}
	}

	goose.SetBaseFS(FS)

	if err := goose.SetDialect("sqlite3"); err != nil {
		logging.Error("Failed to set dialect", "error", err)
		return nil, fmt.Errorf("failed to set dialect: %w", err)
	}

	if err := goose.Up(db, "migrations"); err != nil {
		logging.Error("Failed to apply migrations", "error", err)
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}
	return db, nil
}

func resolveDBPath() (string, error) {
	if explicit := os.Getenv("AGENCY_DB_PATH"); explicit != "" {
		if err := os.MkdirAll(filepath.Dir(explicit), 0o700); err != nil {
			return "", fmt.Errorf("failed to create agency db directory: %w", err)
		}
		return explicit, nil
	}

	if officeDir := os.Getenv("AGENCY_OFFICE_DIR"); officeDir != "" {
		if err := os.MkdirAll(officeDir, 0o700); err != nil {
			return "", fmt.Errorf("failed to create agency office directory: %w", err)
		}
		return filepath.Join(officeDir, "agency.db"), nil
	}

	dataDir := config.Get().Data.Directory
	if dataDir == "" {
		return "", fmt.Errorf("data.dir is not set")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create data directory: %w", err)
	}

	dbFilename := "teamcode.db"
	legacyDBPath := filepath.Join(dataDir, "opencode.db")
	if _, err := os.Stat(filepath.Join(dataDir, dbFilename)); os.IsNotExist(err) {
		if _, legacyErr := os.Stat(legacyDBPath); legacyErr == nil {
			dbFilename = "opencode.db"
		}
	}
	return filepath.Join(dataDir, dbFilename), nil
}
