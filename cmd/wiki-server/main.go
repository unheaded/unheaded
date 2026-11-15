// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the GNU General Public License v3.0 or later.
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

// Package main implements wiki-server: a simple HTTP server that renders
// markdown files from wiki/ as styled HTML pages with sidebar navigation.
// GitHub-wiki-style [[Page]] / [[Display|Page]] links are rewritten to
// standard markdown links before rendering, and links into the rest of the
// repository are pointed at GitHub (repolinks.go).
//
// Usage:
//
//	wiki-server [--port 20002] [--wiki-dir ./wiki] [--vor-url http://127.0.0.1:20110]
//
// Endpoints:
//
//	GET /              Redirect to /wiki/
//	GET /wiki/         Wiki homepage (Home.md, or README.md without one)
//	GET /wiki/{page}   Render wiki/{page}.md as HTML
//	GET /health        Health check (200 OK)
//	GET /ready         Readiness probe (200 OK)
//
// With --vor-url (search.go), answered by vor:
//
//	GET /search?q=     Wiki pages, vor sheets and docs/ files that match
//	GET /sheets        The vor sheets, by category
//	GET /sheet/{name}  One vor sheet
//
// Version: 0.1.0-alpha
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"

	"unheaded/pkg/auth"
	"unheaded/pkg/bff"
	"unheaded/pkg/cmdline"
	"unheaded/pkg/httputil"
	"unheaded/pkg/metrics"
	pqtls "unheaded/pkg/tls"
	"unheaded/pkg/vorclient"
)

// wikiLinkRe matches GitHub-wiki-style links: [[Display|Page-Name]] or [[Page-Name]].
var wikiLinkRe = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)

// rewriteWikiLinks converts [[Page]] and [[Display|Page]] to standard markdown links
// so goldmark can render them. Targets resolve relative to /wiki/.
func rewriteWikiLinks(src []byte) []byte {
	return wikiLinkRe.ReplaceAllFunc(src, func(m []byte) []byte {
		sub := wikiLinkRe.FindSubmatch(m)
		display := string(sub[1])
		target := string(sub[2])
		if target == "" {
			target = display
		}
		return []byte("[" + display + "](/wiki/" + target + ")")
	})
}

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
	wikiDir   string
	md        goldmark.Markdown
	tmpl      *template.Template
	startTime time.Time
	// vor answers /search, /sheets and /sheet/ (search.go). nil: no search
	// box and none of the routes.
	vor    *vorclient.Client
	sheets sheetList
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
	ActiveSlug string
	Version    string
	// NavGroups is the sidebar: the sections of wiki/_Sidebar.md (nav.go).
	NavGroups []NavGroup
	// HomeActive marks the pinned Top link when the homepage is being shown.
	HomeActive bool
	// Search shows the search box and the Sheets link; Query is what the
	// box holds.
	Search bool
	Query  string
}

