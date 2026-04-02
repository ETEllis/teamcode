package agency

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type VoiceGatewayConfig struct {
	Enabled              bool
	Provider             string
	StatePath            string
	AssetDir             string
	ControlChannel       string
	SynthesisChannel     string
	MeetingTranscriptDir string
	DefaultRoom          string
	Projection           VoiceProjectionDefaults
	STT                  SpeechRuntimeConfig
	TTS                  SpeechRuntimeConfig
}

type VoiceProjectionDefaults struct {
	TranscriptProjection  bool
	AudioProjection       bool
	AutoProjectTranscript bool
	AutoProjectSynthesis  bool
}

type SpeechRuntimeConfig struct {
	Enabled     bool
	Command     string
	Args        []string
	InputMode   string
	OutputMode  string
	Language    string
	Voice       string
	AudioFormat string
	Timeout     time.Duration
}

type VoiceProjectionRequest struct {
	OrganizationID       string
	RoomID               string
	ProjectionEnabled    bool
	TranscriptProjection *bool
	AudioProjection      *bool
	Metadata             map[string]string
}

type VoiceTranscriptRequest struct {
	OrganizationID string
	ActorID        string
	RoomID         string
	Text           string
	AudioPath      string
	Metadata       map[string]string
}

type VoiceSynthesisRequest struct {
	OrganizationID string
	ActorID        string
	RoomID         string
	Text           string
	OutputPath     string
	Metadata       map[string]string
}

type VoiceTranscriptResult struct {
	Event       VoiceEvent        `json:"event"`
	Certificate CommitCertificate `json:"certificate"`
	Room        VoiceRoomState    `json:"room"`
}

type VoiceSynthesisResult struct {
	Event       VoiceEvent        `json:"event"`
	Certificate CommitCertificate `json:"certificate"`
	Room        VoiceRoomState    `json:"room"`
}

type VoiceGatewayStatus struct {
	Enabled              bool             `json:"enabled"`
	Provider             string           `json:"provider,omitempty"`
	OrganizationID       string           `json:"organizationId"`
	StatePath            string           `json:"statePath"`
	AssetDir             string           `json:"assetDir"`
	ControlChannel       string           `json:"controlChannel"`
	SynthesisChannel     string           `json:"synthesisChannel"`
	MeetingTranscriptDir string           `json:"meetingTranscriptDir"`
	DefaultRoom          string           `json:"defaultRoom"`
	STTConfigured        bool             `json:"sttConfigured"`
	TTSConfigured        bool             `json:"ttsConfigured"`
	Rooms                []VoiceRoomState `json:"rooms,omitempty"`
	LastVoiceEvent       *VoiceEvent      `json:"lastVoiceEvent,omitempty"`
}

type commandExecutor interface {
	Run(context.Context, string, []string, []byte) ([]byte, error)
}

type execCommandExecutor struct{}

