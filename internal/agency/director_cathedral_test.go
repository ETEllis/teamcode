package agency

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeSpecFetcher implements GISTTraceFetcher AND SpeculativeFetcher so
// the cathedral handler tests can exercise both code paths.
type fakeSpecFetcher struct {
	mu      sync.Mutex
	traces  map[string]InspectorTraceView
	bundles map[string]*SpeculativeBundle
}

func newFakeSpecFetcher() *fakeSpecFetcher {
	return &fakeSpecFetcher{
		traces:  map[string]InspectorTraceView{},
		bundles: map[string]*SpeculativeBundle{},
	}
}
func (f *fakeSpecFetcher) putBundle(id string, b *SpeculativeBundle) {
	f.mu.Lock(); defer f.mu.Unlock()
	f.bundles[id] = b
}
func (f *fakeSpecFetcher) GetInspectorTrace(ctx context.Context, id string) (*InspectorTraceView, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	v, ok := f.traces[id]
	if !ok { return nil, ErrTraceNotFound }
	return &v, nil
}
func (f *fakeSpecFetcher) ListInspectorTraces(ctx context.Context, office string, limit int) ([]InspectorTraceSummary, error) {
	return nil, nil
}
func (f *fakeSpecFetcher) GetSpeculativeBundle(ctx context.Context, id string) (*SpeculativeBundle, error) {
	f.mu.Lock(); defer f.mu.Unlock()
	if id == "missing" {
		return nil, ErrTraceNotFound
	}
	b, ok := f.bundles[id]
	if !ok { return nil, nil } // no bundle, but row exists
	return b, nil
}

func newTestDirectorWithSpecFetcher(t *testing.T, fetcher GISTTraceFetcher) *DirectorHTTPServer {
	t.Helper()
	dir := t.TempDir()
	ledger, err := NewLedgerService(dir)
	require.NoError(t, err)
	director, err := NewDirectorService(DirectorConfig{
		BaseDir:        dir,
		OrganizationID: "office-spec",
		Ledger:         ledger,
	})
	require.NoError(t, err)
	director.SetTraceFetcher(fetcher)
	server := NewDirectorHTTPServer(DirectorHTTPConfig{Addr: "127.0.0.1:0"}, director)
	return server
}

func sampleSpeculativeBundleForTest() *SpeculativeBundle {
	a := fixtureCohortVerdict("agent-a", "test", 0.8)
	b := fixtureCohortVerdict("agent-b", "test", 0.78)
	c := fixtureCohortVerdict("agent-c", "test", 0.76)
	return BuildSpeculativeBundle(SpeculativeBuildInput{
		CohortID: "cathedral-test-cohort",
		Verdicts: []LabeledVerdict{a, b, c},
	})
}

func TestCathedralHTML_PersistedBundle(t *testing.T) {
	fetcher := newFakeSpecFetcher()
	bundle := sampleSpeculativeBundleForTest()
	require.NotNil(t, bundle)
	fetcher.putBundle("trace-1", bundle)

	server := newTestDirectorWithSpecFetcher(t, fetcher)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lattice/spec/trace-1", nil)
	server.handleLatticeRouter(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "Lattice Cathedral")
	require.Contains(t, body, "trace: trace-1")
	require.Contains(t, body, "cathedral-data")
	// Embedded JSON contains the cohort id.
	require.Contains(t, body, "cathedral-test-cohort")
	// HTML pulls Three.js from CDN.
	require.Contains(t, body, "three.min.js")
}

func TestCathedralHTML_NoBundle_FallsBackToDemo(t *testing.T) {
	fetcher := newFakeSpecFetcher()
	// No bundle stored for this id → fetcher returns nil, nil.
	server := newTestDirectorWithSpecFetcher(t, fetcher)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lattice/spec/some-trace", nil)
	server.handleLatticeRouter(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "demo-cohort", "demo cohort id should be embedded")
	require.Contains(t, body, "rendering demo cohort")
}

