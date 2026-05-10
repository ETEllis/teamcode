package agency

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"sync"
)

// SpeculativeFetcher is the optional capability the cathedral route
// requires from a GISTTraceFetcher: read the persisted SpeculativeBundle
// for a given trace id. It is split out so callers can implement
// GISTTraceFetcher without breaking the lattice inspector when they
// haven't wired speculative storage yet.
//
// The cathedral handler type-asserts on this interface and falls back
// to an embedded demo cohort if either the assertion fails or the
// fetched bundle is nil.
type SpeculativeFetcher interface {
	GetSpeculativeBundle(ctx context.Context, traceID string) (*SpeculativeBundle, error)
}

// CathedralPayload is the JSON payload served at /lattice/spec/{id}/data
// and embedded inline by the HTML route. It carries the SpeculativeBundle
// plus a small amount of header metadata the renderer uses for the
// status pill and breadcrumb.
type CathedralPayload struct {
	TraceID  string             `json:"traceId"`
	Source   string             `json:"source"` // "persisted" | "demo" | "none"
	Bundle   *SpeculativeBundle `json:"bundle,omitempty"`
	Headline string             `json:"headline,omitempty"`
	Note     string             `json:"note,omitempty"`
}

// demoCohort lazily builds (and memoises) a small reference cohort
// used when a trace has no persisted speculative bundle. It guarantees
// the cathedral never renders blank on a fresh DB — the whole UI tells
// a story even before the runtime has produced its first cohort.
var (
	demoCohortOnce   sync.Once
	demoCohortBundle *SpeculativeBundle
)

func getDemoCathedralBundle() *SpeculativeBundle {
	demoCohortOnce.Do(func() {
		mk := func(id string, suffix string, conf float64, includeConfounder bool) LabeledVerdict {
			nodes := []CausalNode{
				{ID: NodeID("ev-" + suffix), Role: NodeRoleEvidence,
					Summary: "observation logs cluster", Weight: 0.72},
				{ID: NodeID("iv-" + suffix), Role: NodeRoleIntervention,
					Summary: "rollout-canary",
					Weight:  0.50, Parents: []NodeID{NodeID("ev-" + suffix)}},
				{ID: NodeID("ot-" + suffix), Role: NodeRoleOutcome,
					Summary: "regression bounded",
					Weight:  0.66, Parents: []NodeID{NodeID("iv-" + suffix)}},
			}
			if includeConfounder {
				nodes = append(nodes, CausalNode{
					ID: NodeID("cf-" + suffix), Role: NodeRoleConfounder,
					Summary: "drifted prior", Weight: 0.40,
				})
			}
			return LabeledVerdict{
				ID: id,
				Verdict: GISTVerdict{
					Verdict:     "approved",
					Confidence:  conf,
					CausalGraph: &CausalGraph{Nodes: nodes},
				},
			}
		}

		alpha := mk("alpha", "main", 0.86, false)
		beta := mk("beta", "main", 0.83, false)
		gamma := mk("gamma", "main", 0.81, false)
		delta := mk("delta", "drift", 0.72, true) // dissenter

		meta := mk("meta", "main", 0.88, false)

		demoCohortBundle = BuildSpeculativeBundle(SpeculativeBuildInput{
			CohortID: "demo-cohort",
			Verdicts: []LabeledVerdict{alpha, beta, gamma, delta},
			Meta:     &meta,
		})
	})
	return demoCohortBundle
}

