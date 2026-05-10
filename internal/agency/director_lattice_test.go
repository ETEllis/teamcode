package agency

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeTraceFetcher is a hand-rolled in-memory GISTTraceFetcher for
// exercising the inspector handlers without spinning up the real DB.
type fakeTraceFetcher struct {
	mu        sync.Mutex
	traces    map[string]InspectorTraceView
	listCalls int
}

func newFakeTraceFetcher() *fakeTraceFetcher {
	return &fakeTraceFetcher{traces: map[string]InspectorTraceView{}}
}

func (f *fakeTraceFetcher) put(view InspectorTraceView) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.traces[view.Summary.ID] = view
}

func (f *fakeTraceFetcher) GetInspectorTrace(ctx context.Context, id string) (*InspectorTraceView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.traces[id]
	if !ok {
		return nil, ErrTraceNotFound
	}
	return &v, nil
}

func (f *fakeTraceFetcher) ListInspectorTraces(ctx context.Context, office string, limit int) ([]InspectorTraceSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	out := []InspectorTraceSummary{}
	for _, v := range f.traces {
		if office != "" && v.Summary.OfficeID != office {
			continue
		}
		out = append(out, v.Summary)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func newTestDirectorWithFetcher(t *testing.T, fetcher GISTTraceFetcher) *DirectorHTTPServer {
	t.Helper()
	dir := t.TempDir()
	ledger, err := NewLedgerService(dir)
	require.NoError(t, err)
	director, err := NewDirectorService(DirectorConfig{
		BaseDir:        dir,
		OrganizationID: "office-test",
		Ledger:         ledger,
	})
	require.NoError(t, err)
	director.SetTraceFetcher(fetcher)
	server := NewDirectorHTTPServer(DirectorHTTPConfig{Addr: "127.0.0.1:0"}, director)
	return server
}

func sampleInspectorView(id string) InspectorTraceView {
	verdict := sampleVerdictForBundle()
	bundle := BuildInspectorBundle(verdict)
	return InspectorTraceView{
		Summary: InspectorTraceSummary{
			ID:         id,
			OfficeID:   "office-test",
			AgentID:    "agent-A",
			Verdict:    verdict.Verdict,
			RiskLevel:  verdict.RiskLevel,
			Confidence: verdict.Confidence,
			CreatedAt:  1_700_000_000_000,
			HasBundle:  true,
		},
		Bundle:        bundle,
		BundleSource:  "inspector_json",
		LatticeJSON:   `{"canonicalSlots":64}`,
		TraceJSON:     `{"id":"` + id + `"}`,
		ProofJSON:     `{}`,
		InspectorJSON: `{"protocolVersion":1}`,
	}
}

func TestLatticeIndexHTMLRendersSummaries(t *testing.T) {
	fetcher := newFakeTraceFetcher()
	fetcher.put(sampleInspectorView("trace-1"))
	server := newTestDirectorWithFetcher(t, fetcher)

	req := httptest.NewRequest("GET", "/lattice", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	body := rec.Body.String()
	require.Contains(t, body, "Lattice Inspector")
	require.Contains(t, body, "trace-1")
	require.Contains(t, body, "agent-A")
	require.Equal(t, 1, fetcher.listCalls)
}

func TestLatticeIndexEmptyOfficeShowsEmptyState(t *testing.T) {
	server := newTestDirectorWithFetcher(t, newFakeTraceFetcher())

	req := httptest.NewRequest("GET", "/lattice", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "No traces yet")
}

func TestLatticeViewHTMLRendersBundle(t *testing.T) {
	fetcher := newFakeTraceFetcher()
	fetcher.put(sampleInspectorView("trace-42"))
	server := newTestDirectorWithFetcher(t, fetcher)

	req := httptest.NewRequest("GET", "/lattice/trace-42", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	body := rec.Body.String()
	for _, want := range []string{
		"Trace ·",
		"trace-42",
		"Pearl plan",
		"Causal graph",
		"Necessity attribution",
		"node_evidence",
		"node_confounder",
		"do(publish)",
	} {
		require.True(t, strings.Contains(body, want),
			"expected %q in inspector HTML", want)
	}
}

func TestLatticeViewMissingTraceReturns404(t *testing.T) {
	server := newTestDirectorWithFetcher(t, newFakeTraceFetcher())
	req := httptest.NewRequest("GET", "/lattice/does-not-exist", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAPILatticeViewReturnsStructuredJSON(t *testing.T) {
	fetcher := newFakeTraceFetcher()
	fetcher.put(sampleInspectorView("trace-json-1"))
	server := newTestDirectorWithFetcher(t, fetcher)

	req := httptest.NewRequest("GET", "/api/lattice/trace-json-1", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "application/json", rec.Header().Get("content-type"))

	var got InspectorTraceView
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Equal(t, "trace-json-1", got.Summary.ID)
	require.NotNil(t, got.Bundle)
	require.NotNil(t, got.Bundle.CausalGraph)
	require.Len(t, got.Bundle.Attribution, 3)
}

func TestAPILatticeListJSON(t *testing.T) {
	fetcher := newFakeTraceFetcher()
	fetcher.put(sampleInspectorView("a"))
	fetcher.put(sampleInspectorView("b"))
	server := newTestDirectorWithFetcher(t, fetcher)

	req := httptest.NewRequest("GET", "/api/lattice", nil)
	rec := httptest.NewRecorder()
	server.server.Handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var got []InspectorTraceSummary
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.Len(t, got, 2)
}

func TestLatticeRoutes503WhenNoFetcher(t *testing.T) {
	server := newTestDirectorWithFetcher(t, nil)

	for _, path := range []string{"/lattice", "/lattice/x", "/api/lattice", "/api/lattice/x"} {
		req := httptest.NewRequest("GET", path, nil)
		rec := httptest.NewRecorder()
		server.server.Handler.ServeHTTP(rec, req)
		require.True(t,
			rec.Code == http.StatusServiceUnavailable || rec.Code == http.StatusBadRequest,
			"path=%s code=%d", path, rec.Code)
	}
}

func TestSetTraceFetcherIsOneShot(t *testing.T) {
	fetcher1 := newFakeTraceFetcher()
	fetcher2 := newFakeTraceFetcher()

	dir := t.TempDir()
	ledger, err := NewLedgerService(dir)
	require.NoError(t, err)
	director, err := NewDirectorService(DirectorConfig{
		BaseDir:        dir,
		OrganizationID: "office-x",
		Ledger:         ledger,
	})
	require.NoError(t, err)

	require.Nil(t, director.TraceFetcher())
	director.SetTraceFetcher(fetcher1)
	require.NotNil(t, director.TraceFetcher())
	director.SetTraceFetcher(fetcher2)
	// Second call should NOT swap the fetcher.
	require.Same(t, GISTTraceFetcher(fetcher1), director.TraceFetcher())
}

func TestErrTraceNotFoundIsErrorsIs(t *testing.T) {
	wrapped := errors.New("wrapped: " + ErrTraceNotFound.Error())
	require.False(t, errors.Is(wrapped, ErrTraceNotFound),
		"text-comparison wrapping should not satisfy errors.Is")
	wrapped2 := &wrappedErr{inner: ErrTraceNotFound}
	require.True(t, errors.Is(wrapped2, ErrTraceNotFound))
}

type wrappedErr struct{ inner error }

func (e *wrappedErr) Error() string { return "wrap: " + e.inner.Error() }
func (e *wrappedErr) Unwrap() error { return e.inner }
