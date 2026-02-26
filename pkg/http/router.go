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

package http

import (
	"net/http"
	"strings"
	"sync"
)

// HTTP methods
const (
	MethodGet     = http.MethodGet
	MethodPost    = http.MethodPost
	MethodPut     = http.MethodPut
	MethodDelete  = http.MethodDelete
	MethodPatch   = http.MethodPatch
	MethodHead    = http.MethodHead
	MethodOptions = http.MethodOptions
	MethodConnect = http.MethodConnect
	MethodTrace   = http.MethodTrace
)

// Router is the main HTTP router
type Router struct {
	// trees holds a radix tree for each HTTP method
	trees map[string]*radixNode

	// middleware is the global middleware chain
	middleware HandlersChain

	// groups holds registered route groups
	groups []*RouteGroup

	// pool reuses Context objects
	pool sync.Pool

	// notFoundHandler is called when no route matches
	notFoundHandler HandlerFunc

	// methodNotAllowedHandler is called when method doesn't match
	methodNotAllowedHandler HandlerFunc

	// panicHandler is called when a panic occurs
	panicHandler func(*Context, any)

	// maxParams is the maximum number of parameters across all routes
	maxParams int

	// RedirectTrailingSlash enables automatic redirect for trailing slashes
	RedirectTrailingSlash bool

	// RedirectFixedPath enables automatic redirect for fixed paths
	RedirectFixedPath bool

	// HandleMethodNotAllowed enables 405 Method Not Allowed responses
	HandleMethodNotAllowed bool

	// HandleOPTIONS enables automatic OPTIONS handling
	HandleOPTIONS bool
}

// NewRouter creates a new Router instance
func NewRouter() *Router {
	r := &Router{
		trees:                  make(map[string]*radixNode),
		RedirectTrailingSlash:  true,
		HandleMethodNotAllowed: true,
		HandleOPTIONS:          true,
	}

	r.pool.New = func() any {
		return &Context{
			params: make(map[string]string),
			keys:   make(map[string]any),
		}
	}

	return r
}

// --- Radix Tree Implementation ---

// nodeType represents the type of a radix tree node
type nodeType uint8

const (
	static nodeType = iota // static path segment
	param                  // path parameter :name
	catchAll               // catch-all parameter *name
)

// radixNode represents a node in the radix tree
type radixNode struct {
	// path is the path segment this node represents
	path string

	// indices is a string of first characters of children for quick lookup
	indices string

	// children are the child nodes
	children []*radixNode

	// handlers are the handlers for this node (nil if not a leaf)
	handlers HandlersChain

	// nType is the type of this node
	nType nodeType

	// priority is used for ordering children
	priority uint32

	// wildChild indicates if this node has a wildcard child
	wildChild bool

	// fullPath is the complete path up to this node (for debugging)
	fullPath string

	// paramName is the name of the parameter (for param and catchAll types)
	paramName string
}

// addRoute adds a route to the radix tree
func (n *radixNode) addRoute(path string, handlers HandlersChain) {
	fullPath := path
	n.priority++

	// Empty tree
	if len(n.path) == 0 && len(n.children) == 0 {
		n.insertChild(path, fullPath, handlers)
		return
	}

walk:
	for {
		// Find the longest common prefix
		i := longestCommonPrefix(path, n.path)

		// Split edge if needed
		if i < len(n.path) {
			child := &radixNode{
				path:      n.path[i:],
				wildChild: n.wildChild,
				nType:     static,
				indices:   n.indices,
				children:  n.children,
				handlers:  n.handlers,
				priority:  n.priority - 1,
				fullPath:  n.fullPath,
			}

			n.children = []*radixNode{child}
			n.indices = string([]byte{n.path[i]})
			n.path = path[:i]
			n.handlers = nil
			n.wildChild = false
			n.fullPath = fullPath[:len(fullPath)-len(path)+i]
		}

		// Make new node a child of this node
		if i < len(path) {
			path = path[i:]

			// Check for wildcard
			if n.wildChild {
				n = n.children[len(n.children)-1]
				n.priority++

				// Check if the wildcard matches
				if len(path) >= len(n.path) && n.path == path[:len(n.path)] &&
					n.nType != catchAll &&
					(len(n.path) >= len(path) || path[len(n.path)] == '/') {
					continue walk
				}

				// Wildcard conflict
				panic("wildcard conflict")
			}

			// Check for param or catchAll
			if path[0] == ':' || path[0] == '*' {
				n.insertChild(path, fullPath, handlers)
				return
			}

			// Look for existing child
			c := path[0]
			for i, char := range []byte(n.indices) {
				if c == char {
					n = n.children[i]
					n.priority++
					continue walk
				}
			}

			// Insert new child
			if c != ':' && c != '*' {
				n.indices += string([]byte{c})
				child := &radixNode{
					fullPath: fullPath,
				}
				n.children = append(n.children, child)
				n = child
			}

			n.insertChild(path, fullPath, handlers)
			return
		}

		// Set handlers on this node
		if n.handlers != nil {
			panic("handlers are already registered for path '" + fullPath + "'")
		}
		n.handlers = handlers
		n.fullPath = fullPath
		return
	}
}

