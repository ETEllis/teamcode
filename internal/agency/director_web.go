package agency

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
)

type DirectorHTTPConfig struct {
	Addr  string
	Token string
}

type DirectorHTTPServer struct {
	cfg      DirectorHTTPConfig
	director *DirectorService
	server   *http.Server
}

func NewDirectorHTTPServer(cfg DirectorHTTPConfig, director *DirectorService) *DirectorHTTPServer {
	if strings.TrimSpace(cfg.Addr) == "" {
		cfg.Addr = "127.0.0.1:8765"
	}
	mux := http.NewServeMux()
	s := &DirectorHTTPServer{cfg: cfg, director: director}
	mux.HandleFunc("/", s.requireAuth(s.handleIndex))
	mux.HandleFunc("/api/status", s.requireAuth(s.handleStatus))
	mux.HandleFunc("/api/tickets", s.requireAuth(s.handleTickets))
	mux.HandleFunc("/api/events", s.requireAuth(s.handleEvents))
	mux.HandleFunc("/api/monitor", s.requireAuth(s.handleMonitor))
	mux.HandleFunc("/api/dispatch/", s.requireAuth(s.handleDispatch))
	// Lattice inspector (Phase 3, item #14). Renders typed CausalGraph
	// + PearlPlan + Shapley attribution per persisted GIST trace.
	mux.HandleFunc("/lattice", s.requireAuth(s.handleLatticeIndex))
	mux.HandleFunc("/lattice/", s.requireAuth(s.handleLatticeRouter))
	mux.HandleFunc("/api/lattice", s.requireAuth(s.handleAPILatticeList))
	mux.HandleFunc("/api/lattice/", s.requireAuth(s.handleAPILatticeView))
	s.server = &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *DirectorHTTPServer) Serve(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		err := s.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.server.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *DirectorHTTPServer) URL() string {
	return "http://" + s.cfg.Addr
}

// Handler returns the underlying http.Handler the server dispatches
// from. Exposed so callers (notably the Phase 6 end-to-end test) can
// drive the same routing surface that production traffic flows
// through without spinning up a real listener. The returned handler
// still includes requireAuth, so tests must either set Token to "" or
// pass the configured token via header / query string.
func (s *DirectorHTTPServer) Handler() http.Handler {
	return s.server.Handler
}

func (s *DirectorHTTPServer) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Token == "" {
			next(w, r)
			return
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" {
			token = r.URL.Query().Get("token")
		}
		if token != s.cfg.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *DirectorHTTPServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "text/html; charset=utf-8")
	tokenHint := ""
	if s.cfg.Token != "" {
		tokenHint = "?token=" + template.URLQueryEscaper(s.cfg.Token)
	}
	_ = directorIndexTemplate.Execute(w, map[string]string{
		"TokenHint": tokenHint,
	})
}

func (s *DirectorHTTPServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.director.Status(r.Context())
	writeJSON(w, status, err)
}

func (s *DirectorHTTPServer) handleTickets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tickets, err := s.director.ListTickets()
		writeJSON(w, tickets, err)
	case http.MethodPost:
		var req DirectorTicketRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, nil, err)
			return
		}
		if req.Source == "" {
			req.Source = "director.web"
		}
		ticket, err := s.director.SubmitTicket(r.Context(), req)
		writeJSON(w, ticket, err)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *DirectorHTTPServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	events, err := s.director.ListEvents()
	writeJSON(w, events, err)
}

func (s *DirectorHTTPServer) handleMonitor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.director.Monitor(r.Context())
	writeJSON(w, status, err)
}

func (s *DirectorHTTPServer) handleDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/dispatch/")
	if id == "" {
		writeJSON(w, nil, fmt.Errorf("ticket id is required"))
		return
	}
	ticket, err := s.director.DispatchTicket(r.Context(), id)
	writeJSON(w, ticket, err)
}

