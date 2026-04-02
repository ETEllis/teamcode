package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/ETEllis/teamcode/internal/agency"
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

	if err := svc.StartCoordinator(ctx, bootstrap.Constitution); err != nil {
		log.Fatal(err)
	}

	<-ctx.Done()
}
