package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ETEllis/teamcode/internal/config"
	"github.com/ETEllis/teamcode/internal/llm/models"
	"github.com/ETEllis/teamcode/internal/logging"
)

func GetAgentPrompt(agentName config.AgentName, provider models.ModelProvider) string {
	basePrompt := ""
	switch agentName {
	case config.AgentCoder:
		basePrompt = CoderPrompt(provider)
	case config.AgentTitle:
		basePrompt = TitlePrompt(provider)
	case config.AgentTask:
		basePrompt = TaskPrompt(provider)
	case config.AgentSummarizer:
		basePrompt = SummarizerPrompt(provider)
	default:
		basePrompt = "You are a helpful assistant"
	}

	if agentName == config.AgentCoder || agentName == config.AgentTask {
		// Add context from project-specific instruction files if they exist
		contextContent := getContextFromPaths()
		logging.Debug("Context content", "Context", contextContent)
		if contextContent != "" {
			return fmt.Sprintf("%s\n\n# Project-Specific Context\n Make sure to follow the instructions in the context below\n%s", basePrompt, contextContent)
		}
	}
	return basePrompt
}

var (
	onceContext    sync.Once
	contextContent string
)

func getContextFromPaths() string {
	onceContext.Do(func() {
		var (
			cfg          = config.Get()
			workDir      = cfg.WorkingDir
			contextPaths = cfg.ContextPaths
		)

		contextContent = processContextPaths(workDir, contextPaths)
	})

	return contextContent
}

func processContextPaths(workDir string, paths []string) string {
	var wg sync.WaitGroup

	// Per-path result slices preserve user-specified order. Within a directory
	// walk, filepath.WalkDir already returns entries in lexical order, so the
	// concatenated output is fully deterministic across runs.
	pathResults := make([][]string, len(paths))

	// Track processed files to avoid duplicates (case-insensitive). When two
	// paths refer to the same underlying file, the goroutine that wins the
	// dedupe check captures it; user-specified order in the join below makes
	// the final output stable for non-overlapping path sets.
	processedFiles := make(map[string]bool)
	var processedMutex sync.Mutex

	for i, path := range paths {
		wg.Add(1)
		go func(idx int, p string) {
			defer wg.Done()
			var local []string

			if strings.HasSuffix(p, "/") {
				filepath.WalkDir(filepath.Join(workDir, p), func(walkPath string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					processedMutex.Lock()
					lowerPath := strings.ToLower(walkPath)
					if processedFiles[lowerPath] {
						processedMutex.Unlock()
						return nil
					}
					processedFiles[lowerPath] = true
					processedMutex.Unlock()

					if result := processFile(walkPath); result != "" {
						local = append(local, result)
					}
					return nil
				})
			} else {
				fullPath := filepath.Join(workDir, p)
				processedMutex.Lock()
				lowerPath := strings.ToLower(fullPath)
				if !processedFiles[lowerPath] {
					processedFiles[lowerPath] = true
					processedMutex.Unlock()
					if result := processFile(fullPath); result != "" {
						local = append(local, result)
					}
				} else {
					processedMutex.Unlock()
				}
			}

			pathResults[idx] = local
		}(i, path)
	}

	wg.Wait()

	totalLen := 0
	for _, sub := range pathResults {
		totalLen += len(sub)
	}
	results := make([]string, 0, totalLen)
	for _, sub := range pathResults {
		results = append(results, sub...)
	}

	return strings.Join(results, "\n")
}

func processFile(filePath string) string {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return "# From:" + filePath + "\n" + string(content)
}