// NewWikiServer creates a new wiki server.
func NewWikiServer(wikiDir string) (*WikiServer, error) {
	absDir, err := filepath.Abs(wikiDir)
	if err != nil {
		return nil, fmt.Errorf("resolve wiki dir: %w", err)
	}

	info, err := os.Stat(absDir) //nolint:gosec // wikiDir is operator-supplied via CLI flag, not user input
	if err != nil {
		return nil, fmt.Errorf("stat wiki dir %s: %w", absDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("wiki dir %s is not a directory", absDir)
	}

	// NOTE: html.WithUnsafe() is deliberately NOT set.
	//
	// It was, which told goldmark to pass raw HTML through verbatim. Combined
	// with the template.HTML() wrap in renderPage — which bypasses Go's
	// contextual auto-escaping — any <script> in a markdown source rendered as
	// live markup: stored XSS for anyone able to land a file in the wiki dir.
	//
	// Checked before removing it: 0 of 161 wiki pages use raw HTML tags, so the
	// option bought nothing and cost the escaping. goldmark's default escapes
	// raw HTML, which is what we want for rendered-from-file content.
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Table),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
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
			// README is the wiki homepage (/wiki/), reached via the pinned "Top"
			// link, so it is not repeated as an entry in the article list.
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
	filename := ws.rootFile()
	if slug != "" {
		filename = slug + ".md"
	}

	path := filepath.Join(ws.wikiDir, filename)

	// Prevent path traversal.
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}
	// Require absPath to be wikiDir itself or strictly beneath it. A bare
	// HasPrefix would also match a sibling dir sharing the prefix (e.g. wikiDir
	// "…/wiki" matching "…/wiki-secrets"); the trailing separator closes that.
	if absPath != ws.wikiDir && !strings.HasPrefix(absPath, ws.wikiDir+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("path traversal attempt blocked")
	}

	// The slug comes from the request URL, and the prefix check above is
	// lexical: a symlink inside wikiDir still points anywhere. os.Root
	// refuses to resolve out of wikiDir, symlinks included.
	root, err := os.OpenRoot(ws.wikiDir)
	if err != nil {
		return "", "", fmt.Errorf("open wiki dir: %w", err)
	}
	defer root.Close()
	rel, err := filepath.Rel(ws.wikiDir, absPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve path: %w", err)
	}
	data, err := root.ReadFile(rel)
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

	data = rewriteWikiLinks(data)

	doc := ws.md.Parser().Parse(text.NewReader(data))
	rewriteWikiPageLinks(doc, ws.pageSlugs())
	rewriteRepoLinks(doc)

	var buf bytes.Buffer
	if err := ws.md.Renderer().Render(&buf, data, doc); err != nil {
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

	ws.writePage(w, http.StatusOK, PageData{
		Title:      title,
		ActiveSlug: path,
		NavGroups:  ws.navGroups(path),
		HomeActive: path == "" || path+".md" == ws.rootFile(),
	}, content)
}

// writePage sends one page in the wiki's template. content is HTML that
// goldmark or html/template wrote, never text taken from a request, a file
// or vor as it came.
func (ws *WikiServer) writePage(w http.ResponseWriter, status int, data PageData, content string) {
	data.Content = template.HTML(content) // #nosec G203 -- goldmark emits no raw HTML (WithUnsafe removed: it is dropped or entity-escaped), and the vor pages are written by html/template
	data.Version = serviceVersion
	data.Search = ws.vor != nil

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
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

	// Search, answered by vor.
	if ws.vor != nil {
		mux.HandleFunc(searchPath, ws.handleSearch)
		mux.HandleFunc(sheetsPath, ws.handleSheets)
		mux.HandleFunc(sheetPath, ws.handleSheet)
	}

	// Health and readiness.
	mux.HandleFunc("/health", ws.handleHealth)
	mux.HandleFunc("/ready", ws.handleReady)
	mux.Handle("/metrics", metrics.Handler())

	return mux
}