func loadCathedralPayload(ctx context.Context, fetcher GISTTraceFetcher, traceID string) (*CathedralPayload, error) {
	id := strings.TrimSpace(traceID)
	if id == "" {
		// Zero-id case — used when the user lands on /lattice/spec
		// directly. We render the demo cohort so the page demonstrates
		// the geometry vocabulary without requiring a trace.
		return &CathedralPayload{
			TraceID:  "demo",
			Source:   "demo",
			Bundle:   getDemoCathedralBundle(),
			Headline: "demo",
			Note:     "Embedded sample cohort. Persist a trace with a SpeculativeBundle to render a real cohort.",
		}, nil
	}
	if id == "demo" {
		return &CathedralPayload{
			TraceID:  "demo",
			Source:   "demo",
			Bundle:   getDemoCathedralBundle(),
			Headline: getDemoCathedralBundle().HeadlineStatus(),
			Note:     "Embedded sample cohort. Replace with /lattice/spec/<your-trace-id>.",
		}, nil
	}
	sf, ok := fetcher.(SpeculativeFetcher)
	if !ok {
		return &CathedralPayload{
			TraceID:  id,
			Source:   "demo",
			Bundle:   getDemoCathedralBundle(),
			Headline: "demo",
			Note:     "Trace fetcher does not implement SpeculativeFetcher; serving demo cohort.",
		}, nil
	}
	bundle, err := sf.GetSpeculativeBundle(ctx, id)
	if errors.Is(err, ErrTraceNotFound) {
		return nil, ErrTraceNotFound
	}
	if err != nil {
		return nil, err
	}
	if bundle == nil {
		return &CathedralPayload{
			TraceID:  id,
			Source:   "none",
			Bundle:   getDemoCathedralBundle(),
			Headline: "demo",
			Note:     "Trace exists but has no SpeculativeBundle persisted; rendering demo cohort.",
		}, nil
	}
	return &CathedralPayload{
		TraceID:  id,
		Source:   "persisted",
		Bundle:   bundle,
		Headline: bundle.HeadlineStatus(),
	}, nil
}

func (s *DirectorHTTPServer) handleCathedralView(w http.ResponseWriter, r *http.Request) {
	fetcher := s.director.TraceFetcher()
	id := strings.TrimPrefix(r.URL.Path, "/lattice/spec/")
	id = strings.TrimSuffix(id, "/")
	if strings.Contains(id, "/") {
		// Disallow nested paths under /lattice/spec/.
		http.Redirect(w, r, "/lattice/spec/"+strings.SplitN(id, "/", 2)[0], http.StatusSeeOther)
		return
	}
	payload, err := loadCathedralPayload(r.Context(), fetcher, id)
	if errors.Is(err, ErrTraceNotFound) {
		http.Error(w, "trace not found: "+id, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "cathedral lookup failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	rawJSON, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "cathedral encode failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	tokenHint := ""
	if s.cfg.Token != "" {
		tokenHint = "?token=" + template.URLQueryEscaper(s.cfg.Token)
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	_ = cathedralPageTemplate.Execute(w, map[string]any{
		"TraceID":   payload.TraceID,
		"Source":    payload.Source,
		"Headline":  payload.Headline,
		"Note":      payload.Note,
		"DataJSON":  template.JS(string(rawJSON)),
		"TokenHint": tokenHint,
	})
}

func (s *DirectorHTTPServer) handleAPICathedralData(w http.ResponseWriter, r *http.Request) {
	fetcher := s.director.TraceFetcher()
	path := strings.TrimPrefix(r.URL.Path, "/lattice/spec/")
	path = strings.TrimSuffix(path, "/data")
	id := strings.TrimSuffix(path, "/")
	payload, err := loadCathedralPayload(r.Context(), fetcher, id)
	if errors.Is(err, ErrTraceNotFound) {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not_found", "id": id})
		return
	}
	writeJSON(w, payload, err)
}

// cathedralPageTemplate is the inline HTML / Three.js scene served at
// /lattice/spec/{id}. It loads Three.js from CDN, renders a hex-tile
// cohort floor, evidence orbs / confounder fog / intervention arrows /
// outcome spires per peer, an attestation crystal above each tile,
// and a central convergence crystal that is whole / cracked / shattered
// based on the report status. Reconciliation drift / fabrication are
// drawn as lightning arcs from meta to mismatched nodes.
var cathedralPageTemplate = template.Must(template.New("cathedral").Parse(cathedralPageHTML))

const cathedralPageHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<title>Lattice Cathedral · {{.TraceID}}</title>
<style>
:root {
  --bg-1: #05080f;
  --bg-2: #0b1220;
  --fg: #e8edf6;
  --dim: #94a3b8;
  --cyan: #5eead4;
  --gold: #fbbf24;
  --red: #f87171;
  --amber: #f59e0b;
  --violet: #c4b5fd;
  --grey: #475569;
}
* { box-sizing: border-box; }
html, body {
  margin: 0; padding: 0; height: 100%; background: var(--bg-1); color: var(--fg);
  font: 14px/1.4 system-ui, -apple-system, "SF Pro Text", "Inter", sans-serif;
  overflow: hidden;
}
#scene { position: fixed; inset: 0; width: 100vw; height: 100vh; display: block; }
#topbar {
  position: fixed; top: 0; left: 0; right: 0; z-index: 10;
  display: flex; gap: 14px; align-items: center;
  padding: 14px 18px;
  background: linear-gradient(180deg, rgba(5,8,15,0.92), rgba(5,8,15,0));
  pointer-events: none;
}
#topbar .pill {
  pointer-events: auto;
  display: inline-flex; align-items: center; gap: 6px;
  padding: 5px 11px; border-radius: 999px;
  background: rgba(15,23,42,0.66); border: 1px solid rgba(148,163,184,0.20);
  font-size: 12px; letter-spacing: 0.02em;
}
#topbar .pill.label { color: var(--dim); }
.dot { width: 8px; height: 8px; border-radius: 50%; display: inline-block; }
.dot.cohort { background: var(--cyan); box-shadow: 0 0 10px var(--cyan); }
.dot.partial { background: var(--amber); box-shadow: 0 0 10px var(--amber); }
.dot.divergent { background: var(--red); box-shadow: 0 0 10px var(--red); }
.dot.demo { background: var(--violet); box-shadow: 0 0 10px var(--violet); }
.dot.fabricated { background: var(--red); box-shadow: 0 0 10px var(--red); }
#sidebar {
  position: fixed; top: 64px; right: 14px; width: 290px; max-height: 78vh;
  z-index: 9; overflow: auto; pointer-events: auto;
  background: rgba(11,18,32,0.78); backdrop-filter: blur(8px);
  border: 1px solid rgba(148,163,184,0.18); border-radius: 12px;
  padding: 14px; font-size: 12.5px;
}
#sidebar h3 { margin: 6px 0 10px; font-size: 12px; text-transform: uppercase;
  letter-spacing: 0.08em; color: var(--dim); }