func writeJSON(w http.ResponseWriter, value any, err error) {
	w.Header().Set("content-type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}

var directorIndexTemplate = template.Must(template.New("director").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Agency Director</title>
  <style>
    :root {
      color-scheme: dark;
      --ink: #101114;
      --panel: #191a1f;
      --line: #333842;
      --gold: #e2b76d;
      --cyan: #5eb7c7;
      --green: #7fb069;
      --text: #e8e3d6;
      --muted: #a7aaae;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      font: 15px/1.45 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      color: var(--text);
      background: var(--ink);
    }
    header, main { width: min(1120px, calc(100vw - 32px)); margin: 0 auto; }
    header { padding: 28px 0 18px; display: flex; align-items: end; justify-content: space-between; gap: 20px; border-bottom: 1px solid var(--line); }
    h1 { margin: 0; font-size: 28px; font-weight: 760; letter-spacing: 0; }
    .sub { color: var(--muted); margin-top: 4px; }
    main { display: grid; grid-template-columns: minmax(0, 1fr) 340px; gap: 18px; padding: 20px 0 36px; }
    section, aside { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; padding: 16px; }
    textarea, input, select, button {
      width: 100%;
      border-radius: 7px;
      border: 1px solid var(--line);
      background: #111318;
      color: var(--text);
      padding: 11px 12px;
      font: inherit;
    }
    textarea { min-height: 132px; resize: vertical; }
    button { cursor: pointer; background: var(--gold); color: #17130a; border: none; font-weight: 760; }
    button.secondary { background: #242a32; color: var(--text); border: 1px solid var(--line); }
    .row { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin: 10px 0; }
    .actions { display: flex; gap: 10px; margin-top: 12px; }
    .pill { display: inline-flex; align-items: center; min-height: 28px; padding: 4px 9px; border-radius: 999px; background: #23252b; color: var(--muted); margin: 3px 6px 3px 0; font-size: 13px; }
    .ticket { border-top: 1px solid var(--line); padding: 13px 0; }
    .ticket h3 { margin: 0 0 5px; font-size: 16px; }
    .ticket p { margin: 0 0 9px; color: var(--muted); }
    .muted { color: var(--muted); }
    .accent { color: var(--cyan); }
    .status-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 8px; }
    .metric { border: 1px solid var(--line); border-radius: 7px; padding: 10px; background: #12151a; }
    .metric strong { display: block; font-size: 22px; color: var(--green); }
    @media (max-width: 820px) {
      main { grid-template-columns: 1fr; }
      header { align-items: start; flex-direction: column; }
      .row { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <header>
    <div>
      <h1>Agency Director</h1>
      <div class="sub">A calm personal interface over your local AI office.</div>
    </div>
    <div style="display:flex;gap:8px;align-items:center"><a class="pill" href="/lattice{{.TokenHint}}">Lattice inspector →</a><button class="secondary" style="max-width: 180px" onclick="monitor()">Check Office</button></div>
  </header>
  <main>
    <section>
      <h2>Tell Director</h2>
      <input id="title" placeholder="Short title">
      <textarea id="body" placeholder="What should your Agency work on or watch?"></textarea>
      <div class="row">
        <select id="priority"><option>normal</option><option>high</option><option>low</option></select>
        <select id="risk"><option>unknown</option><option>low</option><option>medium</option><option>high</option></select>
      </div>
      <div class="actions">
        <button onclick="submitTicket(false)">Open Ticket</button>
        <button onclick="submitTicket(true)">Open + Dispatch</button>
      </div>
      <h2>Tickets</h2>
      <div id="tickets" class="muted">Loading...</div>
    </section>
    <aside>
      <h2>Status</h2>
      <div id="status" class="status-grid"></div>
      <h2>Last Word</h2>
      <p id="last" class="muted">Director is standing by.</p>
      <p class="muted">Remote access should stay behind a tunnel plus this app token. Localhost is the safe default.</p>
    </aside>
  </main>
  <script>
    const token = new URLSearchParams(location.search).get('token') || '';
    const tokenHint = "{{.TokenHint}}";
    const headers = () => ({'content-type': 'application/json', ...(token ? {authorization: 'Bearer ' + token} : {})});
    async function api(path, opts = {}) {
      const res = await fetch(path + (!token && tokenHint ? tokenHint : ''), {...opts, headers: {...headers(), ...(opts.headers || {})}});
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    }
    async function refresh() {
      const [status, tickets] = await Promise.all([api('/api/status'), api('/api/tickets')]);
      document.getElementById('status').innerHTML = [
        ['Open', status.openTickets], ['Dispatched', status.dispatched],
        ['Approvals', status.pendingApprovals], ['Ledger', status.ledgerSequence]
      ].map(([k,v]) => '<div class="metric"><span class="muted">'+k+'</span><strong>'+v+'</strong></div>').join('');
      document.getElementById('last').textContent = status.lastEvent ? status.lastEvent.message : 'Director is standing by.';
      document.getElementById('tickets').innerHTML = tickets.length ? tickets.slice().reverse().map(t =>
        '<div class="ticket"><h3>'+escapeHtml(t.title)+'</h3><p>'+escapeHtml(t.body)+'</p><span class="pill">'+t.status+'</span><span class="pill">'+t.priority+'</span><span class="pill">'+t.risk+'</span>' +
        (t.status === 'open' ? '<button class="secondary" onclick="dispatchTicket(\''+t.id+'\')">Dispatch</button>' : '') + '</div>'
      ).join('') : 'No tickets yet.';
    }
    async function submitTicket(autoDispatch) {
      await api('/api/tickets', {method:'POST', body: JSON.stringify({
        title: document.getElementById('title').value,
        body: document.getElementById('body').value,
        priority: document.getElementById('priority').value,
        risk: document.getElementById('risk').value,
        autoDispatch
      })});
      document.getElementById('title').value = '';
      document.getElementById('body').value = '';
      await refresh();
    }
    async function dispatchTicket(id) { await api('/api/dispatch/' + id, {method:'POST'}); await refresh(); }
    async function monitor() { await api('/api/monitor', {method:'POST'}); await refresh(); }
    function escapeHtml(s) { return String(s || '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#039;'}[c])); }
    refresh().catch(err => { document.getElementById('tickets').textContent = err.message; });
    setInterval(() => refresh().catch(() => {}), 10000);
  </script>
</body>
</html>`))
