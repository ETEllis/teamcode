package agency

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type fakeSTT struct {
	text string
}

func (f fakeSTT) Available() bool { return true }

func (f fakeSTT) Transcribe(context.Context, string) (string, string, error) {
	return f.text, "fake-stt", nil
}

type fakeTTS struct{}

func (f fakeTTS) Available() bool { return true }

func (f fakeTTS) Synthesize(_ context.Context, _ string, outputPath string) (string, string, error) {
	return outputPath, "fake-tts", nil
}

func TestVoiceGatewayProjectionAndTranscriptFlow(t *testing.T) {
	t.Parallel()

	ledger, err := NewLedgerService(t.TempDir())
	require.NoError(t, err)

	gateway := NewVoiceGateway(VoiceGatewayConfig{
		Enabled:              true,
		Provider:             "local",
		StatePath:            t.TempDir() + "/voice-state.json",
		AssetDir:             t.TempDir() + "/voice-assets",
		ControlChannel:       "agency.voice.control",
		SynthesisChannel:     "agency.voice.synthesis",
		MeetingTranscriptDir: filepath.Join(t.TempDir(), "transcripts"),
		DefaultRoom:          "strategy-room",
		Projection: VoiceProjectionDefaults{
			TranscriptProjection:  true,
			AudioProjection:       false,
			AutoProjectTranscript: true,
			AutoProjectSynthesis:  false,
		},
	}, ledger, NewMemoryEventBus())
	gateway.stt = fakeSTT{text: "canonical transcript"}

	room, err := gateway.SetProjection(context.Background(), VoiceProjectionRequest{
		OrganizationID:    "org-voice",
		RoomID:            "strategy-room",
		ProjectionEnabled: true,
	})
	require.NoError(t, err)
	require.True(t, room.ProjectionEnabled)

	result, err := gateway.IngestTranscript(context.Background(), VoiceTranscriptRequest{
		OrganizationID: "org-voice",
		ActorID:        "actor-1",
		RoomID:         "strategy-room",
		Text:           "canonical transcript",
	})
	require.NoError(t, err)
	require.Equal(t, VoiceEventTranscript, result.Event.Kind)
	require.Equal(t, "canonical transcript", result.Event.CanonicalText)
	require.NotEmpty(t, result.Event.Metadata["transcriptRef"])
	data, err := os.ReadFile(result.Event.Metadata["transcriptRef"])
	require.NoError(t, err)
	require.Equal(t, "canonical transcript\n", string(data))

	status, err := gateway.Status(context.Background(), "org-voice")
	require.NoError(t, err)
	require.Equal(t, "local", status.Provider)
	require.Equal(t, "agency.voice.synthesis.org-voice", status.SynthesisChannel)
	require.NotNil(t, status.LastVoiceEvent)
	require.Equal(t, VoiceEventTranscript, status.LastVoiceEvent.Kind)
	require.Len(t, status.Rooms, 1)
	require.Equal(t, result.Event.ID, status.Rooms[0].LastTranscriptID)
}

func TestVoiceGatewaySynthesizeUsesCanonicalText(t *testing.T) {
	t.Parallel()

	ledger, err := NewLedgerService(t.TempDir())
	require.NoError(t, err)

	gateway := NewVoiceGateway(VoiceGatewayConfig{
		Enabled:          true,
		Provider:         "local",
		StatePath:        t.TempDir() + "/voice-state.json",
		AssetDir:         t.TempDir() + "/voice-assets",
		ControlChannel:   "agency.voice.control",
		SynthesisChannel: "agency.voice.synthesis",
		DefaultRoom:      "briefing-room",
		Projection: VoiceProjectionDefaults{
			TranscriptProjection:  true,
			AudioProjection:       true,
			AutoProjectTranscript: true,
			AutoProjectSynthesis:  true,
		},
		TTS: SpeechRuntimeConfig{
			Enabled:     true,
			Command:     "fake-tts",
			OutputMode:  "file",
			AudioFormat: "wav",
		},
	}, ledger, NewMemoryEventBus())
	gateway.tts = fakeTTS{}

	result, err := gateway.Synthesize(context.Background(), VoiceSynthesisRequest{
		OrganizationID: "org-voice",
		ActorID:        "actor-1",
		RoomID:         "briefing-room",
		Text:           "ship it",
	})
	require.NoError(t, err)
	require.Equal(t, VoiceEventSynthesis, result.Event.Kind)
	require.Equal(t, "ship it", result.Event.CanonicalText)
	require.NotEmpty(t, result.Event.AudioOutputRef)

	snapshot, err := ledger.LatestSnapshot(context.Background(), "org-voice")
	require.NoError(t, err)
	require.NotNil(t, snapshot.LastVoiceEvent)
	require.Equal(t, VoiceEventSynthesis, snapshot.LastVoiceEvent.Kind)
	require.Equal(t, result.Event.ID, snapshot.VoiceRooms[0].LastSynthesisID)
}