func (e execCommandExecutor) Run(ctx context.Context, command string, args []string, stdin []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Stdin = bytes.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %v: %w: %s", command, args, err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

type speechToTextEngine interface {
	Available() bool
	Transcribe(context.Context, string) (string, string, error)
}

type textToSpeechEngine interface {
	Available() bool
	Synthesize(context.Context, string, string) (string, string, error)
}

type localCommandSTT struct {
	cfg      SpeechRuntimeConfig
	executor commandExecutor
}

func (s localCommandSTT) Available() bool {
	return s.cfg.Enabled && strings.TrimSpace(s.cfg.Command) != ""
}

func (s localCommandSTT) Transcribe(ctx context.Context, audioPath string) (string, string, error) {
	if !s.Available() {
		return "", "", fmt.Errorf("stt engine is not configured")
	}
	args := expandSpeechArgs(s.cfg.Args, map[string]string{
		"input":    audioPath,
		"language": s.cfg.Language,
		"voice":    s.cfg.Voice,
	})
	runCtx := withSpeechTimeout(ctx, s.cfg.Timeout)
	output, err := s.executor.Run(runCtx, s.cfg.Command, args, nil)
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(string(output)), s.cfg.Command, nil
}

type localCommandTTS struct {
	cfg      SpeechRuntimeConfig
	executor commandExecutor
}

func (s localCommandTTS) Available() bool {
	return s.cfg.Enabled && strings.TrimSpace(s.cfg.Command) != ""
}

func (s localCommandTTS) Synthesize(ctx context.Context, text, outputPath string) (string, string, error) {
	if !s.Available() {
		return "", "", fmt.Errorf("tts engine is not configured")
	}
	args := expandSpeechArgs(s.cfg.Args, map[string]string{
		"output":   outputPath,
		"text":     text,
		"language": s.cfg.Language,
		"voice":    s.cfg.Voice,
	})
	var stdin []byte
	if !containsPlaceholder(s.cfg.Args, "{text}") {
		stdin = []byte(text)
	}
	runCtx := withSpeechTimeout(ctx, s.cfg.Timeout)
	if _, err := s.executor.Run(runCtx, s.cfg.Command, args, stdin); err != nil {
		return "", "", err
	}
	return outputPath, s.cfg.Command, nil
}

type VoiceGateway struct {
	cfg      VoiceGatewayConfig
	ledger   *LedgerService
	bus      EventBus
	executor commandExecutor
	stt      speechToTextEngine
	tts      textToSpeechEngine
}

func NewVoiceGateway(cfg VoiceGatewayConfig, ledger *LedgerService, bus EventBus) *VoiceGateway {
	executor := execCommandExecutor{}
	return &VoiceGateway{
		cfg:      cfg,
		ledger:   ledger,
		bus:      bus,
		executor: executor,
		stt:      localCommandSTT{cfg: cfg.STT, executor: executor},
		tts:      localCommandTTS{cfg: cfg.TTS, executor: executor},
	}
}

func (g *VoiceGateway) Status(ctx context.Context, organizationID string) (VoiceGatewayStatus, error) {
	if g == nil || g.ledger == nil {
		return VoiceGatewayStatus{}, fmt.Errorf("voice gateway is not configured")
	}
	snapshot, err := g.ledger.LatestSnapshot(ctx, organizationID)
	if err != nil {
		return VoiceGatewayStatus{}, err
	}
	status := VoiceGatewayStatus{
		Enabled:              g.cfg.Enabled,
		Provider:             g.cfg.Provider,
		OrganizationID:       organizationID,
		StatePath:            g.cfg.StatePath,
		AssetDir:             g.cfg.AssetDir,
		ControlChannel:       g.controlChannel(organizationID),
		SynthesisChannel:     g.synthesisChannel(organizationID),
		MeetingTranscriptDir: g.meetingTranscriptDir(organizationID),
		DefaultRoom:          g.cfg.DefaultRoom,
		STTConfigured:        g.stt != nil && g.stt.Available(),
		TTSConfigured:        g.tts != nil && g.tts.Available(),
		Rooms:                snapshot.VoiceRooms,
		LastVoiceEvent:       snapshot.LastVoiceEvent,
	}
	if err := g.persistStatus(status); err != nil {
		return VoiceGatewayStatus{}, err
	}
	return status, nil
}

func (g *VoiceGateway) SetProjection(ctx context.Context, req VoiceProjectionRequest) (VoiceRoomState, error) {
	if g == nil || g.ledger == nil {
		return VoiceRoomState{}, fmt.Errorf("voice gateway is not configured")
	}
	room := g.roomState(ctx, req.OrganizationID, req.RoomID)
	room.ProjectionEnabled = req.ProjectionEnabled
	if req.TranscriptProjection != nil {
		room.TranscriptProjection = *req.TranscriptProjection
	}
	if req.AudioProjection != nil {
		room.AudioProjection = *req.AudioProjection
	}
	room.UpdatedAt = time.Now().UnixMilli()
	if room.Metadata == nil {
		room.Metadata = map[string]string{}
	}
	for key, value := range req.Metadata {
		room.Metadata[key] = value
	}

	event := VoiceEvent{
		ID:             uuid.NewString(),
		OrganizationID: req.OrganizationID,
		RoomID:         room.RoomID,
		Kind:           VoiceEventProjection,
		Projection:     &room,
		Metadata:       cloneMap(req.Metadata),
		CreatedAt:      room.UpdatedAt,
	}
	if _, err := g.ledger.AppendVoiceEvent(ctx, event); err != nil {
		return VoiceRoomState{}, err
	}
	_ = g.publishVoiceSignal(ctx, req.OrganizationID, room.RoomID, SignalProjection, event)
	return room, nil
}

func (g *VoiceGateway) IngestTranscript(ctx context.Context, req VoiceTranscriptRequest) (VoiceTranscriptResult, error) {
	if g == nil || g.ledger == nil {
		return VoiceTranscriptResult{}, fmt.Errorf("voice gateway is not configured")
	}
	canonicalText := strings.TrimSpace(req.Text)
	engineName := ""
	if canonicalText == "" {
		if strings.TrimSpace(req.AudioPath) == "" {
			return VoiceTranscriptResult{}, fmt.Errorf("either text or audio path is required")
		}
		transcript, engine, err := g.stt.Transcribe(ctx, req.AudioPath)
		if err != nil {
			return VoiceTranscriptResult{}, err
		}
		canonicalText = strings.TrimSpace(transcript)
		engineName = engine
	}
	if canonicalText == "" {
		return VoiceTranscriptResult{}, fmt.Errorf("canonical transcript is required")
	}

	room := g.roomState(ctx, req.OrganizationID, req.RoomID)
	event := VoiceEvent{
		ID:             uuid.NewString(),
		OrganizationID: req.OrganizationID,
		ActorID:        req.ActorID,
		RoomID:         room.RoomID,
		Kind:           VoiceEventTranscript,
		CanonicalText:  canonicalText,
		AudioInputRef:  req.AudioPath,
		Engine:         engineName,
		Metadata:       cloneMap(req.Metadata),
		CreatedAt:      time.Now().UnixMilli(),
	}
	if transcriptRef, err := g.persistTranscript(event); err != nil {
		return VoiceTranscriptResult{}, err
	} else if transcriptRef != "" {
		if event.Metadata == nil {
			event.Metadata = map[string]string{}
		}
		event.Metadata["transcriptRef"] = transcriptRef
	}
	cert, err := g.ledger.AppendVoiceEvent(ctx, event)
	if err != nil {
		return VoiceTranscriptResult{}, err
	}
	room.LastTranscriptID = event.ID
	room.UpdatedAt = event.CreatedAt
	if room.ProjectionEnabled && room.TranscriptProjection {
		_ = g.publishVoiceSignal(ctx, req.OrganizationID, room.RoomID, SignalVoice, event)
	}
	return VoiceTranscriptResult{Event: event, Certificate: cert, Room: room}, nil
}

func (g *VoiceGateway) Synthesize(ctx context.Context, req VoiceSynthesisRequest) (VoiceSynthesisResult, error) {
	if g == nil || g.ledger == nil {
		return VoiceSynthesisResult{}, fmt.Errorf("voice gateway is not configured")
	}
	canonicalText := strings.TrimSpace(req.Text)
	if canonicalText == "" {
		return VoiceSynthesisResult{}, fmt.Errorf("text is required")
	}
	if g.tts == nil || !g.tts.Available() {
		return VoiceSynthesisResult{}, fmt.Errorf("tts engine is not configured")
	}
	outputPath := req.OutputPath
	if outputPath == "" {
		outputPath = filepath.Join(g.cfg.AssetDir, uuid.NewString()+"."+defaultExt(g.cfg.TTS.AudioFormat))
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return VoiceSynthesisResult{}, err
	}
	audioRef, engineName, err := g.tts.Synthesize(ctx, canonicalText, outputPath)
	if err != nil {
		return VoiceSynthesisResult{}, err
	}
	room := g.roomState(ctx, req.OrganizationID, req.RoomID)
	event := VoiceEvent{
		ID:             uuid.NewString(),
		OrganizationID: req.OrganizationID,
		ActorID:        req.ActorID,
		RoomID:         room.RoomID,
		Kind:           VoiceEventSynthesis,
		CanonicalText:  canonicalText,
		AudioOutputRef: audioRef,
		Engine:         engineName,
		Metadata:       cloneMap(req.Metadata),
		CreatedAt:      time.Now().UnixMilli(),
	}
	cert, err := g.ledger.AppendVoiceEvent(ctx, event)
	if err != nil {
		return VoiceSynthesisResult{}, err
	}
	room.LastSynthesisID = event.ID
	room.UpdatedAt = event.CreatedAt
	if room.ProjectionEnabled && room.AudioProjection {
		_ = g.publishVoiceSignal(ctx, req.OrganizationID, room.RoomID, SignalVoice, event)
	}
	_ = g.publishSynthesisSignal(ctx, req.OrganizationID, room.RoomID, event)
	return VoiceSynthesisResult{Event: event, Certificate: cert, Room: room}, nil
}

func (g *VoiceGateway) Serve(ctx context.Context, organizationID string) error {
	if g == nil || g.bus == nil {
		return fmt.Errorf("voice gateway bus is not configured")
	}
	status, err := g.Status(ctx, organizationID)
	if err != nil {
		return err
	}
	if err := g.persistStatus(status); err != nil {
		return err
	}
	controlCh, err := g.bus.Subscribe(ctx, g.controlChannel(organizationID))
	if err != nil {
		return err
	}
	orgCh, err := g.bus.Subscribe(ctx, OrganizationChannel(organizationID))
	if err != nil {
		return err
	}
	synthesisCh, err := g.bus.Subscribe(ctx, g.synthesisChannel(organizationID))
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, ok := <-controlCh:
			if !ok {
				return nil
			}
			status, err := g.Status(ctx, organizationID)
			if err == nil {
				_ = g.persistStatus(status)
			}
		case sig, ok := <-orgCh:
			if !ok {
				return nil
			}
			if sig.Kind != SignalVoice && sig.Kind != SignalProjection {
				continue
			}
			status, err := g.Status(ctx, organizationID)
			if err == nil {
				status.LastVoiceEvent = status.LastVoiceEvent
				_ = g.persistStatus(status)
			}
		case _, ok := <-synthesisCh:
			if !ok {
				return nil
			}
			status, err := g.Status(ctx, organizationID)
			if err == nil {
				_ = g.persistStatus(status)
			}
		}
	}
}