// wrapAuth applies auth from authCfg to the handler (S-IRON-GATE Phase 6).
//
// With AUTH_ENABLED=true and an issuer (AUTH_ISSUER_URL) the wiki has the
// dashboard's login (pkg/bff, ADR-103 section 4): every page needs a
// session cookie or a service token, /auth/* and the probes stay open, and
// a browser without a session gets the page shell with the login panel
// instead of a bare 401 (Stevie 2026-11-11: "tls + login on wiki"). With
// auth on and no issuer, reads stay open as before and a warning says so.
// An error means auth is on but unusable, and the server must not start.
func wrapAuth(ws *WikiServer, mux *http.ServeMux, authCfg auth.ServiceAuthConfig) (http.Handler, error) {
	mux.HandleFunc("/auth/auth.js", bff.ServeAuthJS)
	mux.HandleFunc("/auth/nav.js", bff.ServeNavJS)
	routes := auth.ServiceRoutes{OpenPaths: []string{"/auth"}}
	var gate func(http.Handler) http.Handler
	switch bcfg := bff.ConfigFromEnv(authCfg.ServiceName); {
	case !authCfg.Enabled:
		routes.OpenReads = []string{"/"}
	case !bcfg.Enabled():
		logf("WARN", "auth on without AUTH_ISSUER_URL: the wiki's reads stay open; a write needs a token")
		routes.OpenReads = []string{"/"}
	default:
		b, err := bff.New(bcfg)
		if err != nil {
			return nil, fmt.Errorf("wiki login: %w", err)
		}
		mux.Handle("/auth/", b.Handler())
		routes.Authenticators = []auth.Authenticator{b.Authenticator()}
		gate = ws.loginPage
	}
	authed, err := auth.NewServiceHandler(mux, authCfg, routes)
	if err != nil {
		return nil, err
	}
	if gate != nil {
		return gate(authed), nil
	}
	return authed, nil
}

// loginPage turns the 401 that auth gives a browser's page request into
// the wiki's page shell at 401: the menu, the sidebar brand and a line
// saying to sign in. auth.js on that page shows the login panel and
// reloads after a login. Anything that is not a GET or HEAD for HTML (a
// script, a curl, a service token that was refused) gets auth's own answer.
func (ws *WikiServer) loginPage(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (r.Method != http.MethodGet && r.Method != http.MethodHead) || !strings.Contains(r.Header.Get("Accept"), "text/html") {
			next.ServeHTTP(w, r)
			return
		}
		cw := &catch401{ResponseWriter: w}
		next.ServeHTTP(cw, r)
		if !cw.caught {
			return
		}
		h := w.Header()
		h.Del("WWW-Authenticate")
		h.Del("Content-Type")
		h.Set("Cache-Control", "no-store")
		var body bytes.Buffer
		_ = noticeTmpl.Execute(&body, notice{
			Heading: "Sign in",
			Text:    "This wiki needs a login. Use the Log in button at the bottom right; the page returns here afterwards.",
		})
		ws.writePage(w, http.StatusUnauthorized, PageData{Title: "Sign in", NavGroups: nil}, body.String())
	})
}

// catch401 swallows a 401 answer so the caller can write a page instead.
// Any other status passes through untouched.
type catch401 struct {
	http.ResponseWriter
	caught bool
	wrote  bool
}

func (c *catch401) WriteHeader(status int) {
	if c.wrote {
		return
	}
	c.wrote = true
	if status == http.StatusUnauthorized {
		c.caught = true
		return
	}
	c.ResponseWriter.WriteHeader(status)
}

func (c *catch401) Write(b []byte) (int, error) {
	if !c.wrote {
		c.WriteHeader(http.StatusOK)
	}
	if c.caught {
		return len(b), nil
	}
	return c.ResponseWriter.Write(b)
}

// buildHandler creates the full HTTP handler stack for the wiki server.
// vorURL is the vor that answers the search box; empty means no search.
func buildHandler(wikiDir, vorURL string) (http.Handler, error) {
	ws, err := NewWikiServer(wikiDir)
	if err != nil {
		return nil, err
	}
	if vorURL != "" {
		if ws.vor, err = vorclient.New(vorURL, nil); err != nil {
			return nil, fmt.Errorf("--vor-url: %w", err)
		}
	}
	mux := setupMux(ws)
	authed, err := wrapAuth(ws, mux, auth.LoadServiceAuthConfig("wiki-server"))
	if err != nil {
		return nil, fmt.Errorf("auth: %w (see ADR-103)", err)
	}
	// Outermost, so requests auth rejects are counted too.
	return httputil.SecurityHeaders(httputil.NewServiceMetrics(serviceName).Instrument(mux, authed), httputil.WithCSP(wikiCSP())), nil
}

// newServer creates an http.Server with standard timeouts.
func newServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

