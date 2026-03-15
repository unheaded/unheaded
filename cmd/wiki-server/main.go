// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

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

// setupMux creates the HTTP mux with all routes for the wiki server.
func setupMux(ws *WikiServer) *http.ServeMux {
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

	return mux
}

// wrapAuth applies authentication middleware to the given handler.
func wrapAuth(handler http.Handler) http.Handler {
	authCfg := auth.LoadServiceAuthConfig("wiki-server")
	return auth.WrapHandler(handler, auth.SetupMiddleware(authCfg))
}

// buildHandler creates the full HTTP handler stack for the wiki server.
func buildHandler(wikiDir string) (http.Handler, error) {
	ws, err := NewWikiServer(wikiDir)
	if err != nil {
		return nil, err
	}
	mux := setupMux(ws)
	return wrapAuth(mux), nil
}

// newServer creates an http.Server with standard timeouts.
func newServer(port int, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           handler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
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

	httpHandler, err := buildHandler(*wikiDir)
	if err != nil {
		logf("FATAL", "failed to create wiki server", "err", err)
		os.Exit(1)
	}

	server := newServer(*port, httpHandler)

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
    <title>{{.Title}} | (U)NHEADED Wiki</title>
    <style>
        :root {
            --bg-primary: #0a0a0a;
            --bg-secondary: #111111;
            --bg-tertiary: #161616;
            --text-primary: #c9c9c9;
            --text-secondary: #666666;
            --text-heading: #ffffff;
            --accent: #4a9eff;
            --accent-hover: #6ab0ff;
            --border: #222222;
            --code-bg: #111111;
            --link: #4a9eff;
            --success: #3d9e50;
            --warning: #cc8800;
            --danger: #cc4444;
        }

        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: 'JetBrains Mono', 'SF Mono', 'Fira Code', monospace;
            background: var(--bg-primary);
            color: var(--text-primary);
            font-size: 14px;
            line-height: 1.8;
        }

        .layout {
            display: flex;
            min-height: 100vh;
        }

        /* Sidebar */
        .sidebar {
            width: 280px;
            min-width: 280px;
            background: var(--bg-secondary);
            border-right: 1px solid var(--border);
            padding: 24px 0;
            position: sticky;
            top: 0;
            height: 100vh;
            overflow-y: auto;
        }

        .sidebar-brand {
            padding: 0 20px 20px;
            border-bottom: 1px solid var(--border);
            margin-bottom: 16px;
        }

        .sidebar-brand h1 {
            font-size: 14px;
            color: var(--text-secondary);
            font-weight: 300;
            letter-spacing: 0.3em;
            text-transform: lowercase;
        }

        .sidebar-brand .subtitle {
            font-size: 11px;
            color: #3a3a3a;
            margin-top: 4px;
            letter-spacing: 0.1em;
        }

        .sidebar nav ul {
            list-style: none;
        }

        .sidebar nav a {
            display: block;
            padding: 8px 20px;
            color: var(--text-secondary);
            text-decoration: none;
            font-size: 14px;
            border-left: 3px solid transparent;
            transition: all 0.15s ease;
        }

        .sidebar nav a:hover {
            color: var(--text-primary);
            background: var(--bg-tertiary);
        }

        .sidebar nav a.active {
            color: var(--accent);
            border-left-color: var(--accent);
            background: var(--bg-tertiary);
        }

        .sidebar-footer {
            padding: 16px 20px;
            border-top: 1px solid var(--border);
            margin-top: 16px;
            font-size: 12px;
            color: var(--text-secondary);
        }

        .sidebar-footer a {
            color: var(--accent);
            text-decoration: none;
        }

        /* Main content */
        .main {
            flex: 1;
            max-width: 900px;
            padding: 40px 48px;
        }

        .main h1 {
            font-size: 1.5rem;
            color: var(--text-heading);
            font-weight: 300;
            letter-spacing: 0.3em;
            margin-bottom: 16px;
            padding-bottom: 8px;
            border-bottom: 1px solid var(--border);
        }

        .main h2 {
            font-size: 1.25rem;
            color: var(--text-primary);
            font-weight: 300;
            letter-spacing: 0.2em;
            margin-top: 32px;
            margin-bottom: 12px;
            padding-bottom: 6px;
            border-bottom: 1px solid var(--border);
        }

        .main h3 {
            font-size: 1.125rem;
            color: var(--text-secondary);
            font-weight: 300;
            letter-spacing: 0.15em;
            margin-top: 24px;
            margin-bottom: 8px;
        }

        .main h4 {
            font-size: 1rem;
            color: var(--text-secondary);
            font-weight: 300;
            letter-spacing: 0.1em;
            margin-top: 20px;
            margin-bottom: 6px;
        }

        .main p {
            margin-bottom: 16px;
        }

        .main a {
            color: var(--link);
            text-decoration: none;
        }

        .main a:hover {
            text-decoration: underline;
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
            border-radius: 4px;
            font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
            font-size: 0.9em;
            color: var(--accent);
        }

        .main pre {
            background: var(--code-bg);
            border: 1px solid var(--border);
            border-radius: 6px;
            padding: 16px;
            overflow-x: auto;
            margin-bottom: 16px;
        }

        .main pre code {
            background: none;
            padding: 0;
            color: var(--text-primary);
            font-size: 13px;
            line-height: 1.5;
        }

        .main table {
            width: 100%;
            border-collapse: collapse;
            margin-bottom: 16px;
        }

        .main th {
            background: var(--bg-tertiary);
            color: var(--text-primary);
            padding: 10px 12px;
            text-align: left;
            font-weight: 400;
            letter-spacing: 0.1em;
            border: 1px solid var(--border);
            font-size: 12px;
            text-transform: uppercase;
        }

        .main td {
            padding: 8px 12px;
            border: 1px solid var(--border);
            font-size: 14px;
        }

        .main tr:nth-child(even) {
            background: var(--bg-secondary);
        }

        .main blockquote {
            border-left: 4px solid var(--accent);
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
            border-radius: 6px;
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

        .wiki-wrapper {
            position: relative;
            z-index: 1;
        }

        /* Responsive */
        @media (max-width: 768px) {
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
    <div class="wiki-wrapper">
    <div class="layout">
        <aside class="sidebar">
            <div class="sidebar-brand">
                <h1>(u)nheaded</h1>
                <div class="subtitle">wiki</div>
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
                <p><a href="/">Dashboard</a> | <a href="/kanban">Kanban</a> | <a href="/health">Health</a></p>
            </div>
        </aside>
        <main class="main">
            {{.Content}}
        </main>
    </div>
    </div>
    <script>
    (function(){
        var c=document.getElementById('particle-canvas'),x=c.getContext('2d'),
            ps=[],N=50,W,H,dpr=window.devicePixelRatio||1;
        function resize(){W=window.innerWidth;H=window.innerHeight;c.width=W*dpr;c.height=H*dpr;c.style.width=W+'px';c.style.height=H+'px';x.scale(dpr,dpr);}
        resize();window.addEventListener('resize',resize);
        for(var i=0;i<N;i++)ps.push({x:Math.random()*W,y:Math.random()*H,s:1+Math.random()*2,vx:(Math.random()-0.5)*0.3,vy:(Math.random()-0.5)*0.3,o:0.2+Math.random()*0.4});
        function draw(){x.clearRect(0,0,W,H);
            for(var i=0;i<ps.length;i++){var p=ps[i];p.x+=p.vx;p.y+=p.vy;
                if(p.x<-10)p.x=W+10;if(p.x>W+10)p.x=-10;if(p.y<-10)p.y=H+10;if(p.y>H+10)p.y=-10;
                x.beginPath();x.arc(p.x,p.y,p.s,0,Math.PI*2);x.fillStyle='rgba(74,158,255,'+p.o+')';x.fill();
                for(var j=i+1;j<ps.length;j++){var q=ps[j],dx=p.x-q.x,dy=p.y-q.y,d=Math.sqrt(dx*dx+dy*dy);
                    if(d<150){x.beginPath();x.moveTo(p.x,p.y);x.lineTo(q.x,q.y);x.strokeStyle='rgba(74,158,255,'+(1-d/150)*0.2+')';x.stroke();}}}
            requestAnimationFrame(draw);}
        draw();
    })();
    </script>
</body>
</html>`