// insertChild inserts a child node
func (n *radixNode) insertChild(path, fullPath string, handlers HandlersChain) {
	for {
		// Find prefix until first wildcard
		wildcard, i, valid := findWildcard(path)
		if i < 0 {
			break
		}

		if !valid {
			panic("invalid wildcard in path '" + fullPath + "'")
		}

		// Check for param
		if wildcard[0] == ':' {
			if i > 0 {
				// Insert prefix before param
				n.path = path[:i]
				path = path[i:]
			}

			child := &radixNode{
				nType:     param,
				path:      wildcard,
				fullPath:  fullPath,
				paramName: wildcard[1:],
			}

			n.wildChild = true
			n.children = append(n.children, child)
			n = child
			n.priority++

			// If path continues after param
			if len(wildcard) < len(path) {
				path = path[len(wildcard):]

				child := &radixNode{
					priority: 1,
					fullPath: fullPath,
				}
				n.children = append(n.children, child)
				n = child
				continue
			}

			// Set handlers
			n.handlers = handlers
			return
		}

		// CatchAll
		if i+len(wildcard) != len(path) {
			panic("catch-all routes must be at the end of the path")
		}

		if len(n.path) > 0 && n.path[len(n.path)-1] == '/' {
			panic("catch-all conflicts with existing handle for path segment")
		}

		i--
		if path[i] != '/' {
			panic("catch-all must be preceded by '/'")
		}

		n.path = path[:i]

		// Create catchAll node
		child := &radixNode{
			wildChild: true,
			nType:     catchAll,
			fullPath:  fullPath,
		}
		n.children = append(n.children, child)
		n.indices = string('/')
		n = child
		n.priority++

		// Create leaf node
		child = &radixNode{
			path:      path[i:],
			nType:     catchAll,
			handlers:  handlers,
			priority:  1,
			fullPath:  fullPath,
			paramName: wildcard[1:],
		}
		n.children = append(n.children, child)

		return
	}

	// No wildcard found
	n.path = path
	n.handlers = handlers
	n.fullPath = fullPath
}

// getValue finds the handlers and params for a path
func (n *radixNode) getValue(path string, params map[string]string) (handlers HandlersChain, tsr bool) {
walk:
	for {
		prefix := n.path
		if len(path) > len(prefix) {
			if path[:len(prefix)] == prefix {
				path = path[len(prefix):]

				// Try static children first
				c := path[0]
				for i, char := range []byte(n.indices) {
					if c == char {
						n = n.children[i]
						continue walk
					}
				}

				// Try wildcard child
				if n.wildChild {
					n = n.children[len(n.children)-1]

					switch n.nType {
					case param:
						// Find param end
						end := 0
						for end < len(path) && path[end] != '/' {
							end++
						}

						// Save param
						if params != nil {
							params[n.paramName] = path[:end]
						}

						// Continue if path has more
						if end < len(path) {
							if len(n.children) > 0 {
								path = path[end:]
								n = n.children[0]
								continue walk
							}

							// Trailing slash redirect
							tsr = len(path) == end+1
							return
						}

						if handlers = n.handlers; handlers != nil {
							return
						}

						// Check for trailing slash redirect
						if len(n.children) == 1 {
							n = n.children[0]
							tsr = n.path == "/" && n.handlers != nil
						}
						return

					case catchAll:
						// Save param
						if params != nil {
							params[n.paramName] = path
						}
						handlers = n.handlers
						return

					default:
						panic("invalid node type")
					}
				}

				// No match
				tsr = path == "/" && n.handlers != nil
				return
			}
		}

		if path == prefix {
			// Found exact match
			if handlers = n.handlers; handlers != nil {
				return
			}

			// Check for trailing slash redirect
			if path == "/" && n.wildChild && n.nType != static {
				tsr = true
				return
			}

			// Check children for trailing slash
			for i, c := range []byte(n.indices) {
				if c == '/' {
					n = n.children[i]
					tsr = (len(n.path) == 1 && n.handlers != nil) ||
						(n.nType == catchAll && n.children[0].handlers != nil)
					return
				}
			}
			return
		}

		// Nothing found, check for trailing slash redirect
		tsr = (path == "/") ||
			(len(prefix) == len(path)+1 && prefix[len(path)] == '/' &&
				path == prefix[:len(prefix)-1] && n.handlers != nil)
		return
	}
}

