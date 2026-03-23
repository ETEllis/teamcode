package team

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/ETEllis/teamcode/internal/config"
)

var invalidTeamNameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

type store struct {
	baseDir string
	mu      sync.Mutex
}

func newStore(baseDir string) *store {
	if baseDir == "" {
		baseDir = defaultBaseDir()
	}
	return &store{baseDir: baseDir}
}

func defaultBaseDir() string {
	if cfg := config.Get(); cfg != nil && cfg.Data.Directory != "" {
		return filepath.Join(cfg.Data.Directory, "teams")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return filepath.Join(os.TempDir(), "teamcode", "teams")
	}
	return filepath.Join(cwd, ".teamcode", "teams")
}

func normalizeName(name string) string {
	name = strings.TrimSpace(name)
	name = invalidTeamNameChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	if name == "" {
		return "default"
	}
	return strings.ToLower(name)
}

func (s *store) teamDir(teamName string) string {
	return filepath.Join(s.baseDir, normalizeName(teamName))
}

func (s *store) ensureTeamDir(teamName string) error {
	return os.MkdirAll(s.teamDir(teamName), 0o755)
}

func (s *store) readJSON(teamName, filename string, target any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.teamDir(teamName), filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, target)
}

func (s *store) writeJSON(teamName, filename string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureTeamDir(teamName); err != nil {
		return err
	}

	path := filepath.Join(s.teamDir(teamName), filename)
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filename, err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func (s *store) inboxDir(teamName string) string {
	return filepath.Join(s.teamDir(teamName), "inboxes")
}

func (s *store) readInbox(teamName, agentName string) ([]InboxMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.inboxDir(teamName), normalizeName(agentName)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var messages []InboxMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (s *store) writeInbox(teamName, agentName string, messages []InboxMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureTeamDir(teamName); err != nil {
		return err
	}
	if err := os.MkdirAll(s.inboxDir(teamName), 0o755); err != nil {
		return err
	}

	path := filepath.Join(s.inboxDir(teamName), normalizeName(agentName)+".json")
	data, err := json.MarshalIndent(messages, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
