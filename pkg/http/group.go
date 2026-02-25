// SPDX-License-Identifier: MIT
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

/*
 * This file is part of the Unheaded distributed system platform.
 *
 * Unheaded is licensed under the Business Source License 1.1 (BSL 1.1).
 * See the LICENSE file in the root directory for the full license text.
 *
 * For protocol specifications, see LICENSE-PROTOCOLS.
 * For GPL 2.0-licensed components (DOOM engine), see doom/LICENSE.
 */

package http

import "strings"

// RouteGroup represents a group of routes with a common prefix
type RouteGroup struct {
	// prefix is the common prefix for all routes in this group
	prefix string

	// middleware is the middleware chain for this group
	middleware HandlersChain

	// router is the parent router
	router *Router

	// parent is the parent group (for nested groups)
	parent *RouteGroup
}

// calculateAbsolutePath calculates the absolute path including parent prefixes
func (g *RouteGroup) calculateAbsolutePath(relativePath string) string {
	if relativePath == "" {
		return g.absolutePrefix()
	}

	// Ensure path starts with /
	if relativePath[0] != '/' {
		relativePath = "/" + relativePath
	}

	// Build absolute path
	return joinPaths(g.absolutePrefix(), relativePath)
}

// absolutePrefix returns the absolute prefix including parent prefixes
func (g *RouteGroup) absolutePrefix() string {
	if g.parent == nil {
		return g.prefix
	}
	return joinPaths(g.parent.absolutePrefix(), g.prefix)
}

// combinedMiddleware returns the combined middleware chain including parent middleware
func (g *RouteGroup) combinedMiddleware() HandlersChain {
	if g.parent == nil {
		return g.middleware
	}
	return g.parent.combinedMiddleware().combine(g.middleware)
}

// Use adds middleware to the group
func (g *RouteGroup) Use(middleware ...HandlerFunc) *RouteGroup {
	g.middleware = append(g.middleware, middleware...)
	return g
}

// Handle registers a route with a specific method
func (g *RouteGroup) Handle(method, path string, handlers ...HandlerFunc) {
	absolutePath := g.calculateAbsolutePath(path)

	// Combine group middleware with route handlers
	finalHandlers := g.combinedMiddleware().combine(handlers)

	// Register with router (which will add global middleware)
	g.router.Handle(method, absolutePath, finalHandlers...)
}

// GET registers a GET route
func (g *RouteGroup) GET(path string, handlers ...HandlerFunc) {
	g.Handle(MethodGet, path, handlers...)
}

// POST registers a POST route
func (g *RouteGroup) POST(path string, handlers ...HandlerFunc) {
	g.Handle(MethodPost, path, handlers...)
}

// PUT registers a PUT route
func (g *RouteGroup) PUT(path string, handlers ...HandlerFunc) {
	g.Handle(MethodPut, path, handlers...)
}

// DELETE registers a DELETE route
func (g *RouteGroup) DELETE(path string, handlers ...HandlerFunc) {
	g.Handle(MethodDelete, path, handlers...)
}

// PATCH registers a PATCH route
func (g *RouteGroup) PATCH(path string, handlers ...HandlerFunc) {
	g.Handle(MethodPatch, path, handlers...)
}

// HEAD registers a HEAD route
func (g *RouteGroup) HEAD(path string, handlers ...HandlerFunc) {
	g.Handle(MethodHead, path, handlers...)
}

// OPTIONS registers an OPTIONS route
func (g *RouteGroup) OPTIONS(path string, handlers ...HandlerFunc) {
	g.Handle(MethodOptions, path, handlers...)
}

// Any registers a route for all HTTP methods
func (g *RouteGroup) Any(path string, handlers ...HandlerFunc) {
	g.GET(path, handlers...)
	g.POST(path, handlers...)
	g.PUT(path, handlers...)
	g.DELETE(path, handlers...)
	g.PATCH(path, handlers...)
	g.HEAD(path, handlers...)
	g.OPTIONS(path, handlers...)
}

// Match registers a route for multiple methods
func (g *RouteGroup) Match(methods []string, path string, handlers ...HandlerFunc) {
	for _, method := range methods {
		g.Handle(method, path, handlers...)
	}
}

// Group creates a nested route group
func (g *RouteGroup) Group(prefix string, middleware ...HandlerFunc) *RouteGroup {
	newGroup := &RouteGroup{
		prefix:     prefix,
		middleware: middleware,
		router:     g.router,
		parent:     g,
	}
	return newGroup
}

// Static registers a static file handler for the group
func (g *RouteGroup) Static(path, root string) {
	absolutePath := g.calculateAbsolutePath(path)

	if !strings.HasSuffix(absolutePath, "/*path") {
		absolutePath = strings.TrimSuffix(absolutePath, "/") + "/*path"
	}

	handler := func(c *Context) {
		filePath := c.Param("path")
		fullPath := root + "/" + filePath
		c.File(fullPath)
	}

	// Combine group middleware
	finalHandlers := g.combinedMiddleware().combine(HandlersChain{handler})

	g.router.Handle(MethodGet, absolutePath, finalHandlers...)
	g.router.Handle(MethodHead, absolutePath, finalHandlers...)
}