#sidebar .row { display: flex; justify-content: space-between; gap: 10px;
  padding: 4px 0; border-bottom: 1px solid rgba(148,163,184,0.10); }
#sidebar .row:last-child { border-bottom: none; }
#sidebar .row .k { color: var(--dim); }
#sidebar .row .v { color: var(--fg); font-feature-settings: "tnum"; }
#sidebar .legend-key {
  display: inline-flex; align-items: center; gap: 6px; margin-right: 12px;
}
#sidebar .legend-swatch {
  width: 10px; height: 10px; border-radius: 50%; display: inline-block;
}
#tooltip {
  position: fixed; pointer-events: none; z-index: 20;
  padding: 7px 10px; border-radius: 8px;
  background: rgba(11,18,32,0.92); border: 1px solid rgba(148,163,184,0.32);
  font-size: 12px; color: var(--fg); max-width: 240px;
  display: none;
}
#tooltip .head { color: var(--cyan); font-weight: 600; }
#tooltip .role { color: var(--dim); font-size: 11px; }
#footer {
  position: fixed; left: 14px; bottom: 12px; z-index: 9;
  font-size: 11px; color: var(--dim);
}
#footer a { color: var(--cyan); text-decoration: none; }
#error {
  position: fixed; left: 50%; top: 40%; transform: translate(-50%,-50%);
  background: rgba(11,18,32,0.94); border: 1px solid rgba(248,113,113,0.4);
  border-radius: 10px; padding: 16px 20px; max-width: 480px; z-index: 30;
  display: none; color: var(--red);
}
</style>
</head>
<body>
<canvas id="scene"></canvas>

<div id="topbar">
  <span class="pill label">Lattice Cathedral</span>
  <span class="pill" id="trace-pill">trace: {{.TraceID}}</span>
  <span class="pill" id="status-pill"><span class="dot demo" id="status-dot"></span><span id="status-text">{{.Headline}}</span></span>
  <span class="pill label" id="source-pill">source: {{.Source}}</span>
  {{if .Note}}<span class="pill label" id="note-pill">{{.Note}}</span>{{end}}
</div>

