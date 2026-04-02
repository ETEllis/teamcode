package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"

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

	if err := svc.StartScheduler(ctx, bootstrap.Constitution.OrganizationID); err != nil {
		log.Fatal(err)
	}

	// DB poll loop: fires WakeSignals for schedules whose next_fire_at is due.
	conn, err := db.Connect()
	if err != nil {
		log.Printf("scheduler-daemon: DB unavailable, skipping DB poll loop: %v", err)
	} else {
		q := db.New(conn)
		orgID := bootstrap.Constitution.OrganizationID
		go runDBPollScheduler(ctx, q, svc, orgID)
	}

	<-ctx.Done()
}

func runDBPollScheduler(ctx context.Context, q *db.Queries, svc *agency.Service, orgID string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			now := time.Now()
			schedules, err := q.ListDueAgencySchedules(ctx, now.UnixMilli())
			if err != nil {
				log.Printf("scheduler-daemon: ListDueAgencySchedules: %v", err)
				continue
			}
			for _, sched := range schedules {
				if !sched.AgentID.Valid || sched.AgentID.String == "" {
					continue
				}
				agentID := sched.AgentID.String
				sig := agency.WakeSignal{
					ID:             uuid.NewString(),
					OrganizationID: orgID,
					ActorID:        agentID,
					Channel:        agency.ActorChannel(agentID),
					Kind:           agency.SignalKind(sched.WakeEvent),
					Payload: map[string]string{
						"scheduleId":  sched.ID,
						"expression":  sched.CronExpr,
						"timezone":    sched.Timezone,
						"entrySource": "db_poll_scheduler",
					},
					CreatedAt: now.UnixMilli(),
				}
				if err := svc.Bus.Publish(ctx, sig); err != nil {
					log.Printf("scheduler-daemon: publish signal for agent %s: %v", agentID, err)
				}

				interval, err := agency.ParseScheduleInterval(sched.CronExpr)
				var nextFireAt sql.NullInt64
				if err == nil {
					nextFireAt = sql.NullInt64{Int64: now.Add(interval).UnixMilli(), Valid: true}
				}
				if _, err := q.UpdateAgencyScheduleFireTimes(ctx, db.UpdateAgencyScheduleFireTimesParams{
					LastFiredAt: sql.NullInt64{Int64: now.UnixMilli(), Valid: true},
					NextFireAt:  nextFireAt,
					Metadata:    sched.Metadata,
					ID:          sched.ID,
				}); err != nil {
					log.Printf("scheduler-daemon: UpdateAgencyScheduleFireTimes %s: %v", sched.ID, err)
				}
			}
		}
	}
}
