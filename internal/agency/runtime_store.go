package agency

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func actorSpecsDir(baseDir string) string {
	return filepath.Join(baseDir, "runtime", "actors")
}

func actorSpecPath(baseDir, actorID string) string {
	return filepath.Join(actorSpecsDir(baseDir), actorID+".json")
}

func ensureActorSpecsDir(baseDir string) error {
	if stringsTrim(baseDir) == "" {
		return nil
	}
	return os.MkdirAll(actorSpecsDir(baseDir), 0o755)
}

func writeActorSpec(baseDir string, spec ActorRuntimeSpec) error {
	if stringsTrim(baseDir) == "" || stringsTrim(spec.Identity.ID) == "" {
		return nil
	}
	if err := ensureActorSpecsDir(baseDir); err != nil {
		return err
	}
	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(actorSpecPath(baseDir, spec.Identity.ID), data, 0o644)
}

func loadActorSpecs(baseDir string) ([]ActorRuntimeSpec, error) {
	if stringsTrim(baseDir) == "" {
		return nil, nil
	}
	dir := actorSpecsDir(baseDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	specs := make([]ActorRuntimeSpec, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var spec ActorRuntimeSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("load actor spec %s: %w", path, err)
		}
		if stringsTrim(spec.Identity.ID) == "" {
			continue
		}
		specs = append(specs, spec)
	}

	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Identity.ID < specs[j].Identity.ID
	})
	return specs, nil
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}