<aside id="sidebar">
  <h3>Cohort</h3>
  <div class="row"><span class="k">Peers</span><span class="v" id="m-peers">—</span></div>
  <div class="row"><span class="k">Convergence</span><span class="v" id="m-converge">—</span></div>
  <div class="row"><span class="k">Quorum</span><span class="v" id="m-quorum">—</span></div>
  <div class="row"><span class="k">Threshold</span><span class="v" id="m-threshold">—</span></div>
  <div class="row"><span class="k">Reconciliation</span><span class="v" id="m-recon">—</span></div>
  <div class="row"><span class="k">Coverage</span><span class="v" id="m-coverage">—</span></div>
  <div class="row"><span class="k">Dyad slots</span><span class="v" id="m-dyad">—</span></div>
  <h3 style="margin-top:14px;">Legend</h3>
  <div>
    <span class="legend-key"><span class="legend-swatch" style="background:var(--cyan);"></span>evidence</span>
    <span class="legend-key"><span class="legend-swatch" style="background:var(--grey);"></span>confounder</span>
    <span class="legend-key"><span class="legend-swatch" style="background:var(--amber);"></span>intervention</span>
    <span class="legend-key"><span class="legend-swatch" style="background:var(--gold);"></span>outcome</span>
  </div>
  <p style="color:var(--dim); margin-top:12px; line-height:1.5;">
    Every shape encodes the typed causal mechanism. Crystal above each
    tile = peer Merkle attestation. Centre crystal = cohort consensus
    root. Lightning = reconciliation drift.
  </p>
</aside>

<div id="tooltip"></div>

<div id="footer">
  drag · scroll · double-click to reset · <a href="/lattice{{.TokenHint}}">↩ all traces</a>
</div>

<div id="error"></div>

<script type="application/json" id="cathedral-data">{{.DataJSON}}</script>

