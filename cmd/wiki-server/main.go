// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the MIT License.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Package main implements wiki-server: a simple HTTP server that renders
// markdown files from docs/wiki/ as styled HTML pages with sidebar navigation.
//
// Usage:
//
//	wiki-server [--port 20002] [--wiki-dir ./docs/wiki]
//
// Endpoints:
//
//	GET /              Redirect to /wiki/
//	GET /wiki/         Wiki homepage (README.md)
//	GET /wiki/{page}   Render docs/wiki/{page}.md as HTML
//	GET /health        Health check (200 OK)
//	GET /ready         Readiness probe (200 OK)
//
// Version: 0.1.0-alpha
package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"

	"unheaded/pkg/auth"
)

const (
	serviceVersion = "0.1.0-alpha"
	serviceName    = "wiki-server"
)

// logf writes a structured log message to stderr.
func logf(level, msg string, kvPairs ...interface{}) {
	timestamp := time.Now().Format(time.RFC3339)
	fmt.Fprintf(os.Stderr, "[%s] %s: %s", timestamp, level, msg)
	for i := 0; i+1 < len(kvPairs); i += 2 {
		fmt.Fprintf(os.Stderr, " %v=%v", kvPairs[i], kvPairs[i+1])
	}
	fmt.Fprintln(os.Stderr)
}

// WikiServer serves markdown files as HTML.
type WikiServer struct {
	wikiDir  string
	md       goldmark.Markdown
	tmpl     *template.Template
	startTime time.Time
}

// NavItem represents a sidebar navigation entry.
type NavItem struct {
	Slug  string
	Title string
}

// PageData holds data passed to the HTML template.
type PageData struct {
	Title      string
	Content    template.HTML
	NavItems   []NavItem
	ActiveSlug string
	Version    string
}

// NewWikiServer creates a new wiki server.
func NewWikiServer(wikiDir string) (*WikiServer, error) {
	absDir, err := filepath.Abs(wikiDir)
	if err != nil {
		return nil, fmt.Errorf("resolve wiki dir: %w", err)
	}

	info, err := os.Stat(absDir)
	if err != nil {
		return nil, fmt.Errorf("stat wiki dir %s: %w", absDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("wiki dir %s is not a directory", absDir)
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Table),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)

	tmpl, err := template.New("page").Parse(pageTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	return &WikiServer{
		wikiDir:   absDir,
		md:        md,
		tmpl:      tmpl,
		startTime: time.Now(),
	}, nil
}

// listPages returns all markdown files in the wiki directory as NavItems.
func (ws *WikiServer) listPages() []NavItem {
	entries, err := os.ReadDir(ws.wikiDir)
	if err != nil {
		logf("ERROR", "list wiki dir", "err", err)
		return nil
	}

	var items []NavItem
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		if name == "README" {
			items = append(items, NavItem{Slug: "", Title: "Home"})
			continue
		}
		title := slugToTitle(name)
		items = append(items, NavItem{Slug: name, Title: title})
	}

	// Sort: Home first, then alphabetical.
	sort.Slice(items, func(i, j int) bool {
		if items[i].Slug == "" {
			return true
		}
		if items[j].Slug == "" {
			return false
		}
		return items[i].Title < items[j].Title
	})

	return items
}