// StaticFile registers a single static file handler for the group
func (g *RouteGroup) StaticFile(path, filepath string) {
	absolutePath := g.calculateAbsolutePath(path)

	handler := func(c *Context) {
		c.File(filepath)
	}

	finalHandlers := g.combinedMiddleware().combine(HandlersChain{handler})

	g.router.Handle(MethodGet, absolutePath, finalHandlers...)
	g.router.Handle(MethodHead, absolutePath, finalHandlers...)
}

// --- Helper Functions ---

// joinPaths joins two paths, handling slashes correctly
func joinPaths(absolutePath, relativePath string) string {
	if relativePath == "" {
		return absolutePath
	}

	// Normalize paths
	absolutePath = strings.TrimSuffix(absolutePath, "/")
	if relativePath[0] != '/' {
		relativePath = "/" + relativePath
	}

	finalPath := absolutePath + relativePath

	// Handle double slashes
	for strings.Contains(finalPath, "//") {
		finalPath = strings.ReplaceAll(finalPath, "//", "/")
	}

	// Ensure path starts with /
	if len(finalPath) > 0 && finalPath[0] != '/' {
		finalPath = "/" + finalPath
	}

	return finalPath
}

// --- Route Builder (Fluent API) ---

// RouteBuilder provides a fluent API for building routes
type RouteBuilder struct {
	method   string
	path     string
	handlers HandlersChain
	group    *RouteGroup
	router   *Router
}

// Route starts building a new route
func (r *Router) Route(method, path string) *RouteBuilder {
	return &RouteBuilder{
		method: method,
		path:   path,
		router: r,
	}
}

// Route starts building a new route on a group
func (g *RouteGroup) Route(method, path string) *RouteBuilder {
	return &RouteBuilder{
		method: method,
		path:   path,
		group:  g,
		router: g.router,
	}
}

// Use adds middleware to the route
func (rb *RouteBuilder) Use(middleware ...HandlerFunc) *RouteBuilder {
	rb.handlers = append(rb.handlers, middleware...)
	return rb
}

// Handler sets the final handler for the route
func (rb *RouteBuilder) Handler(handler HandlerFunc) {
	rb.handlers = append(rb.handlers, handler)

	if rb.group != nil {
		rb.group.Handle(rb.method, rb.path, rb.handlers...)
	} else {
		rb.router.Handle(rb.method, rb.path, rb.handlers...)
	}
}

// --- CRUD Resource Helper ---

// Resource represents a RESTful resource controller
type Resource interface {
	// Index handles GET /resource
	Index(c *Context)

	// Show handles GET /resource/:id
	Show(c *Context)

	// Create handles POST /resource
	Create(c *Context)

	// Update handles PUT/PATCH /resource/:id
	Update(c *Context)

	// Delete handles DELETE /resource/:id
	Delete(c *Context)
}

// ResourceRoutes registers CRUD routes for a resource
func (r *Router) Resource(path string, resource Resource) {
	r.GET(path, resource.Index)
	r.GET(path+"/:id", resource.Show)
	r.POST(path, resource.Create)
	r.PUT(path+"/:id", resource.Update)
	r.PATCH(path+"/:id", resource.Update)
	r.DELETE(path+"/:id", resource.Delete)
}

// Resource registers CRUD routes for a resource on a group
func (g *RouteGroup) Resource(path string, resource Resource) {
	g.GET(path, resource.Index)
	g.GET(path+"/:id", resource.Show)
	g.POST(path, resource.Create)
	g.PUT(path+"/:id", resource.Update)
	g.PATCH(path+"/:id", resource.Update)
	g.DELETE(path+"/:id", resource.Delete)
}

// --- Partial Resource (Optional Methods) ---

// PartialResource allows implementing only some CRUD methods
type PartialResource struct {
	IndexHandler  HandlerFunc
	ShowHandler   HandlerFunc
	CreateHandler HandlerFunc
	UpdateHandler HandlerFunc
	DeleteHandler HandlerFunc
}

// Index handles GET /resource
func (pr *PartialResource) Index(c *Context) {
	if pr.IndexHandler != nil {
		pr.IndexHandler(c)
	} else {
		c.NotFound()
	}
}

// Show handles GET /resource/:id
func (pr *PartialResource) Show(c *Context) {
	if pr.ShowHandler != nil {
		pr.ShowHandler(c)
	} else {
		c.NotFound()
	}
}

// Create handles POST /resource
func (pr *PartialResource) Create(c *Context) {
	if pr.CreateHandler != nil {
		pr.CreateHandler(c)
	} else {
		c.NotFound()
	}
}

// Update handles PUT/PATCH /resource/:id
func (pr *PartialResource) Update(c *Context) {
	if pr.UpdateHandler != nil {
		pr.UpdateHandler(c)
	} else {
		c.NotFound()
	}
}

// Delete handles DELETE /resource/:id
func (pr *PartialResource) Delete(c *Context) {
	if pr.DeleteHandler != nil {
		pr.DeleteHandler(c)
	} else {
		c.NotFound()
	}
}