// newListener returns the server for addr: TLS when the process serves TLS
// (UNHEADED_TLS_ENABLED with a certificate and key, pkg/tls; the Preferred
// profile, browsers reach this, or Strict with UNHEADED_TLS_PROFILE=strict),
// plain HTTP otherwise. The second value is what to call to serve.
func newListener(addr string, handler http.Handler) (*http.Server, func(*http.Server) error, error) {
	if !pqtls.ServesTLS() {
		return newServer(addr, handler), (*http.Server).ListenAndServe, nil
	}
	cfg := pqtls.LoadServiceTLSConfig()
	certPEM, err := os.ReadFile(cfg.CertFile)
	if err != nil {
		return nil, nil, fmt.Errorf("tls certificate: %w", err)
	}
	keyPEM, err := os.ReadFile(cfg.KeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("tls key: %w", err)
	}
	srv, err := pqtls.NewTLSServer(addr, handler, pqtls.ServerConfig{CertPEM: certPEM, KeyPEM: keyPEM})
	if err != nil {
		return nil, nil, err
	}
	if cfg.Profile == "strict" {
		pqtls.ApplyStrict(srv.TLSConfig)
	}
	srv.ReadHeaderTimeout = 5 * time.Second
	srv.MaxHeaderBytes = 1 << 20
	return srv, func(s *http.Server) error { return s.ListenAndServeTLS("", "") }, nil
}

// listenFlags collects --listen, repeatable.
type listenFlags []string

func (l *listenFlags) String() string { return strings.Join(*l, ",") }
func (l *listenFlags) Set(v string) error {
	ap, err := netip.ParseAddrPort(strings.TrimSpace(v))
	if err != nil {
		return fmt.Errorf("--listen %q: want IP:PORT", v)
	}
	if ap.Addr().IsUnspecified() {
		return fmt.Errorf("--listen %q: name each address; the wildcard address is not accepted", v)
	}
	*l = append(*l, ap.String())
	return nil
}

// defaultListen is the listener --port names. It goes through
// listenFlags.Set like every --listen, so a port outside 0-65535 is refused
// there and never wrapped into another port.
func defaultListen(port int) string {
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func main() {
	port := flag.Int("port", 20002, "port of the default listener, 127.0.0.1:PORT, when no --listen is given")
	var listens listenFlags
	flag.Var(&listens, "listen", "IP:PORT to serve on, repeatable; no wildcard address (default 127.0.0.1:20002). TLS when UNHEADED_TLS_ENABLED=true with UNHEADED_TLS_CERT and UNHEADED_TLS_KEY_FILE")
	wikiDir := flag.String("wiki-dir", "./wiki", "Path to wiki markdown directory")
	vorURL := flag.String("vor-url", "", "vor base URL, such as http://127.0.0.1:20110: adds the search box, /search, /sheets and /sheet/ (unset: none of them)")
	cmdline.Parse(cmdline.Spec{
		Name:    "wiki-server",
		Purpose: "Serve the Kingdom wiki as rendered HTML.",
		Examples: []string{
			"wiki-server",
			"wiki-server --port 20002 --wiki-dir ./wiki",
			"wiki-server --listen 127.0.0.1:20002 --listen 192.0.2.10:20002 --vor-url http://127.0.0.1:20110",
		},
	})
	if len(listens) == 0 {
		if err := listens.Set(defaultListen(*port)); err != nil {
			logf("FATAL", "bad --port", "port", *port, "err", err)
			os.Exit(2)
		}
	}

	logf("INFO", "starting wiki-server",
		"version", serviceVersion,
		"listen", listens.String(),
		"tls", pqtls.ServesTLS(),
		"wiki_dir", *wikiDir,
		"vor_url", *vorURL,
	)

	httpHandler, err := buildHandler(*wikiDir, *vorURL)
	if err != nil {
		logf("FATAL", "failed to create wiki server", "err", err)
		os.Exit(1)
	}

	var servers []*http.Server
	for _, addr := range listens {
		server, serve, err := newListener(addr, httpHandler)
		if err != nil {
			logf("FATAL", "listener", "addr", addr, "err", err)
			os.Exit(1)
		}
		servers = append(servers, server)
		go func() {
			logf("INFO", "wiki-server listening", "addr", server.Addr, "tls", pqtls.ServesTLS())
			if err := serve(server); err != nil && err != http.ErrServerClosed {
				logf("FATAL", "server error", "addr", server.Addr, "err", err)
				os.Exit(1)
			}
		}()
	}

	// Graceful shutdown.
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGTERM)
	<-done
	logf("INFO", "shutting down wiki-server")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, server := range servers {
		_ = server.Shutdown(ctx)
	}
}