// findWildcard finds a wildcard segment in a path
func findWildcard(path string) (wildcard string, i int, valid bool) {
	for start, c := range []byte(path) {
		if c != ':' && c != '*' {
			continue
		}

		// Find end of segment
		valid = true
		for end, c := range []byte(path[start+1:]) {
			switch c {
			case '/':
				return path[start : start+1+end], start, valid
			case ':', '*':
				valid = false
			}
		}
		return path[start:], start, valid
	}
	return "", -1, false
}

// longestCommonPrefix finds the longest common prefix
func longestCommonPrefix(a, b string) int {
	i := 0
	max := len(a)
	if len(b) < max {
		max = len(b)
	}
	for i < max && a[i] == b[i] {
		i++
	}
	return i
}

// --- Router Methods ---

// Use adds global middleware
func (r *Router) Use(middleware ...HandlerFunc) {
	r.middleware = append(r.middleware, middleware...)
}

// Handle registers a route with a specific method
func (r *Router) Handle(method, path string, handlers ...HandlerFunc) {
	if len(path) == 0 || path[0] != '/' {
		panic("path must begin with '/'")
	}

	// Get or create tree for method
	root := r.trees[method]
	if root == nil {
		root = &radixNode{}
		r.trees[method] = root
	}

	// Combine global middleware with route handlers
	finalHandlers := r.middleware.combine(handlers)

	// Add route to tree
	root.addRoute(path, finalHandlers)

	// Update maxParams
	if pc := countParams(path); pc > r.maxParams {
		r.maxParams = pc
	}
}

// GET registers a GET route
func (r *Router) GET(path string, handlers ...HandlerFunc) {
	r.Handle(MethodGet, path, handlers...)
}

// POST registers a POST route
func (r *Router) POST(path string, handlers ...HandlerFunc) {
	r.Handle(MethodPost, path, handlers...)
}

// PUT registers a PUT route
func (r *Router) PUT(path string, handlers ...HandlerFunc) {
	r.Handle(MethodPut, path, handlers...)
}

// DELETE registers a DELETE route
func (r *Router) DELETE(path string, handlers ...HandlerFunc) {
	r.Handle(MethodDelete, path, handlers...)
}

// PATCH registers a PATCH route
func (r *Router) PATCH(path string, handlers ...HandlerFunc) {
	r.Handle(MethodPatch, path, handlers...)
}

// HEAD registers a HEAD route
func (r *Router) HEAD(path string, handlers ...HandlerFunc) {
	r.Handle(MethodHead, path, handlers...)
}

// OPTIONS registers an OPTIONS route
func (r *Router) OPTIONS(path string, handlers ...HandlerFunc) {
	r.Handle(MethodOptions, path, handlers...)
}

// Any registers a route for all HTTP methods
func (r *Router) Any(path string, handlers ...HandlerFunc) {
	r.GET(path, handlers...)
	r.POST(path, handlers...)
	r.PUT(path, handlers...)
	r.DELETE(path, handlers...)
	r.PATCH(path, handlers...)
	r.HEAD(path, handlers...)
	r.OPTIONS(path, handlers...)
}

// Match registers a route for multiple methods
func (r *Router) Match(methods []string, path string, handlers ...HandlerFunc) {
	for _, method := range methods {
		r.Handle(method, path, handlers...)
	}
}

