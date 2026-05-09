package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ETEllis/teamcode/internal/agency"
)

func main() {
	cwd, _ := os.Getwd()
	bootstrap, bootstrapErr := agency.LoadBootstrap(cwd, os.Getenv("AGENCY_CONSTITUTION_NAME"), agency.RuntimeModeEmbedded, "")

	orgID := os.Getenv("AGENCY_ORG_ID")
	if orgID == "" && bootstrapErr == nil {
		orgID = bootstrap.Constitution.OrganizationID
	}
	if orgID == "" {
		orgID = "default"
	}
	baseDir := os.Getenv("AGENCY_BASE_DIR")
	if baseDir == "" && bootstrapErr == nil {
		baseDir = bootstrap.Config.BaseDir
	}
	if baseDir == "" {
		baseDir = "."
	}
	redisAddr := os.Getenv("AGENCY_REDIS_ADDR")
	redisDB := 0
	if bootstrapErr == nil && bootstrap.Config.Redis != nil {
		if redisAddr == "" {
			redisAddr = bootstrap.Config.Redis.Addr
		}
		redisDB = bootstrap.Config.Redis.DB
	}

	var bus agency.EventBus
	if redisAddr != "" {
		bus = agency.NewRedisEventBus(agency.RedisConfig{Addr: redisAddr, DB: redisDB})
		fmt.Fprintf(os.Stderr, "ipc-server: using Redis at %s db=%d\n", redisAddr, redisDB)
	} else {
		bus = agency.NewMemoryEventBus()
		fmt.Fprintf(os.Stderr, "ipc-server: using in-memory bus (no Redis)\n")
	}
	defer bus.Close(context.Background())

	socketPath := agency.IPCSocketPath(baseDir, orgID)
	srv := agency.NewIPCServer(socketPath, bus)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, "ipc-server: shutting down")
		cancel()
	}()

	fmt.Fprintf(os.Stderr, "ipc-server: listening on %s (orgId=%s)\n", socketPath, orgID)
	if err := srv.Serve(ctx); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "ipc-server: %v\n", err)
		os.Exit(1)
	}
}