<script src="https://cdnjs.cloudflare.com/ajax/libs/three.js/r128/three.min.js"></script>
<script>
(function () {
  "use strict";

  function showError(msg) {
    var el = document.getElementById("error");
    el.textContent = msg;
    el.style.display = "block";
  }

  if (typeof THREE === "undefined") {
    showError("Three.js failed to load. Check network access to cdnjs.cloudflare.com.");
    return;
  }

  var raw = document.getElementById("cathedral-data").textContent;
  var payload;
  try { payload = JSON.parse(raw); } catch (e) {
    showError("Cathedral payload is not valid JSON: " + e.message);
    return;
  }
  var bundle = payload && payload.bundle ? payload.bundle : null;

  // ---------- topbar status pill ----------
  var statusDot = document.getElementById("status-dot");
  var statusText = document.getElementById("status-text");
  var headline = (payload && payload.headline) || "demo";
  statusText.textContent = headline;
  statusDot.classList.remove("demo", "cohort", "partial", "divergent", "fabricated");
  if (headline === "converged") statusDot.classList.add("cohort");
  else if (headline === "partial") statusDot.classList.add("partial");
  else if (headline === "divergent") statusDot.classList.add("divergent");
  else if (headline === "fabricated") statusDot.classList.add("fabricated");
  else if (headline === "drifted") statusDot.classList.add("partial");
  else statusDot.classList.add("demo");

  // ---------- sidebar metrics ----------
  function setText(id, val) {
    var el = document.getElementById(id);
    if (el) el.textContent = (val === null || val === undefined || val === "") ? "—" : String(val);
  }
  if (bundle) {
    setText("m-peers", (bundle.peers || []).length);
    if (bundle.convergence) {
      setText("m-converge", bundle.convergence.status);
      setText("m-quorum", bundle.convergence.quorumSize + " / " + bundle.convergence.totalPeers);
      setText("m-threshold", (bundle.convergence.threshold || 0).toFixed(2));
    }
    if (bundle.reconciliation) {
      setText("m-recon", bundle.reconciliation.status);
      setText("m-coverage", (bundle.reconciliation.coverage || 0).toFixed(2));
    }
    if (bundle.dyads) {
      setText("m-dyad", bundle.dyads.slotsAfter + " / " + bundle.dyads.slotsBefore);
    }
  }

  // ---------- three.js scene ----------
  var canvas = document.getElementById("scene");
  var renderer = new THREE.WebGLRenderer({ canvas: canvas, antialias: true, alpha: true });
  renderer.setPixelRatio(Math.min(window.devicePixelRatio || 1, 2));
  function resize() {
    var w = window.innerWidth, h = window.innerHeight;
    renderer.setSize(w, h, false);
    camera.aspect = w / h; camera.updateProjectionMatrix();
  }
  var scene = new THREE.Scene();
  scene.background = null;
  scene.fog = new THREE.FogExp2(0x05080f, 0.018);

  var camera = new THREE.PerspectiveCamera(48, 1, 0.1, 200);
  camera.position.set(0, 14, 28);
  camera.lookAt(0, 4, 0);
  window.addEventListener("resize", resize);

  // Lighting: cool ambient, warm key from above, rim from below.
  scene.add(new THREE.AmbientLight(0x223047, 0.7));
  var key = new THREE.DirectionalLight(0xfff1cc, 1.0);
  key.position.set(8, 18, 12); scene.add(key);
  var rim = new THREE.PointLight(0x5eead4, 0.8, 80); rim.position.set(0, 4, -10); scene.add(rim);

  // Floor: large dark hex grid as a static prop.
  (function () {
    var floor = new THREE.Mesh(
      new THREE.CircleGeometry(40, 64),
      new THREE.MeshStandardMaterial({ color: 0x070b14, roughness: 1, metalness: 0 })
    );
    floor.rotation.x = -Math.PI / 2;
    floor.position.y = -0.05;
    scene.add(floor);
    var grid = new THREE.GridHelper(60, 30, 0x1f2a40, 0x0d1424);
    grid.position.y = 0; scene.add(grid);
  })();

  // ---------- color helpers ----------
  var COL = {
    evidence:     0x5eead4,
    confounder:   0x64748b,
    intervention: 0xf59e0b,
    outcome:      0xfbbf24,
    cohort:       0x5eead4,
    dissent:      0xf87171,
    meta:         0xc4b5fd,
    converge:     0xfbbf24,
    crack:        0xf87171
  };

  // Hex prism geometry (radius, height) as a tile under each peer.
  function hexPrism(r, h) {
    return new THREE.CylinderGeometry(r, r, h, 6, 1, false);
  }

  // Build a Three.js group representing one peer's typed CausalGraph.
  // Geometry encodes the role: evidence orbs, confounder fog spheres,
  // intervention cones, outcome spires. Position derived from a stable
  // hash of NodeID so the same node always lands in the same place.
  function nodeHash(s) {
    var h = 0;
    for (var i = 0; i < s.length; i++) {
      h = ((h << 5) - h + s.charCodeAt(i)) | 0;
    }
    return h;
  }
  function buildPeerGraphGroup(peer) {
    var g = new THREE.Group();
    var nodes = (peer.graph && peer.graph.nodes) ? peer.graph.nodes : [];
    var roleY = { evidence: 1.2, confounder: 1.6, intervention: 2.4, outcome: 3.4, unknown: 1.0 };
    var meshes = [];
    nodes.forEach(function (n) {
      var h = nodeHash(String(n.id || ""));
      var ang = ((h % 360) + 360) % 360 * Math.PI / 180;
      var rad = 0.85 + ((h >> 8) & 0x7) * 0.12;
      var x = Math.cos(ang) * rad;
      var z = Math.sin(ang) * rad;
      var y = roleY[(n.role || "unknown")] || 1.5;
      var mesh;
      var w = (typeof n.weight === "number" && isFinite(n.weight)) ? n.weight : 0.5;
      if (n.role === "evidence") {
        mesh = new THREE.Mesh(
          new THREE.SphereGeometry(0.18 + 0.30 * w, 18, 14),
          new THREE.MeshStandardMaterial({
            color: COL.evidence, emissive: COL.evidence,
            emissiveIntensity: 0.65, roughness: 0.35, metalness: 0.1
          })
        );
      } else if (n.role === "confounder") {
        mesh = new THREE.Mesh(
          new THREE.SphereGeometry(0.40 + 0.30 * w, 16, 12),
          new THREE.MeshStandardMaterial({
            color: COL.confounder, transparent: true, opacity: 0.45,
            roughness: 0.9, metalness: 0
          })
        );
      } else if (n.role === "intervention") {
        mesh = new THREE.Mesh(
          new THREE.ConeGeometry(0.22, 0.85, 16),
          new THREE.MeshStandardMaterial({
            color: COL.intervention, emissive: COL.intervention,
            emissiveIntensity: 0.45
          })
        );
      } else if (n.role === "outcome") {
        mesh = new THREE.Mesh(
          new THREE.ConeGeometry(0.36, 1.5, 16),
          new THREE.MeshStandardMaterial({
            color: COL.outcome, emissive: COL.outcome,
            emissiveIntensity: 0.55
          })
        );
      } else {
        mesh = new THREE.Mesh(
          new THREE.SphereGeometry(0.20, 12, 10),
          new THREE.MeshStandardMaterial({ color: 0x6b7280 })
        );
      }
      mesh.position.set(x, y, z);
      mesh.userData = {
        kind: "node",
        peerId: peer.agentId,
        nodeId: n.id, role: n.role,
        summary: n.summary, weight: w
      };
      g.add(mesh);
      meshes.push(mesh);
    });
    g.userData = { kind: "peer-graph", peerId: peer.agentId, meshes: meshes };
    return g;
  }

  // Crystal above each peer tile: octahedron coloured by cohort flag.
  function buildPeerCrystal(peer) {
    var col = peer.inCohort ? COL.cohort : COL.dissent;
    var mat = new THREE.MeshStandardMaterial({
      color: col, emissive: col, emissiveIntensity: 0.55,
      roughness: 0.25, metalness: 0.5,
      transparent: !peer.inCohort, opacity: peer.inCohort ? 1 : 0.85
    });
    var geom = new THREE.OctahedronGeometry(0.45);
    var crystal = new THREE.Mesh(geom, mat);
    crystal.userData = {
      kind: "crystal", peerId: peer.agentId,
      summary: "Merkle root: " + (peer.attestation && peer.attestation.root ? peer.attestation.root.slice(0, 16) + "…" : "—"),
      role: peer.inCohort ? "consensus" : "dissent"
    };
    if (!peer.inCohort) {
      var edges = new THREE.LineSegments(
        new THREE.EdgesGeometry(geom),
        new THREE.LineBasicMaterial({ color: COL.crack })
      );
      crystal.add(edges);
    }
    return crystal;
  }

  // Single peer assembly: floor tile + graph group + crystal.
  function buildPeerAssembly(peer, theta) {
    var radius = 6.0;
    var x = Math.cos(theta) * radius;
    var z = Math.sin(theta) * radius;
    var grp = new THREE.Group();
    grp.position.set(x, 0, z);

    var tileColor = peer.inCohort ? 0x10243a : 0x281015;
    var tile = new THREE.Mesh(
      hexPrism(2.4, 0.3),
      new THREE.MeshStandardMaterial({
        color: tileColor, roughness: 0.6, metalness: 0.15,
        emissive: peer.inCohort ? 0x0e2c46 : 0x381820, emissiveIntensity: 0.4
      })
    );
    tile.position.y = 0.15;
    tile.userData = {
      kind: "tile", peerId: peer.agentId,
      summary: "verdict: " + (peer.verdict || "—") + "  conf: " + ((peer.confidence || 0).toFixed(2)),
      role: peer.inCohort ? "consensus" : "dissent"
    };
    grp.add(tile);

    var graphGroup = buildPeerGraphGroup(peer);
    grp.add(graphGroup);

    var crystal = buildPeerCrystal(peer);
    crystal.position.set(0, 5.0, 0);
    grp.add(crystal);

    grp.userData = { kind: "peer", peerId: peer.agentId, crystal: crystal };
    return grp;
  }

  // Centre cohort crystal: convergence root.
  function buildCohortCrystal(convergence) {
    var status = (convergence && convergence.status) || "unknown";
    var size = 1.4;
    var color = COL.converge;
    if (status === "partial") color = COL.intervention;
    if (status === "divergent") color = COL.dissent;
    var mat = new THREE.MeshStandardMaterial({
      color: color, emissive: color, emissiveIntensity: 0.85,
      roughness: 0.2, metalness: 0.5
    });
    var geom = new THREE.OctahedronGeometry(size, status === "divergent" ? 1 : 0);
    var crystal = new THREE.Mesh(geom, mat);
    crystal.position.set(0, 8.0, 0);
    crystal.userData = {
      kind: "cohort-crystal",
      summary: "consensus root: " + ((convergence && convergence.consensusRoot) ?
        convergence.consensusRoot.slice(0, 16) + "…" : "—"),
      role: status
    };
    if (status !== "converged") {
      var edges = new THREE.LineSegments(
        new THREE.EdgesGeometry(geom),
        new THREE.LineBasicMaterial({ color: COL.crack })
      );
      crystal.add(edges);
    }
    return crystal;
  }

  // Meta tile: floats above center if a meta is present.
  function buildMetaAssembly(meta, reconciliation) {
    if (!meta) return null;
    var grp = new THREE.Group();
    grp.position.set(0, 11.5, 0);
    var tile = new THREE.Mesh(
      hexPrism(2.6, 0.25),
      new THREE.MeshStandardMaterial({
        color: 0x1a1730, emissive: 0x352b66, emissiveIntensity: 0.45,
        roughness: 0.5, metalness: 0.3
      })
    );
    tile.userData = {
      kind: "meta-tile",
      summary: "meta verdict: " + (meta.verdict || "—") + "  conf: " + ((meta.confidence || 0).toFixed(2)),
      role: (reconciliation && reconciliation.status) || "—"
    };
    grp.add(tile);

    // mini-graph for the meta
    var sub = buildPeerGraphGroup({
      agentId: meta.agentId, graph: meta.graph,
      attestation: meta.attestation, inCohort: true
    });
    sub.scale.set(0.85, 0.85, 0.85);
    sub.position.y = 0.3;
    grp.add(sub);

    return grp;
  }

  // Lightning arc from origin to target — used for reconciliation drift.
  function lightning(origin, target, color) {
    var pts = [];
    var seg = 16;
    for (var i = 0; i <= seg; i++) {
      var t = i / seg;
      var jx = (Math.random() - 0.5) * 0.35 * (1 - Math.abs(t - 0.5) * 2);
      var jy = (Math.random() - 0.5) * 0.35 * (1 - Math.abs(t - 0.5) * 2);
      var jz = (Math.random() - 0.5) * 0.35 * (1 - Math.abs(t - 0.5) * 2);
      pts.push(new THREE.Vector3(
        origin.x + (target.x - origin.x) * t + jx,
        origin.y + (target.y - origin.y) * t + jy,
        origin.z + (target.z - origin.z) * t + jz
      ));
    }
    var line = new THREE.Line(
      new THREE.BufferGeometry().setFromPoints(pts),
      new THREE.LineBasicMaterial({ color: color, linewidth: 2 })
    );
    return line;
  }

  // ---------- assemble ----------
  var peers = (bundle && bundle.peers) || [];
  var peerGroups = [];
  peers.forEach(function (p, i) {
    var theta = (i / Math.max(1, peers.length)) * Math.PI * 2;
    var asm = buildPeerAssembly(p, theta);
    scene.add(asm);
    peerGroups.push({ peer: p, group: asm });
  });

  if (bundle && bundle.convergence) {
    var cohortCrystal = buildCohortCrystal(bundle.convergence);
    scene.add(cohortCrystal);
    window.__cohortCrystal = cohortCrystal;
  }

  if (bundle && bundle.meta) {
    var metaAsm = buildMetaAssembly(bundle.meta, bundle.reconciliation);
    if (metaAsm) {
      scene.add(metaAsm);

      // Reconciliation lightning: from meta down to each problematic peer.
      var rec = bundle.reconciliation || {};
      var bad = (rec.nodes || []).filter(function (n) {
        return n.status === "fabricated" || n.status === "drifted";
      });
      bad.forEach(function () {
        // Pick a random peer position to attach lightning to, since
        // node→peer is many-to-many. Visually conveys "meta argued
        // something the cohort can't anchor".
        var idx = Math.floor(Math.random() * peerGroups.length);
        var pg = peerGroups[idx];
        if (!pg) return;
        var origin = new THREE.Vector3(0, 11.5, 0);
        var target = new THREE.Vector3(
          pg.group.position.x, 5.0, pg.group.position.z
        );
        scene.add(lightning(origin, target, COL.dissent));
      });
    }
  }

  // ---------- raycaster for hover tooltips ----------
  var raycaster = new THREE.Raycaster();
  var mouseNDC = new THREE.Vector2(2, 2); // off-screen by default
  var tooltip = document.getElementById("tooltip");
  function clientToNDC(ev) {
    var rect = canvas.getBoundingClientRect();
    mouseNDC.x = ((ev.clientX - rect.left) / rect.width) * 2 - 1;
    mouseNDC.y = -((ev.clientY - rect.top) / rect.height) * 2 + 1;
    tooltip.style.left = (ev.clientX + 14) + "px";
    tooltip.style.top = (ev.clientY + 14) + "px";
  }
  canvas.addEventListener("mousemove", clientToNDC);
  canvas.addEventListener("mouseleave", function () {
    tooltip.style.display = "none";
    mouseNDC.set(2, 2);
  });

  // ---------- minimal orbit controls (drag rotate, scroll zoom, dblclick reset) ----------
  var orbit = {
    target: new THREE.Vector3(0, 4, 0),
    radius: 28, minR: 12, maxR: 60,
    azimuth: 0, polar: Math.PI * 0.32,
    minPolar: 0.05, maxPolar: Math.PI * 0.49
  };
  function applyOrbit() {
    var sp = Math.sin(orbit.polar), cp = Math.cos(orbit.polar);
    var sa = Math.sin(orbit.azimuth), ca = Math.cos(orbit.azimuth);
    camera.position.set(
      orbit.target.x + orbit.radius * sp * sa,
      orbit.target.y + orbit.radius * cp,
      orbit.target.z + orbit.radius * sp * ca
    );
    camera.lookAt(orbit.target);
  }
  applyOrbit();
  var dragging = false, lx = 0, ly = 0;
  canvas.addEventListener("mousedown", function (ev) {
    dragging = true; lx = ev.clientX; ly = ev.clientY;
  });
  window.addEventListener("mouseup", function () { dragging = false; });
  window.addEventListener("mousemove", function (ev) {
    if (!dragging) return;
    var dx = ev.clientX - lx, dy = ev.clientY - ly;
    lx = ev.clientX; ly = ev.clientY;
    orbit.azimuth -= dx * 0.005;
    orbit.polar = Math.max(orbit.minPolar, Math.min(orbit.maxPolar, orbit.polar - dy * 0.005));
    applyOrbit();
  });
  canvas.addEventListener("wheel", function (ev) {
    ev.preventDefault();
    orbit.radius = Math.max(orbit.minR, Math.min(orbit.maxR, orbit.radius + ev.deltaY * 0.02));
    applyOrbit();
  }, { passive: false });
  canvas.addEventListener("dblclick", function () {
    orbit.azimuth = 0; orbit.polar = Math.PI * 0.32; orbit.radius = 28;
    applyOrbit();
  });

  // ---------- main loop ----------
  function tick(t) {
    requestAnimationFrame(tick);
    // gentle rotation of attestation crystals
    peerGroups.forEach(function (pg) {
      if (pg.group.userData && pg.group.userData.crystal) {
        pg.group.userData.crystal.rotation.y += 0.005;
        pg.group.userData.crystal.rotation.x += 0.002;
      }
    });
    if (window.__cohortCrystal) {
      window.__cohortCrystal.rotation.y += 0.003;
      window.__cohortCrystal.rotation.x += 0.001;
      window.__cohortCrystal.position.y = 8.0 + Math.sin(t * 0.0009) * 0.18;
    }

    // raycast for hover tooltip
    raycaster.setFromCamera(mouseNDC, camera);
    var hits = raycaster.intersectObjects(scene.children, true);
    var hover = null;
    for (var i = 0; i < hits.length; i++) {
      var ud = hits[i].object.userData;
      if (ud && ud.kind) { hover = hits[i].object; break; }
    }
    if (hover) {
      var d = hover.userData || {};
      var head = d.peerId || d.kind || "node";
      var role = d.role ? (" · " + d.role) : "";
      var sub = d.summary || "";
      var weight = (typeof d.weight === "number") ? (" · w=" + d.weight.toFixed(2)) : "";
      tooltip.innerHTML = "<div class=\"head\">" + head + "<span class=\"role\">" + role + weight + "</span></div>" +
        (sub ? ("<div>" + sub + "</div>") : "");
      tooltip.style.display = "block";
    } else {
      tooltip.style.display = "none";
    }

    renderer.render(scene, camera);
  }

  resize();
  requestAnimationFrame(tick);
})();
</script>
</body>
</html>
`

// handleLatticeRouter dispatches `/lattice/...` requests between the
// existing Phase 3 inspector view (`/lattice/{id}`) and the Phase 6
// cathedral routes (`/lattice/spec/{id}` and `/lattice/spec/{id}/data`).
// It owns the prefix mounted at "/lattice/" because Go's net/http mux
// is prefix-based and we don't want to register overlapping handlers.
func (s *DirectorHTTPServer) handleLatticeRouter(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/lattice/")
	switch {
	case rest == "spec" || rest == "spec/":
		s.handleCathedralView(w, r)
		return
	case strings.HasPrefix(rest, "spec/"):
		// /lattice/spec/<id> or /lattice/spec/<id>/data
		if strings.HasSuffix(rest, "/data") {
			s.handleAPICathedralData(w, r)
			return
		}
		s.handleCathedralView(w, r)
		return
	default:
		s.handleLatticeView(w, r)
		return
	}
}
