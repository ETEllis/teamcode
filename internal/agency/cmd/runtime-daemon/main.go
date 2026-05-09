package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/ETEllis/teamcode/internal/agency"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cwd, _ := os.Getwd()
	actorBinary := os.Getenv("AGENCY_ACTOR_BINARY")
	if actorBinary == "" {
		candidate := filepath.Join(cwd, "dist", "agency-actor-daemon")
		if _, err := os.Stat(candidate); err == nil {
			actorBinary = candidate
		}
	}
	bootstrap, err := agency.LoadBootstrap(cwd, os.Getenv("AGENCY_CONSTITUTION_NAME"), agency.RuntimeModeDaemonized, actorBinary)
	if err != nil {
		log.Fatal(err)
	}

	svc, err := agency.NewService(ctx, bootstrap.Config)
	if err != nil {
		log.Fatal(err)
	}
	defer svc.Shutdown(context.Background())

	if err := svc.StartRuntime(ctx, bootstrap.Constitution); err != nil {
		log.Fatal(err)
	}

	<-ctx.Done()
}