// Group creates a new route group
func (r *Router) Group(prefix string, middleware ...HandlerFunc) *RouteGroup {
	group := &RouteGroup{
		prefix:     prefix,
		middleware: middleware,
		router:     r,
	}
	r.groups = append(r.groups, group)
	return group
}

// Static registers a static file handler
func (r *Router) Static(path, root string) {
	if !strings.HasSuffix(path, "/*path") {
		path = strings.TrimSuffix(path, "/") + "/*path"
	}

	handler := func(c *Context) {
		filePath := c.Param("path")
		fullPath := root + "/" + filePath

		// Check if file exists
		c.File(fullPath)
	}

	r.GET(path, handler)
	r.HEAD(path, handler)
}

// StaticFile registers a single static file handler
func (r *Router) StaticFile(path, filepath string) {
	handler := func(c *Context) {
		c.File(filepath)
	}

	r.GET(path, handler)
	r.HEAD(path, handler)
}

// NotFound sets the handler for 404 Not Found
func (r *Router) NotFound(handler HandlerFunc) {
	r.notFoundHandler = handler
}

// MethodNotAllowed sets the handler for 405 Method Not Allowed
func (r *Router) MethodNotAllowed(handler HandlerFunc) {
	r.methodNotAllowedHandler = handler
}

// ServeHTTP implements the http.Handler interface
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Get context from pool
	c := r.pool.Get().(*Context)
	c.reset(w, req)
	defer r.pool.Put(c)

	// Wrap response writer
	rw := NewResponseWriter(w)
	c.Writer = rw

	// Handle panics
	defer func() {
		if err := recover(); err != nil {
			if r.panicHandler != nil {
				r.panicHandler(c, err)
			} else {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}
	}()

	// Find route
	path := req.URL.Path
	method := req.Method

	if root := r.trees[method]; root != nil {
		handlers, tsr := root.getValue(path, c.params)

		if handlers != nil {
			c.handlers = handlers
			c.Next()
			return
		}

		// Handle trailing slash redirect
		if r.RedirectTrailingSlash && tsr {
			redirectPath := path
			if len(path) > 1 && path[len(path)-1] == '/' {
				redirectPath = path[:len(path)-1]
			} else {
				redirectPath = path + "/"
			}

			http.Redirect(w, req, redirectPath, http.StatusMovedPermanently)
			return
		}
	}

	// Handle 405 Method Not Allowed
	if r.HandleMethodNotAllowed {
		for m, root := range r.trees {
			if m == method {
				continue
			}
			handlers, _ := root.getValue(path, nil)
			if handlers != nil {
				if r.HandleOPTIONS && method == MethodOptions {
					// Collect allowed methods
					allowed := make([]string, 0, len(r.trees))
					for m := range r.trees {
						if handlers, _ := r.trees[m].getValue(path, nil); handlers != nil {
							allowed = append(allowed, m)
						}
					}
					w.Header().Set("Allow", strings.Join(allowed, ", "))
					w.WriteHeader(http.StatusNoContent)
					return
				}

				if r.methodNotAllowedHandler != nil {
					c.handlers = HandlersChain{r.methodNotAllowedHandler}
					c.Next()
				} else {
					http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
				}
				return
			}
		}
	}

	// Handle 404 Not Found
	if r.notFoundHandler != nil {
		c.handlers = HandlersChain{r.notFoundHandler}
		c.Next()
	} else {
		http.NotFound(w, req)
	}
}

// countParams counts the number of params in a path
func countParams(path string) int {
	count := 0
	for i := 0; i < len(path); i++ {
		if path[i] == ':' || path[i] == '*' {
			count++
		}
	}
	return count
}

// Routes returns all registered routes
func (r *Router) Routes() []RouteInfo {
	var routes []RouteInfo
	for method, root := range r.trees {
		routes = append(routes, collectRoutes(method, root, "")...)
	}
	return routes
}

// RouteInfo contains information about a route
type RouteInfo struct {
	Method  string
	Path    string
	Handler string
}

// collectRoutes recursively collects route information
func collectRoutes(method string, node *radixNode, path string) []RouteInfo {
	var routes []RouteInfo

	currentPath := path + node.path

	if node.handlers != nil {
		routes = append(routes, RouteInfo{
			Method: method,
			Path:   currentPath,
		})
	}

	for _, child := range node.children {
		routes = append(routes, collectRoutes(method, child, currentPath)...)
	}

	return routes
}