// pageTemplate is the HTML template for rendering wiki pages.
// wikiCSP allows exactly the page template's inline <style> and <script>
// blocks, by sha256 (CSP Level 2 hash sources), and nothing else inline:
// rendered markdown cannot add a script. Images may come from https (page
// badges). The hashes are taken from a render of pageTemplate, not its
// source: html/template rewrites the blocks (it strips CSS comments), and
// the browser hashes what is served. The blocks hold no template actions,
// so every page serves them identically.
func wikiCSP() string {
	var page bytes.Buffer
	if err := template.Must(template.New("csp").Parse(pageTemplate)).Execute(&page, PageData{}); err != nil {
		panic("wiki-server: render page template for CSP: " + err.Error())
	}
	hashes := func(tag string) string {
		re := regexp.MustCompile(`(?s)<` + tag + `>(.*?)</` + tag + `>`)
		var out []string
		for _, m := range re.FindAllStringSubmatch(page.String(), -1) {
			sum := sha256.Sum256([]byte(m[1]))
			out = append(out, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
		}
		return strings.Join(out, " ")
	}
	return "default-src 'self'; script-src 'self' " + hashes("script") + "; style-src " + hashes("style") +
		"; img-src 'self' data: https:; frame-ancestors 'none'; base-uri 'none'"
}

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
            display: flex;
            flex-direction: column;
            overflow: hidden;
        }

        .sidebar-brand {
            padding: 0 20px 20px;
            border-bottom: 1px solid var(--border);
            margin-bottom: 16px;
            flex-shrink: 0;
        }

        .sidebar-brand a { text-decoration: none; }

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

        .sidebar-links {
            padding: 0 20px 16px;
            border-bottom: 1px solid var(--border);
            margin-bottom: 16px;
            font-size: 12px;
            color: var(--text-secondary);
            flex-shrink: 0;
        }

        .sidebar-links a {
            color: var(--accent);
            text-decoration: none;
        }

        .sidebar-links a:hover {
            text-decoration: underline;
        }

        /* Search box and Sheets link, shown when vor is configured. */
        .sidebar-search {
            display: flex;
            align-items: center;
            gap: 10px;
            padding: 0 20px 12px;
            font-size: 12px;
            flex-shrink: 0;
        }

        .sidebar-search a {
            color: var(--accent);
            text-decoration: none;
        }

        .sidebar-search a:hover {
            text-decoration: underline;
        }

        .sidebar-search input {
            flex: 1;
            min-width: 0;
            padding: 6px 8px;
            background: var(--bg-primary);
            color: var(--text-primary);
            border: 1px solid var(--border);
            border-radius: 4px;
            font: inherit;
            font-size: 13px;
        }

        .sidebar-search input:focus {
            border-color: var(--accent);
        }

        /* Pinned "Home" link (returns to the wiki root without scrolling up). */
        .sidebar-home {
            display: block;
            padding: 8px 20px;
            margin-bottom: 8px;
            color: var(--accent);
            text-decoration: none;
            font-size: 14px;
            flex-shrink: 0;
        }

        .sidebar-home:hover,
        .sidebar-home.active {
            background: var(--bg-tertiary);
        }

        /* Only the article list scrolls; brand + cross-app links stay pinned. */
        .sidebar nav {
            flex: 1;
            overflow-y: auto;
            min-height: 0;
        }

        .sidebar nav ul {
            list-style: none;
        }

        .nav-group {
            margin-bottom: 12px;
        }

        .nav-group h2 {
            padding: 8px 20px 4px;
            font-size: 11px;
            font-weight: 600;
            letter-spacing: 0.08em;
            text-transform: uppercase;
            color: var(--text-heading);
            opacity: 0.55;
        }

        .sidebar nav summary {
            padding: 4px 20px;
            font-size: 13px;
            color: var(--text-secondary);
            cursor: pointer;
        }

        .sidebar nav summary:hover {
            color: var(--text-primary);
        }

        .sidebar nav li.sub a,
        .sidebar nav li.sub .nav-missing {
            padding-left: 36px;
            font-size: 13px;
        }

        .nav-note {
            font-size: 12px;
            opacity: 0.6;
        }

        .nav-missing {
            display: block;
            padding: 4px 20px;
            font-size: 14px;
            color: var(--text-secondary);
            opacity: 0.5;
        }

        .sidebar nav a {
            display: block;
            padding: 4px 20px;
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
            flex-shrink: 0;
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

        /* Search results, the sheet listing and the foot of a sheet. */
        .page-note {
            color: var(--text-secondary);
            font-size: 0.85em;
            font-weight: normal;
        }

        .sheet-index a {
            margin-right: 10px;
            white-space: nowrap;
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
                <a href="/wiki/">
                    <h1>(u)nheaded</h1>
                    <div class="subtitle">wiki</div>
                </a>
            </div>
            <div class="sidebar-links" data-kingdom-nav="wiki" data-kingdom-nav-class="" data-kingdom-nav-sep=" | "></div>
            {{if .Search}}<form class="sidebar-search" action="/search" method="get" role="search">
                <input type="search" name="q" value="{{.Query}}" placeholder="search" aria-label="Search the wiki, vor sheets and docs">
                <a href="/sheets">Sheets</a>
            </form>{{end}}
            <a href="/wiki/" class="sidebar-home{{if .HomeActive}} active{{end}}">&#8962; Top</a>
            <nav>
                {{range .NavGroups}}
                <section class="nav-group">
                    {{if .Title}}<h2>{{.Title}}</h2>{{end}}
                    {{range .Blocks}}
                    {{if .Summary}}{{if .Open}}<details open>{{else}}<details>{{end}}<summary>{{.Summary}}</summary>{{end}}
                    <ul>
                    {{range .Items}}
                        <li class="{{if .Sub}}sub{{end}}">{{if .Missing}}<span class="nav-missing">{{.Title}}{{if .Note}} <span class="nav-note">{{.Note}}</span>{{end}}</span>{{else}}<a href="/wiki/{{.Slug}}" class="{{if .Active}}active{{end}}">{{.Title}}{{if .Note}} <span class="nav-note">{{.Note}}</span>{{end}}</a>{{end}}</li>
                    {{end}}
                    </ul>
                    {{if .Summary}}</details>{{end}}
                    {{end}}
                </section>
                {{end}}
            </nav>
            <div class="sidebar-footer">
                <p>v{{.Version}}</p>
            </div>
        </aside>
        <main class="main">
            {{.Content}}
        </main>
    </div>
    </div>
    <script>
    (function(){
        var n=document.querySelector('.sidebar nav'),a=n&&n.querySelector('a.active');
        if(a){n.scrollTop+=a.getBoundingClientRect().top-n.getBoundingClientRect().top-n.clientHeight/2;}
    })();
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
    <script src="/auth/nav.js"></script>
    <script src="/auth/auth.js"></script>
</body>
</html>`