func TestCathedralHTML_DemoRoute(t *testing.T) {
	fetcher := newFakeSpecFetcher()
	server := newTestDirectorWithSpecFetcher(t, fetcher)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lattice/spec/demo", nil)
	server.handleLatticeRouter(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "demo-cohort")
}

func TestCathedralAPIData_PersistedBundle(t *testing.T) {
	fetcher := newFakeSpecFetcher()
	bundle := sampleSpeculativeBundleForTest()
	fetcher.putBundle("trace-2", bundle)

	server := newTestDirectorWithSpecFetcher(t, fetcher)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lattice/spec/trace-2/data", nil)
	server.handleLatticeRouter(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("content-type"), "application/json")

	var payload CathedralPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, "trace-2", payload.TraceID)
	require.Equal(t, "persisted", payload.Source)
	require.NotNil(t, payload.Bundle)
	require.Equal(t, "cathedral-test-cohort", payload.Bundle.CohortID)
	require.Len(t, payload.Bundle.Peers, 3)
	require.Equal(t, "converged", payload.Headline)
}

func TestCathedralAPIData_MissingTraceReturns404(t *testing.T) {
	fetcher := newFakeSpecFetcher()
	server := newTestDirectorWithSpecFetcher(t, fetcher)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lattice/spec/missing/data", nil)
	server.handleLatticeRouter(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Contains(t, rec.Body.String(), "not_found")
}

func TestCathedralAPIData_DemoFallback(t *testing.T) {
	fetcher := newFakeSpecFetcher()
	server := newTestDirectorWithSpecFetcher(t, fetcher)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lattice/spec/no-bundle/data", nil)
	server.handleLatticeRouter(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var payload CathedralPayload
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.Equal(t, "none", payload.Source)
	require.NotNil(t, payload.Bundle)
	require.Equal(t, "demo-cohort", payload.Bundle.CohortID)
}

func TestLatticeRouter_DispatchesToInspectorByDefault(t *testing.T) {
	// /lattice/<id> (non-spec prefix) must route to the existing
	// inspector handler. We don't put a bundle in the fetcher; the
	// inspector should still 404 cleanly rather than fall through to
	// the cathedral.
	fetcher := newFakeSpecFetcher()
	server := newTestDirectorWithSpecFetcher(t, fetcher)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lattice/some-other-id", nil)
	server.handleLatticeRouter(rec, req)
	// Inspector view returns 404 for unknown ids, NOT 200.
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.NotContains(t, rec.Body.String(), "Lattice Cathedral",
		"non-spec lattice path must not render the cathedral page")
}

func TestCathedralHTML_DivergentStatusCrack(t *testing.T) {
	// Build a divergent cohort: 4 peers with different graphs → the
	// HTML should reach the rendering path; we just verify the
	// headline pill text is "divergent".
	mk := func(id, suffix string) LabeledVerdict {
		return LabeledVerdict{
			ID: id,
			Verdict: GISTVerdict{
				Verdict:     "approved",
				CausalGraph: fixtureCohortGraph(suffix),
			},
		}
	}
	bundle := BuildSpeculativeBundle(SpeculativeBuildInput{
		CohortID: "divergent-cohort",
		Verdicts: []LabeledVerdict{
			mk("a1", "p1"), mk("a2", "p2"), mk("a3", "p3"),
		},
	})
	require.NotNil(t, bundle)
	require.Equal(t, ConvergenceStatusDivergent, bundle.Convergence.Status)

	fetcher := newFakeSpecFetcher()
	fetcher.putBundle("div-trace", bundle)
	server := newTestDirectorWithSpecFetcher(t, fetcher)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/lattice/spec/div-trace", nil)
	server.handleLatticeRouter(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	// Headline should include "divergent" in the embedded JSON or in
	// the rendered status pill.
	require.True(t, strings.Contains(body, "divergent") || strings.Contains(body, "\"status\":\"divergent\""),
		"divergent cohort should surface in rendered HTML")
}
