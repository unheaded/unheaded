# ADR-006: Vanilla JavaScript Frontend (No React/Vue/Angular)

## Status: Accepted

## Date: 2026-01-26

## Context

Unheaded has two user-facing frontends: the main **Dashboard** (real-time packet visualization, system metrics) and the **Kanban App** (the "Meta Moment" -- Unheaded tracking its own development). Both require interactive UIs with real-time WebSocket updates, drag-and-drop interactions, and canvas-based visualizations.

The modern frontend ecosystem overwhelmingly defaults to React, Vue, or Angular for any interactive web application. These frameworks provide component models, state management, virtual DOM diffing, and rich ecosystems of UI libraries. However, they also bring significant baggage:

1. **npm and node_modules**: A React project typically pulls in 200-1,000+ npm packages. Each is a supply chain risk (cf. event-stream, ua-parser-js, colors.js, left-pad). This directly conflicts with ADR-004 (no-external-deps policy).
2. **Build toolchain complexity**: Webpack/Vite/esbuild/Babel/TypeScript -- the JavaScript build pipeline is notoriously complex, fragile, and resource-intensive. A broken `node_modules` can block the entire team.
3. **Bundle size**: A minimal React app ships ~130KB of framework JavaScript before any application code. Vue is ~80KB, Angular ~200KB+.
4. **Runtime overhead**: Virtual DOM diffing, reconciliation, and framework event systems add latency to every user interaction. For a real-time packet visualization updating at 20Hz, this overhead matters.
5. **Server-side dependency**: React/Vue/Angular typically require a Node.js server or build step. Unheaded's backend is Go; introducing Node.js adds a second runtime to operate and secure.

The question was whether the interactive requirements of the Dashboard and Kanban App justified adopting a framework, or whether vanilla JavaScript could meet all requirements.

## Decision

We use **vanilla HTML + CSS + JavaScript** for all frontend code. No framework, no npm, no node_modules, no build step, no transpilation.

The implementation pattern is:

```
Go backend (embed directive)
  └── static/
      ├── index.html        # Single HTML file
      ├── css/
      │   ├── main.css      # Kingdom theming (dark + gold)
      │   ├── board.css     # Layout-specific styles
      │   └── cards.css     # Component-specific styles
      └── js/
          ├── app.js        # Main orchestrator
          ├── board.js       # Board state management
          ├── cards.js       # Card components, drag-and-drop
          ├── api.js         # REST API client
          └── websocket.js   # WebSocket real-time updates
```

Key technical decisions:

- **Go `embed` directive** serves static files directly from the compiled binary. No separate file server, no CDN, no build artifacts. Single binary deployment.
- **No build step**: JavaScript files are shipped as-is. Modern browsers natively support ES modules, `fetch()`, WebSocket API, Canvas API, CSS Grid, and CSS Custom Properties. No transpilation needed.
- **Canvas API** for the Dashboard packet-flow visualization (particle animations inspired by bellis.tech). Canvas provides direct pixel control with no DOM overhead.
- **Native WebSocket API** for real-time updates from dashboard-backend. No Socket.IO, no SockJS.
- **CSS Custom Properties** for theming (dark theme with gold accents). No Sass, no styled-components.
- **Vanilla drag-and-drop API** for Kanban card interactions.

Reference implementations that validated this approach exist in the team's other projects:
- `~/tmp/weather-daemon-main/weather.js` -- Python backend, vanilla JS frontend
- `~/tmp/rss-daemon-main/frontend.js` -- Pure JS RSS display
- `~/tmp/www-main/html/` -- Static HTML served by Python

## Consequences

### Positive

- **Zero supply chain risk on the frontend**: No npm packages, no node_modules, no package-lock.json. The frontend cannot be compromised by a supply chain attack on an npm dependency.
- **No build step**: `go build` compiles the entire application -- backend and frontend -- into a single binary. No webpack, no Vite, no esbuild. The CI/CD pipeline is simpler by an order of magnitude.
- **Minimal payload**: The Dashboard frontend is approximately 4,500 lines of JavaScript, served as-is. No framework overhead, no unused library code. First contentful paint is near-instant.
- **Full control**: Canvas rendering, WebSocket management, and DOM manipulation are all done with direct browser APIs. There is no framework abstraction layer between the developer and the browser.
- **Single binary deployment**: The Go `embed` directive bakes static files into the binary. Deployment is copying one file. No `npm install`, no `npm run build`, no artifact storage for frontend bundles.
- **No Node.js runtime**: The operations team does not need to manage, secure, or update a Node.js installation. The entire application is Go + Rust.
- **Performance**: Direct DOM manipulation and Canvas API rendering avoid virtual DOM overhead. For the 20Hz packet-flow visualization, this eliminates a significant source of jank.
- **Consistency with ADR-004**: The no-external-deps policy applies uniformly across the entire stack, not just the backend.

### Negative

- **No component model**: Vanilla JS has no built-in component abstraction. UI components are managed through conventions (one JS file per feature area) rather than a framework-enforced structure. As the UI grows, this requires discipline.
- **No declarative rendering**: React's JSX and Vue's templates make UI structure self-documenting. Vanilla JS DOM manipulation (`createElement`, `innerHTML`, `appendChild`) is more verbose and harder to review.
- **Manual state management**: Without Redux/Vuex/Pinia, UI state is managed through module-level variables and event listeners. The Kanban board state in `board.js` must be carefully synchronized with the API and WebSocket updates.
- **Limited hiring pool**: Most frontend developers in 2026 have framework experience but limited experience with vanilla JS at scale. Candidates who can write performant, maintainable vanilla JS are less common.
- **Testing limitations**: No Jest, no React Testing Library, no Cypress component testing. Frontend testing relies on E2E browser tests, which are slower and less granular.
- **Accessibility**: Frameworks like React have mature accessibility libraries (react-aria, reach-ui). Vanilla JS requires manual ARIA attribute management.

## References

- `cmd/kanban-app/static/` -- Kanban frontend (vanilla HTML/CSS/JS)
- `dashboard/` -- Dashboard frontend (vanilla HTML/CSS/JS with Canvas API)
- `docs/MICROSERVICES.md` -- "The Purity of Interface" section
- `docs/THE_META_MOMENT.md` -- Kanban App tech stack description
- ADR-004 -- No-External-Dependencies Policy
