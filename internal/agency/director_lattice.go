package agency

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ErrTraceNotFound is the sentinel a GISTTraceFetcher returns when the
// requested trace id has no row in storage. The lattice inspector
// translates this into a 404.
var ErrTraceNotFound = errors.New("gist trace not found")

// GISTTraceFetcher is the read-only side door the lattice inspector
// uses to load persisted traces. Implementations live alongside the DB
// (see internal/agency/cmd/director-daemon/trace_fetcher.go) so the
// inspector can render /lattice/<trace_id> without coupling
// DirectorService to the database layer.
type GISTTraceFetcher interface {
	GetInspectorTrace(ctx context.Context, traceID string) (*InspectorTraceView, error)
	ListInspectorTraces(ctx context.Context, officeID string, limit int) ([]InspectorTraceSummary, error)
}

// InspectorTraceSummary is the headline list-row for the inspector
// index page. Cheap to serialise, used by the picker UI.
type InspectorTraceSummary struct {
	ID         string  `json:"id"`
	OfficeID   string  `json:"officeId"`
	AgentID    string  `json:"agentId"`
	Verdict    string  `json:"verdict"`
	RiskLevel  string  `json:"riskLevel,omitempty"`
	Confidence float64 `json:"confidence"`
	CreatedAt  int64   `json:"createdAt"`
	HasBundle  bool    `json:"hasBundle"`
}

// InspectorTraceView is the full payload rendered at /lattice/<trace_id>.
// It composes the persisted blobs (lattice JSON, raw trace, proof) with
// the typed Inspector bundle so the HTML template can render any of
// them without further parsing.
type InspectorTraceView struct {
	Summary       InspectorTraceSummary `json:"summary"`
	Bundle        *GISTInspectorBundle  `json:"bundle,omitempty"`
	BundleSource  string                `json:"bundleSource"`
	LatticeJSON   string                `json:"latticeJson,omitempty"`
	TraceJSON     string                `json:"traceJson,omitempty"`
	ProofJSON     string                `json:"proofJson,omitempty"`
	InspectorJSON string                `json:"inspectorJson,omitempty"`
}

// SetTraceFetcher attaches a fetcher to the DirectorService. May be
// called at most once during construction; later calls are silently
// ignored to avoid mid-flight swaps.
func (d *DirectorService) SetTraceFetcher(f GISTTraceFetcher) {
	if d == nil || d.traceFetcher != nil {
		return
	}
	d.traceFetcher = f
}

// TraceFetcher returns the registered fetcher or nil if none is wired.
// The HTTP handlers gate inspector routes on a non-nil fetcher.
func (d *DirectorService) TraceFetcher() GISTTraceFetcher {
	if d == nil {
		return nil
	}
	return d.traceFetcher
}

