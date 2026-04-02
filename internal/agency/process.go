package agency

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

type ProcessHandle interface {
	PID() int
	Wait() error
	Stop(context.Context) error
}

type ProcessSpawner interface {
	Spawn(context.Context, ActorRuntimeSpec) (ProcessHandle, error)
}

type ExecProcessSpawner struct {
	BinaryPath       string
	WorkingDirectory string
	BaseDir          string
	SharedWorkplace  string
	Redis            *RedisConfig
	ConstitutionName string
}

type execProcessHandle struct {
	cmd      *exec.Cmd
	waitOnce sync.Once
	waitDone chan struct{}
	waitErr  error
}

func (h *execProcessHandle) PID() int {
	if h == nil || h.cmd == nil || h.cmd.Process == nil {
		return 0
	}
	return h.cmd.Process.Pid
}

func (h *execProcessHandle) Wait() error {
	if h == nil || h.cmd == nil {
		return nil
	}
	h.waitOnce.Do(func() {
		h.waitErr = h.cmd.Wait()
		close(h.waitDone)
	})
	<-h.waitDone
	return h.waitErr
}

func (h *execProcessHandle) Stop(ctx context.Context) error {
	if h == nil || h.cmd == nil || h.cmd.Process == nil {
		return nil
	}
	_ = h.cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan error, 1)
	go func() {
		done <- h.cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(5 * time.Second):
		_ = h.cmd.Process.Kill()
		return <-done
	}
}

func (s *ExecProcessSpawner) Spawn(ctx context.Context, spec ActorRuntimeSpec) (ProcessHandle, error) {
	if spec.Identity.ID == "" {
		return nil, fmt.Errorf("actor id is required")
	}
	if s == nil || s.BinaryPath == "" {
		return nil, fmt.Errorf("actor binary path is required")
	}

	cmd := exec.CommandContext(ctx, s.BinaryPath)
	cmd.Dir = s.WorkingDirectory
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"AGENCY_BASE_DIR="+s.BaseDir,
		"AGENCY_SHARED_WORKPLACE="+s.SharedWorkplace,
		"AGENCY_ACTOR_ID="+spec.Identity.ID,
		"AGENCY_ACTOR_SPEC_PATH="+actorSpecPath(s.BaseDir, spec.Identity.ID),
		"AGENCY_CONSTITUTION_NAME="+s.ConstitutionName,
	)
	if s.Redis != nil {
		cmd.Env = append(cmd.Env,
			"AGENCY_REDIS_ADDR="+s.Redis.Addr,
			"AGENCY_REDIS_PASSWORD="+s.Redis.Password,
			"AGENCY_REDIS_DB="+strconv.Itoa(s.Redis.DB),
		)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &execProcessHandle{
		cmd:      cmd,
		waitDone: make(chan struct{}),
	}, nil
}
