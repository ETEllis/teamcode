package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/ETEllis/teamcode/internal/agency"
	"github.com/ETEllis/teamcode/internal/db"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cwd, _ := os.Getwd()
	bootstrap, err := agency.LoadBootstrap(cwd, os.Getenv("AGENCY_CONSTITUTION_NAME"), agency.RuntimeModeEmbedded, "")
	if err != nil {
		log.Fatal(err)
	}

	svc, err := agency.NewService(ctx, bootstrap.Config)
	if err != nil {
		log.Fatal(err)
	}
	defer svc.Shutdown(context.Background())

	director, err := agency.NewDirectorService(agency.DirectorConfig{
		BaseDir:         bootstrap.Config.BaseDir,
		OrganizationID:  bootstrap.Constitution.OrganizationID,
		SharedWorkplace: bootstrap.Config.SharedWorkplace,
		Ledger:          svc.Ledger,
		Bus:             svc.Bus,
		Router: agency.NewModelRouter(
			agency.BuiltinProviderAdaptersForDirector(),
			agency.NewCredentialBroker(),
			agency.ExecutionPolicy{PrivacyLevel: "any", PreferLocal: true},
		),
	})
	if err != nil {
		log.Fatal(err)
	}

	// Lattice inspector (Phase 3, item #14): wire a read-only DB-backed
	// trace fetcher when the database is available. Inspector routes
	// gracefully degrade to 503 if the DB is offline so the rest of
	// the Director portal stays up.
	if conn, dbErr := db.Connect(); dbErr != nil {
		log.Printf("director-daemon: lattice inspector disabled (DB unavailable): %v", dbErr)
	} else {
		director.SetTraceFetcher(newDBGISTTraceFetcher(db.New(conn)))
	}

	addr := getenv("AGENCY_DIRECTOR_ADDR", "127.0.0.1:8765")
	token := os.Getenv("AGENCY_DIRECTOR_TOKEN")
	server := agency.NewDirectorHTTPServer(agency.DirectorHTTPConfig{Addr: addr, Token: token}, director)
	fmt.Fprintf(os.Stderr, "director-daemon: %s available at %s\n", director.Agent().Identity().Name, directorURL(server.URL(), token))

	interval := monitorInterval()
	go runMonitor(ctx, director, interval)

	if err := server.Serve(ctx); err != nil {
		log.Fatal(err)
	}
}

func runMonitor(ctx context.Context, director *agency.DirectorService, interval time.Duration) {
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = director.Monitor(ctx)
		}
	}
}

func monitorInterval() time.Duration {
	raw := os.Getenv("AGENCY_DIRECTOR_MONITOR_INTERVAL_SECONDS")
	if raw == "" {
		return 5 * time.Minute
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 5 * time.Minute
	}
	return time.Duration(seconds) * time.Second
}

func getenv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func directorURL(baseURL string, token string) string {
	if token == "" {
		return baseURL
	}
	return baseURL + "?token=" + token
}