func (s *DirectorHTTPServer) handleLatticeIndex(w http.ResponseWriter, r *http.Request) {
	fetcher := s.director.TraceFetcher()
	if fetcher == nil {
		http.Error(w, "lattice inspector unavailable: no trace store wired", http.StatusServiceUnavailable)
		return
	}
	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	office := s.director.cfg.OrganizationID
	if q := r.URL.Query().Get("office"); q != "" {
		office = q
	}
	summaries, err := fetcher.ListInspectorTraces(r.Context(), office, limit)
	if err != nil {
		writeJSON(w, nil, fmt.Errorf("list traces: %w", err))
		return
	}
	tokenHint := ""
	if s.cfg.Token != "" {
		tokenHint = "?token=" + template.URLQueryEscaper(s.cfg.Token)
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	_ = latticeIndexTemplate.Execute(w, map[string]any{
		"Summaries": summaries,
		"TokenHint": tokenHint,
		"Office":    office,
	})
}

func (s *DirectorHTTPServer) handleLatticeView(w http.ResponseWriter, r *http.Request) {
	fetcher := s.director.TraceFetcher()
	if fetcher == nil {
		http.Error(w, "lattice inspector unavailable: no trace store wired", http.StatusServiceUnavailable)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/lattice/")
	if id == "" || strings.Contains(id, "/") {
		http.Redirect(w, r, "/lattice", http.StatusSeeOther)
		return
	}
	view, err := fetcher.GetInspectorTrace(r.Context(), id)
	if errors.Is(err, ErrTraceNotFound) {
		http.Error(w, "trace not found: "+id, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "lookup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tokenHint := ""
	if s.cfg.Token != "" {
		tokenHint = "?token=" + template.URLQueryEscaper(s.cfg.Token)
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	_ = latticeViewTemplate.Execute(w, map[string]any{
		"View":      view,
		"TokenHint": tokenHint,
		"CreatedAt": time.UnixMilli(view.Summary.CreatedAt).UTC().Format(time.RFC3339),
	})
}

func (s *DirectorHTTPServer) handleAPILatticeList(w http.ResponseWriter, r *http.Request) {
	fetcher := s.director.TraceFetcher()
	if fetcher == nil {
		writeJSON(w, nil, errors.New("trace store not configured"))
		return
	}
	limit := 25
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	office := s.director.cfg.OrganizationID
	if q := r.URL.Query().Get("office"); q != "" {
		office = q
	}
	summaries, err := fetcher.ListInspectorTraces(r.Context(), office, limit)
	writeJSON(w, summaries, err)
}

func (s *DirectorHTTPServer) handleAPILatticeView(w http.ResponseWriter, r *http.Request) {
	fetcher := s.director.TraceFetcher()
	if fetcher == nil {
		writeJSON(w, nil, errors.New("trace store not configured"))
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/lattice/")
	if id == "" || strings.Contains(id, "/") {
		writeJSON(w, nil, errors.New("trace id is required"))
		return
	}
	view, err := fetcher.GetInspectorTrace(r.Context(), id)
	if errors.Is(err, ErrTraceNotFound) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not_found", "id": id})
		return
	}
	writeJSON(w, view, err)
}

var latticeIndexTemplate = template.Must(template.New("lattice-index").Parse(`<!doctype html>
<html lang="en"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Agency · Lattice Inspector</title>
<style>
:root { color-scheme: dark; --ink:#101114; --panel:#191a1f; --line:#333842; --gold:#e2b76d; --cyan:#5eb7c7; --green:#7fb069; --red:#d97a6c; --text:#e8e3d6; --muted:#a7aaae; }
*{box-sizing:border-box}
body{margin:0;font:15px/1.45 ui-sans-serif,system-ui,-apple-system,sans-serif;color:var(--text);background:var(--ink)}
header{padding:24px 24px 12px;border-bottom:1px solid var(--line);display:flex;justify-content:space-between;align-items:end;gap:16px}
h1{margin:0;font-size:24px;font-weight:760}
.sub{color:var(--muted)}
main{padding:18px 24px 32px;max-width:1200px;margin:0 auto}
table{width:100%;border-collapse:collapse;background:var(--panel);border:1px solid var(--line);border-radius:8px;overflow:hidden}
th,td{text-align:left;padding:10px 12px;border-bottom:1px solid var(--line);font-size:14px}
th{background:#13151a;color:var(--muted);font-weight:600;letter-spacing:.02em}
tr:last-child td{border-bottom:none}
tr:hover{background:#1d1f25}
a{color:var(--cyan);text-decoration:none}
a:hover{text-decoration:underline}
.pill{display:inline-block;padding:2px 8px;border-radius:999px;background:#23252b;color:var(--muted);font-size:12px}
.pill.high{background:#3a221f;color:var(--red)}
.pill.medium{background:#3a2f1f;color:var(--gold)}
.pill.low{background:#1f3a26;color:var(--green)}
.empty{color:var(--muted);text-align:center;padding:32px}
</style></head><body>
<header><div><h1>Lattice Inspector</h1><div class="sub">Office: {{.Office}} · {{len .Summaries}} traces</div></div><a href="/{{.TokenHint}}">← Director</a></header>
<main>
{{if .Summaries}}
<table>
<thead><tr><th>ID</th><th>Agent</th><th>Verdict</th><th>Risk</th><th>Conf</th><th>When</th><th></th></tr></thead>
<tbody>
{{range .Summaries}}
<tr>
<td><code>{{.ID}}</code></td>
<td>{{.AgentID}}</td>
<td>{{.Verdict}}</td>
<td><span class="pill {{.RiskLevel}}">{{if .RiskLevel}}{{.RiskLevel}}{{else}}—{{end}}</span></td>
<td>{{printf "%.2f" .Confidence}}</td>
<td><span class="pill">{{.CreatedAt}}</span></td>
<td><a href="/lattice/{{.ID}}{{$.TokenHint}}">Inspect →</a></td>
</tr>
{{end}}
</tbody></table>
{{else}}
<div class="empty">No traces yet for office <code>{{.Office}}</code>.</div>
{{end}}
</main></body></html>`))

var latticeViewTemplate = template.Must(template.New("lattice-view").Funcs(template.FuncMap{
	"pct": func(f float64) string { return strconv.FormatFloat(f*100, 'f', 1, 64) + "%" },
	"f2":  func(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) },
	"f3":  func(f float64) string { return strconv.FormatFloat(f, 'f', 3, 64) },
}).Parse(`<!doctype html>
<html lang="en"><head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Lattice · {{.View.Summary.ID}}</title>
<style>
:root { color-scheme: dark; --ink:#101114; --panel:#191a1f; --line:#333842; --gold:#e2b76d; --cyan:#5eb7c7; --green:#7fb069; --red:#d97a6c; --purple:#b08fcf; --text:#e8e3d6; --muted:#a7aaae; }
*{box-sizing:border-box}
body{margin:0;font:15px/1.45 ui-sans-serif,system-ui,-apple-system,sans-serif;color:var(--text);background:var(--ink)}
header{padding:20px 24px;border-bottom:1px solid var(--line);display:flex;justify-content:space-between;align-items:end;gap:16px;flex-wrap:wrap}
h1{margin:0;font-size:22px;font-weight:760}
h2{font-size:16px;color:var(--muted);margin:0 0 10px;letter-spacing:.04em;text-transform:uppercase;font-weight:660}
h3{font-size:14px;color:var(--muted);margin:14px 0 6px}
.sub{color:var(--muted);font-size:14px;margin-top:4px}
main{padding:18px 24px 36px;max-width:1280px;margin:0 auto;display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:18px}
section{background:var(--panel);border:1px solid var(--line);border-radius:8px;padding:14px 16px}
section.full{grid-column:1/-1}
.kv{display:grid;grid-template-columns:max-content 1fr;gap:6px 14px;font-size:13.5px}
.kv dt{color:var(--muted)}
.kv dd{margin:0}
.pill{display:inline-block;padding:2px 8px;border-radius:999px;background:#23252b;color:var(--muted);font-size:12px;margin:1px 2px}
.pill.high{background:#3a221f;color:var(--red)}
.pill.medium{background:#3a2f1f;color:var(--gold)}
.pill.low{background:#1f3a26;color:var(--green)}
.role-evidence{color:var(--green)}
.role-confounder{color:var(--red)}
.role-intervention{color:var(--gold)}
.role-outcome{color:var(--cyan)}
.role-unknown{color:var(--muted)}
table{width:100%;border-collapse:collapse;font-size:13px}
th,td{text-align:left;padding:6px 8px;border-bottom:1px solid var(--line);vertical-align:top}
th{color:var(--muted);font-weight:600}
tr:last-child td{border-bottom:none}
.bar{display:inline-block;height:6px;border-radius:3px;background:var(--cyan);vertical-align:middle;margin-left:8px}
.bar.neg{background:var(--red)}
pre{background:#0d0e12;border:1px solid var(--line);border-radius:6px;padding:10px;font-size:12px;overflow:auto;max-height:340px;color:#cdd1d6}
ul{margin:6px 0 0;padding-left:18px}
li{margin:2px 0}
a{color:var(--cyan);text-decoration:none}
a:hover{text-decoration:underline}
.legend{font-size:12px;color:var(--muted);margin-top:6px}
.warn{color:var(--gold)}
.crit{color:var(--red)}
.muted{color:var(--muted)}
@media (max-width:900px){main{grid-template-columns:1fr} }
</style></head><body>
<header>
  <div>
    <h1>Trace · <code>{{.View.Summary.ID}}</code></h1>
    <div class="sub">{{.View.Summary.AgentID}} · office {{.View.Summary.OfficeID}} · {{.CreatedAt}}</div>
  </div>
  <div>
    <a href="/lattice{{.TokenHint}}">← All traces</a>
    &nbsp;·&nbsp;
    <a href="/api/lattice/{{.View.Summary.ID}}{{.TokenHint}}">JSON</a>
  </div>
</header>
<main>

<section>
  <h2>Verdict</h2>
  <dl class="kv">
    <dt>Verdict</dt><dd>{{.View.Summary.Verdict}}</dd>
    <dt>Risk</dt><dd><span class="pill {{.View.Summary.RiskLevel}}">{{if .View.Summary.RiskLevel}}{{.View.Summary.RiskLevel}}{{else}}—{{end}}</span></dd>
    <dt>Confidence</dt><dd>{{f2 .View.Summary.Confidence}} ({{pct .View.Summary.Confidence}})</dd>
    {{with .View.Bundle}}
    {{if .Degraded}}<dt>Degraded</dt><dd class="warn">{{.DegradedReason}}</dd>{{end}}
    {{if .ExecutionIntent}}<dt>Intent</dt><dd>{{.ExecutionIntent}}</dd>{{end}}
    {{end}}
    <dt>Source</dt><dd class="muted">{{.View.BundleSource}}</dd>
  </dl>
  {{with .View.Bundle}}
  {{if .OpenQuestions}}
    <h3>Open questions</h3>
    <ul>{{range .OpenQuestions}}<li>{{.}}</li>{{end}}</ul>
  {{end}}
  {{end}}
</section>

<section>
  <h2>Pearl plan</h2>
  {{with .View.Bundle}}
  {{if .PearlPlan}}
    <h3>Hypothesis</h3>
    <dl class="kv">
      <dt>Evidence weight</dt><dd>{{f3 .PearlPlan.Hypothesis.EvidenceWeight}}</dd>
      <dt>Confounder load</dt><dd>{{f3 .PearlPlan.Hypothesis.ConfounderLoad}}</dd>
      <dt>Evidence nodes</dt><dd>{{range .PearlPlan.Hypothesis.Evidence}}<span class="pill role-evidence">{{.}}</span>{{end}}{{if not .PearlPlan.Hypothesis.Evidence}}<span class="muted">none</span>{{end}}</dd>
      <dt>Confounder nodes</dt><dd>{{range .PearlPlan.Hypothesis.Confounders}}<span class="pill role-confounder">{{.}}</span>{{end}}{{if not .PearlPlan.Hypothesis.Confounders}}<span class="muted">none</span>{{end}}</dd>
    </dl>
    <h3>Action candidates</h3>
    {{if .PearlPlan.Actions}}
    <table>
      <thead><tr><th></th><th>Label</th><th>Score</th><th>Risk</th></tr></thead>
      <tbody>
      {{range .PearlPlan.Actions}}
      <tr>
        <td>{{if .Recommended}}<span class="pill low">REC</span>{{end}}</td>
        <td><span class="role-intervention">{{.Label}}</span></td>
        <td>{{f3 .Score}}</td>
        <td>{{f3 .Risk}}</td>
      </tr>
      {{end}}
      </tbody>
    </table>
    {{else}}<div class="muted">No action candidates were considered.</div>{{end}}
    <h3>Prediction</h3>
    <dl class="kv">
      <dt>Recommended</dt><dd>{{if .PearlPlan.Prediction.Recommended}}<span class="role-intervention">{{.PearlPlan.Prediction.Recommended}}</span>{{else}}<span class="muted">none</span>{{end}}</dd>
      <dt>Projected confidence</dt><dd>{{f3 .PearlPlan.Prediction.ProjectedConfidence}}</dd>
      {{if .PearlPlan.Prediction.BlockedByConfounder}}<dt>Status</dt><dd class="crit">blocked by confounder load</dd>{{end}}
    </dl>
    {{if .PearlPlan.Prediction.Residual}}
    <h3>Residual open questions</h3>
    <ul>{{range .PearlPlan.Prediction.Residual}}<li>{{.}}</li>{{end}}</ul>
    {{end}}
  {{else}}
    <div class="muted">No Pearl plan was attached to this verdict (likely a degraded or pre-Phase-2 trace).</div>
  {{end}}
  {{else}}
  <div class="muted">No inspector bundle persisted for this trace.</div>
  {{end}}
</section>

<section class="full">
  <h2>Causal graph</h2>
  {{with .View.Bundle}}
  {{if .CausalGraph}}
  <div class="legend">Roles: <span class="pill role-outcome">outcome</span><span class="pill role-evidence">evidence</span><span class="pill role-intervention">intervention</span><span class="pill role-confounder">confounder</span><span class="pill role-unknown">unknown</span></div>
  <table>
    <thead><tr><th>Role</th><th>Node</th><th>Summary</th><th>Weight</th><th>Parents</th></tr></thead>
    <tbody>
    {{range .CausalGraph.Nodes}}
    <tr>
      <td><span class="role-{{.Role}}">{{.Role}}</span></td>
      <td><code>{{.ID}}</code></td>
      <td>{{.Summary}}</td>
      <td>{{f3 .Weight}}</td>
      <td>{{range $i, $p := .Parents}}{{if $i}}, {{end}}<code>{{$p}}</code>{{end}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
  {{else}}<div class="muted">No typed causal graph for this trace.</div>{{end}}
  {{end}}
</section>

<section class="full">
  <h2>Necessity attribution (Shapley φ)</h2>
  {{with .View.Bundle}}
  {{if .Attribution}}
  <table>
    <thead><tr><th>#</th><th>Role</th><th>Node</th><th>φ</th><th></th></tr></thead>
    <tbody>
    {{range .Attribution}}
    <tr>
      <td>{{.Rank}}</td>
      <td><span class="role-{{.Role}}">{{.Role}}</span></td>
      <td><code>{{.NodeID}}</code></td>
      <td>{{f3 .Phi}}{{if .Approximate}} <span class="muted">~</span>{{end}}</td>
      <td>{{if lt .Phi 0.0}}<span class="bar neg" style="width:{{f2 .Phi}}px"></span>{{else}}<span class="bar" style="width:{{f2 .Phi}}px"></span>{{end}}</td>
    </tr>
    {{end}}
    </tbody>
  </table>
  <div class="legend">φ &gt; 0 pushed confidence up · φ &lt; 0 pulled it down. Confounders typically appear with negative φ.</div>
  {{else}}<div class="muted">No necessity attribution for this trace.</div>{{end}}
  {{end}}
</section>

<section>
  <h2>Reasoning chain</h2>
  {{with .View.Bundle}}
  {{if .FlatChain}}
  <ol>{{range .FlatChain}}<li>{{.}}</li>{{end}}</ol>
  {{else}}<div class="muted">empty</div>{{end}}
  {{end}}
</section>

<section>
  <h2>Lattice JSON</h2>
  {{if .View.LatticeJSON}}<pre>{{.View.LatticeJSON}}</pre>{{else}}<div class="muted">none</div>{{end}}
</section>

</main></body></html>`))