func (g *VoiceGateway) publishVoiceSignal(ctx context.Context, organizationID, roomID string, kind SignalKind, event VoiceEvent) error {
	if g.bus == nil {
		return nil
	}
	payload := map[string]string{
		"voiceEventId": event.ID,
		"voiceKind":    string(event.Kind),
		"roomId":       roomID,
	}
	if event.CanonicalText != "" {
		payload["canonicalText"] = event.CanonicalText
	}
	return g.bus.Publish(ctx, WakeSignal{
		ID:             event.ID,
		OrganizationID: organizationID,
		Channel:        g.controlChannel(organizationID),
		Kind:           kind,
		Payload:        payload,
		CreatedAt:      event.CreatedAt,
	})
}

func (g *VoiceGateway) publishSynthesisSignal(ctx context.Context, organizationID, roomID string, event VoiceEvent) error {
	if g.bus == nil {
		return nil
	}
	payload := map[string]string{
		"voiceEventId": event.ID,
		"voiceKind":    string(event.Kind),
		"roomId":       roomID,
	}
	if event.AudioOutputRef != "" {
		payload["audioOutputRef"] = event.AudioOutputRef
	}
	if event.CanonicalText != "" {
		payload["canonicalText"] = event.CanonicalText
	}
	return g.bus.Publish(ctx, WakeSignal{
		ID:             event.ID,
		OrganizationID: organizationID,
		Channel:        g.synthesisChannel(organizationID),
		Kind:           SignalVoice,
		Payload:        payload,
		CreatedAt:      event.CreatedAt,
	})
}