// slugToTitle converts a file slug to a display title.
func slugToTitle(slug string) string {
	words := strings.Split(slug, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// renderPage reads a markdown file and renders it as HTML.
func (ws *WikiServer) renderPage(slug string) (string, string, error) {
	filename := "README.md"
	if slug != "" {
		filename = slug + ".md"
	}

	path := filepath.Join(ws.wikiDir, filename)

	// Prevent path traversal.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}
	if !strings.HasPrefix(absPath, ws.wikiDir) {
		return "", "", fmt.Errorf("path traversal attempt blocked")
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", "", fmt.Errorf("read file %s: %w", absPath, err)
	}

	// Extract title from first heading.
	title := slugToTitle(slug)
	if slug == "" {
		title = "Welcome to the Kingdom"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "# ") {
			title = strings.TrimPrefix(line, "# ")
			break
		}
	}

	var buf bytes.Buffer
	if err := ws.md.Convert(data, &buf); err != nil {
		return "", "", fmt.Errorf("render markdown: %w", err)
	}

	return title, buf.String(), nil
}

// handleWiki serves wiki pages.
func (ws *WikiServer) handleWiki(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/wiki/")
	path = strings.TrimSuffix(path, "/")

	title, content, err := ws.renderPage(path)
	if err != nil {
		if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") {
			http.Error(w, "Page not found", http.StatusNotFound)
			return
		}
		logf("ERROR", "render page", "slug", path, "err", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := PageData{
		Title:      title,
		Content:    template.HTML(content),
		NavItems:   ws.listPages(),
		ActiveSlug: path,
		Version:    serviceVersion,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ws.tmpl.Execute(w, data); err != nil {
		logf("ERROR", "execute template", "err", err)
	}
}

// handleHealth returns a health check response.
func (ws *WikiServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"healthy","service":"%s","version":"%s","uptime":"%s"}`,
		serviceName, serviceVersion, time.Since(ws.startTime).Round(time.Second))
}

// handleReady returns a readiness probe response.
func (ws *WikiServer) handleReady(w http.ResponseWriter, _ *http.Request) {
	// Check that wiki directory is readable.
	if _, err := os.ReadDir(ws.wikiDir); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"not_ready","error":"%s"}`, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ready","service":"%s","wiki_dir":"%s"}`, serviceName, ws.wikiDir)
}

func main() {
	port := flag.Int("port", 20002, "HTTP server port")
	wikiDir := flag.String("wiki-dir", "./docs/wiki", "Path to wiki markdown directory")
	flag.Parse()

	logf("INFO", "starting wiki-server",
		"version", serviceVersion,
		"port", *port,
		"wiki_dir", *wikiDir,
	)

	ws, err := NewWikiServer(*wikiDir)
	if err != nil {
		logf("FATAL", "failed to create wiki server", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	// Root redirects to wiki.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.Redirect(w, r, "/wiki/", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	})

	// Wiki pages.
	mux.HandleFunc("/wiki/", ws.handleWiki)

	// Health and readiness.
	mux.HandleFunc("/health", ws.handleHealth)
	mux.HandleFunc("/ready", ws.handleReady)

	// Auth middleware (activated via AUTH_ENABLED=true)
	authCfg := auth.LoadServiceAuthConfig("wiki-server")
	var httpHandler http.Handler = auth.WrapHandler(mux, auth.SetupMiddleware(authCfg))

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", *port),
		Handler:           httpHandler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	// Graceful shutdown.
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)

	go func() {
		logf("INFO", "wiki-server listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logf("FATAL", "server error", "err", err)
			os.Exit(1)
		}
	}()

	<-done
	logf("INFO", "shutting down wiki-server")
}

// pageTemplate is the HTML template for rendering wiki pages.
const pageTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>{{.Title}} - Unheaded Wiki</title>
    <style>
        :root {
            /* Design-system tokens (inlined — wiki is standalone) */
            --bg-primary:    #0a0a0a;
            --bg-secondary:  #111111;
            --bg-tertiary:   #161616;
            --bg-glass:      rgba(22, 22, 22, 0.8);
            --text-primary:  #c9c9c9;
            --text-secondary:#666666;
            --text-heading:  #ffffff;
            --text-dim:      #666666;
            --text-ghost:    #3a3a3a;
            --text-code:     #a0a0a0;
            --accent:        #666666;
            --accent-hover:  #c9c9c9;
            --border:        #222222;
            --border-card:   #2a2a2a;
            --code-bg:       #111111;
            --link:          #666666;
            --link-hover:    #c9c9c9;
            --font-mono:     'JetBrains Mono', 'Courier New', Courier, monospace;
            --font-body:     'Space Grotesk', system-ui, -apple-system, sans-serif;
            --nav-height:    48px;
        }

        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        html {
            -webkit-font-smoothing: antialiased;
            -moz-osx-font-smoothing: grayscale;
        }

        body {
            font-family: var(--font-mono);
            background: var(--bg-primary);
            color: var(--text-primary);
            line-height: 1.8;
            font-size: 0.875rem;
        }

        /* Particle canvas */
        #particle-canvas {
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            z-index: 0;
            pointer-events: none;
        }

        /* Nav bar — frosted glass */
        .nav {
            position: sticky;
            top: 0;
            z-index: 100;
            height: var(--nav-height);
            background: var(--bg-glass);
            backdrop-filter: blur(12px);
            -webkit-backdrop-filter: blur(12px);
            border-bottom: 1px solid var(--border);
            display: flex;
            align-items: center;
            padding: 0 2rem;
            gap: 2rem;
        }

        .nav-logo {
            font-family: var(--font-mono);
            font-size: 0.75rem;
            color: var(--text-dim);
            letter-spacing: 0.3em;
            text-decoration: none;
            text-transform: lowercase;
            transition: color 0.25s ease;
        }

        .nav-logo:hover { color: var(--text-primary); }
        .nav-logo span { color: var(--text-primary); }

        .nav-links {
            display: flex;
            gap: 1.5rem;
            list-style: none;
        }

        .nav-link {
            font-family: var(--font-mono);
            font-size: 0.75rem;
            color: var(--text-dim);
            letter-spacing: 0.1em;
            text-decoration: none;
            transition: color 0.25s ease;
        }

        .nav-link:hover, .nav-link.active { color: var(--text-primary); }

        .layout {
            display: flex;
            min-height: calc(100vh - var(--nav-height));
            position: relative;
            z-index: 1;
        }

        /* Sidebar */
        .sidebar {
            width: 240px;
            min-width: 240px;
            background: var(--bg-secondary);
            border-right: 1px solid var(--border);
            padding: 24px 0;
            position: sticky;
            top: var(--nav-height);
            height: calc(100vh - var(--nav-height));
            overflow-y: auto;
        }

        .sidebar-brand {
            padding: 0 20px 20px;
            border-bottom: 1px solid var(--border);
            margin-bottom: 16px;
        }

        .sidebar-brand h1 {
            font-family: var(--font-mono);
            font-size: 0.75rem;
            color: var(--text-dim);
            font-weight: 300;
            letter-spacing: 0.2em;
            text-transform: uppercase;
        }

        .sidebar-brand .subtitle {
            font-size: 0.625rem;
            color: var(--text-ghost);
            margin-top: 4px;
            letter-spacing: 0.1em;
            transition: color 0.5s ease;
        }

        .sidebar-brand .subtitle:hover {
            color: var(--text-secondary);
        }

        .sidebar nav ul {
            list-style: none;
        }

        .sidebar nav a {
            display: block;
            padding: 6px 20px;
            color: var(--text-ghost);
            text-decoration: none;
            font-family: var(--font-mono);
            font-size: 0.75rem;
            letter-spacing: 0.1em;
            border-left: 2px solid transparent;
            transition: all 0.25s ease;
        }

        .sidebar nav a:hover {
            color: var(--text-primary);
            background: var(--bg-tertiary);
        }

        .sidebar nav a.active {
            color: var(--text-primary);
            border-left-color: var(--border-card);
            background: var(--bg-tertiary);
        }

        .sidebar-footer {
            padding: 16px 20px;
            border-top: 1px solid var(--border);
            margin-top: 16px;
            font-size: 0.625rem;
            color: var(--text-ghost);
            letter-spacing: 0.1em;
        }

        .sidebar-footer a {
            color: var(--text-ghost);
            text-decoration: none;
            transition: color 0.25s ease;
        }

        .sidebar-footer a:hover {
            color: var(--text-primary);
        }

        /* Main content */
        .main {
            flex: 1;
            max-width: 900px;
            padding: 40px 48px;
        }

        .main h1 {
            font-family: var(--font-mono);
            font-size: 1.5rem;
            font-weight: 300;
            color: var(--text-heading);
            margin-bottom: 16px;
            padding-bottom: 8px;
            border-bottom: 1px solid var(--border);
            letter-spacing: 0.3em;
        }

        .main h2 {
            font-family: var(--font-mono);
            font-size: 1.25rem;
            font-weight: 300;
            color: var(--text-primary);
            margin-top: 32px;
            margin-bottom: 12px;
            padding-bottom: 6px;
            border-bottom: 1px solid var(--border);
            letter-spacing: 0.2em;
        }

        .main h3 {
            font-family: var(--font-mono);
            font-size: 1.125rem;
            font-weight: 300;
            color: var(--text-dim);
            margin-top: 24px;
            margin-bottom: 8px;
            letter-spacing: 0.2em;
        }

        .main h4 {
            font-family: var(--font-mono);
            font-size: 1rem;
            font-weight: 400;
            color: var(--text-dim);
            margin-top: 20px;
            margin-bottom: 6px;
        }

        .main p {
            margin-bottom: 16px;
        }

        .main a {
            color: var(--text-dim);
            text-decoration: none;
            transition: color 0.25s ease;
        }

        .main a:hover {
            color: var(--text-primary);
        }

        .main ul, .main ol {
            margin-bottom: 16px;
            padding-left: 24px;
        }

        .main li {
            margin-bottom: 4px;
        }

        .main code {
            background: var(--code-bg);
            padding: 2px 6px;
            border-radius: 2px;
            font-family: var(--font-mono);
            font-size: 0.75rem;
            color: var(--text-code);
        }

        .main pre {
            background: var(--code-bg);
            border: 1px solid var(--border);
            border-radius: 4px;
            padding: 16px;
            overflow-x: auto;
            margin-bottom: 16px;
        }

        .main pre code {
            background: none;
            padding: 0;
            color: var(--text-primary);
            font-size: 0.75rem;
            line-height: 1.5;
        }

        .main table {
            width: 100%;
            border-collapse: collapse;
            margin-bottom: 16px;
            font-size: 0.75rem;
        }

        .main th {
            background: var(--bg-tertiary);
            color: var(--text-dim);
            padding: 10px 12px;
            text-align: left;
            font-weight: 400;
            border: 1px solid var(--border);
            letter-spacing: 0.1em;
        }

        .main td {
            padding: 8px 12px;
            border: 1px solid var(--border);
            color: var(--text-primary);
            font-family: var(--font-mono);
        }

        .main tr:hover td {
            background: var(--bg-tertiary);
        }

        .main blockquote {
            border-left: 2px solid var(--border-card);
            padding: 8px 16px;
            margin-bottom: 16px;
            background: var(--bg-secondary);
            border-radius: 0 4px 4px 0;
            color: var(--text-secondary);
        }

        .main hr {
            border: none;
            border-top: 1px solid var(--border);
            margin: 24px 0;
        }

        .main strong {
            color: var(--text-heading);
        }

        .main img {
            max-width: 100%;
            border-radius: 4px;
        }

        /* Responsive */
        @media (max-width: 768px) {
            .nav-links { display: none; }

            .layout {
                flex-direction: column;
            }

            .sidebar {
                width: 100%;
                min-width: 100%;
                height: auto;
                position: relative;
                border-right: none;
                border-bottom: 1px solid var(--border);
            }

            .main {
                padding: 24px 16px;
            }
        }
    </style>
</head>
<body>
    <canvas id="particle-canvas"></canvas>

    <!-- Navigation -->
    <nav class="nav">
        <a href="/" class="nav-logo"><span>(U)</span>NHEADED</a>
        <ul class="nav-links">
            <li><a href="/dashboard" class="nav-link">dashboard</a></li>
            <li><a href="/kanban" class="nav-link">kanban</a></li>
            <li><a href="/wiki/" class="nav-link active">docs</a></li>
            <li><a href="/status" class="nav-link">status</a></li>
        </ul>
    </nav>

    <div class="layout">
        <aside class="sidebar">
            <div class="sidebar-brand">
                <h1>Wiki</h1>
                <div class="subtitle">the kingdom's knowledge base</div>
            </div>
            <nav>
                <ul>
                {{range .NavItems}}
                    <li><a href="/wiki/{{.Slug}}" class="{{if eq .Slug $.ActiveSlug}}active{{end}}">{{.Title}}</a></li>
                {{end}}
                </ul>
            </nav>
            <div class="sidebar-footer">
                <p>v{{.Version}}</p>
                <p><a href="/">dashboard</a> | <a href="/kanban">kanban</a> | <a href="/health">health</a></p>
            </div>
        </aside>
        <main class="main">
            {{.Content}}
        </main>
    </div>

    <script>
    // Minimal particle animation (inline — wiki is standalone)
    (function() {
        var canvas = document.getElementById('particle-canvas');
        if (!canvas) return;
        var ctx = canvas.getContext('2d');
        var particles = [];
        var w, h, dpr = window.devicePixelRatio || 1;

        function resize() {
            w = window.innerWidth; h = window.innerHeight;
            canvas.width = w * dpr; canvas.height = h * dpr;
            canvas.style.width = w + 'px'; canvas.style.height = h + 'px';
            ctx.scale(dpr, dpr);
        }

        function init() {
            resize();
            particles = [];
            for (var i = 0; i < 50; i++) {
                particles.push({
                    x: Math.random() * w, y: Math.random() * h,
                    vx: (Math.random() - 0.5) * 0.3, vy: (Math.random() - 0.5) * 0.3,
                    s: 1 + Math.random() * 2, o: 0.3 + Math.random() * 0.4
                });
            }
        }

        function animate() {
            ctx.clearRect(0, 0, w, h);
            for (var i = 0; i < particles.length; i++) {
                var p = particles[i];
                p.x += p.vx; p.y += p.vy;
                if (p.x < -10) p.x = w + 10;
                if (p.x > w + 10) p.x = -10;
                if (p.y < -10) p.y = h + 10;
                if (p.y > h + 10) p.y = -10;
                ctx.beginPath();
                ctx.arc(p.x, p.y, p.s, 0, Math.PI * 2);
                ctx.fillStyle = 'rgba(74,158,255,' + p.o + ')';
                ctx.fill();
                for (var j = i + 1; j < particles.length; j++) {
                    var q = particles[j];
                    var dx = p.x - q.x, dy = p.y - q.y;
                    var d = Math.sqrt(dx * dx + dy * dy);
                    if (d < 150) {
                        ctx.beginPath();
                        ctx.moveTo(p.x, p.y); ctx.lineTo(q.x, q.y);
                        ctx.strokeStyle = 'rgba(74,158,255,' + ((1 - d / 150) * 0.2) + ')';
                        ctx.lineWidth = 1; ctx.stroke();
                    }
                }
            }
            requestAnimationFrame(animate);
        }

        window.addEventListener('resize', function() { resize(); });
        init(); animate();
    })();
    </script>
</body>
</html>`
