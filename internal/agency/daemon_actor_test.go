package agency

import "testing"

func TestShouldProcessActorSignal(t *testing.T) {
	t.Parallel()

	actorID := "lead"
	tests := []struct {
		name   string
		signal WakeSignal
		want   bool
	}{
		{
			name: "accept schedule wake",
			signal: WakeSignal{
				ActorID: actorID,
				Kind:    SignalSchedule,
				Payload: map[string]string{"entrySource": "office.open"},
			},
			want: true,
		},
		{
			name: "ignore actor broadcast echo",
			signal: WakeSignal{
				ActorID: actorID,
				Kind:    SignalBroadcast,
				Payload: map[string]string{"entrySource": "actor.daemon.broadcast"},
			},
			want: false,
		},
		{
			name: "ignore actor approval echo",
			signal: WakeSignal{
				ActorID: actorID,
				Kind:    SignalReview,
				Payload: map[string]string{"entrySource": "actor.daemon.approval"},
			},
			want: false,
		},
		{
			name: "ignore any org broadcast",
			signal: WakeSignal{
				ActorID: "peer",
				Kind:    SignalBroadcast,
				Payload: map[string]string{"entrySource": "peer.broadcast"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := shouldProcessActorSignal(actorID, tt.signal); got != tt.want {
				t.Fatalf("shouldProcessActorSignal() = %v, want %v", got, tt.want)
			}
		})
	}
}