func (g *VoiceGateway) roomState(ctx context.Context, organizationID, roomID string) VoiceRoomState {
	if strings.TrimSpace(roomID) == "" {
		roomID = g.cfg.DefaultRoom
	}
	state := VoiceRoomState{
		RoomID:               roomID,
		ProjectionEnabled:    true,
		TranscriptProjection: g.cfg.Projection.TranscriptProjection,
		AudioProjection:      g.cfg.Projection.AudioProjection,
		UpdatedAt:            time.Now().UnixMilli(),
	}
	if g.ledger == nil {
		return state
	}
	snapshot, err := g.ledger.LatestSnapshot(ctx, organizationID)
	if err != nil || snapshot == nil {
		return state
	}
	for _, room := range snapshot.VoiceRooms {
		if room.RoomID == roomID {
			return room
		}
	}
	return state
}

func (g *VoiceGateway) persistStatus(status VoiceGatewayStatus) error {
	if strings.TrimSpace(g.cfg.StatePath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(g.cfg.StatePath), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(g.cfg.StatePath, payload, 0o644)
}

func (g *VoiceGateway) controlChannel(organizationID string) string {
	base := strings.TrimSpace(g.cfg.ControlChannel)
	if base == "" {
		base = "agency.voice.control"
	}
	if strings.TrimSpace(organizationID) == "" {
		return base
	}
	return base + "." + organizationID
}

func (g *VoiceGateway) synthesisChannel(organizationID string) string {
	base := strings.TrimSpace(g.cfg.SynthesisChannel)
	if base == "" {
		base = "agency.voice.synthesis"
	}
	if strings.TrimSpace(organizationID) == "" {
		return base
	}
	return base + "." + organizationID
}

func (g *VoiceGateway) meetingTranscriptDir(organizationID string) string {
	base := strings.TrimSpace(g.cfg.MeetingTranscriptDir)
	if base == "" {
		base = filepath.Join(g.cfg.AssetDir, "transcripts")
	}
	if strings.TrimSpace(organizationID) == "" {
		return base
	}
	return filepath.Join(base, organizationID)
}

func (g *VoiceGateway) persistTranscript(event VoiceEvent) (string, error) {
	if strings.TrimSpace(event.CanonicalText) == "" {
		return "", nil
	}
	dir := g.meetingTranscriptDir(event.OrganizationID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	filename := event.ID + ".txt"
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(event.CanonicalText+"\n"), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func expandSpeechArgs(args []string, replacements map[string]string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		replaced := arg
		for key, value := range replacements {
			replaced = strings.ReplaceAll(replaced, "{"+key+"}", value)
		}
		out = append(out, replaced)
	}
	return out
}

func containsPlaceholder(args []string, placeholder string) bool {
	for _, arg := range args {
		if strings.Contains(arg, placeholder) {
			return true
		}
	}
	return false
}

func withSpeechTimeout(ctx context.Context, timeout time.Duration) context.Context {
	if timeout <= 0 {
		return ctx
	}
	runCtx, _ := context.WithTimeout(ctx, timeout)
	return runCtx
}

func cloneMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func defaultExt(audioFormat string) string {
	if strings.TrimSpace(audioFormat) == "" {
		return "wav"
	}
	return audioFormat
}
