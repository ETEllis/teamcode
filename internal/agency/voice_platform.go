package agency

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// KokoroVoiceMap maps agent role archetypes to Kokoro voice IDs.
var KokoroVoiceMap = map[string]string{
	"coordinator": "am_adam",
	"architect":   "bm_daniel",
	"developer":   "am_michael",
	"analyst":     "bf_emma",
	"scheduler":   "af_bella",
	"reviewer":    "bm_george",
	"default":     "af_heart",
}

// PlatformTTSCommand returns the best available TTS command for the current platform.
// Priority: Kokoro Python wrapper > macOS say fallback.
func PlatformTTSCommand(baseDir string) (command string, args []string, available bool) {
	kokoroScript := filepath.Join(baseDir, "voice", "kokoro-tts.py")
	if python := findPython(); python != "" {
		return python, []string{kokoroScript, "--voice", "{voice}", "--output", "{output}", "--text", "{text}"}, true
	}
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("say"); err == nil {
			return "say", []string{"-v", "Samantha", "-o", "{output}", "--data-format=LEF32@22050", "{text}"}, true
		}
	}
	return "", nil, false
}

func findPython() string {
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// VoiceIDForRole returns the Kokoro voice ID for a given agent role name.
func VoiceIDForRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	if v, ok := KokoroVoiceMap[role]; ok {
		return v
	}
	return KokoroVoiceMap["default"]
}

// KokoroProsodyRate returns the speech rate multiplier for a given signal kind.
func KokoroProsodyRate(signalKind string) string {
	switch signalKind {
	case "broadcast":
		return "1.1"
	case "error", "rejected":
		return "0.9"
	case "handoff":
		return "0.95"
	default:
		return "1.0"
	}
}

// TTSNotInstalledMsg returns the install hint message.
func TTSNotInstalledMsg() string {
	return fmt.Sprintf("Voice not installed. Run: scripts/install-voice (platform: %s/%s)", runtime.GOOS, runtime.GOARCH)
}
