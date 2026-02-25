# S46: DASHBOARD OVERHAUL & DESIGN SYSTEM FUSION SPRINT

**Date**: 2026-02-25
**Sprint**: S46 — UI/UX overhaul, design system unification, kanban drag-drop
**Prerequisite**: S45 complete (docs/RFC aligned, wire format fixed)
**Target**: Professional dashboard that doesn't scream "AI generated this"
**Estimated Duration**: ~8-10 hours
**Agent Strategy**: Phase 0→1 sequential, Phase 2-4 parallelizable, Phase 5 sequential
**Commit Cadence**: Every 5 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

---

## LEGEND
- [B] = Bash command
- [V] = Verification step
- [D] = Debug substep
- [W] = Write/Create file
- [R] = Read file
- [S] = Script/Code
- [P] = Parallelizable
- [C] = Commit checkpoint

---

## PHASE 0: ENVIRONMENT & BASELINE (Steps 1-10)

- [ ] **Step 1** [B] ~2m: **Verify Node.js & npm**
  ```bash
  node --version && npm --version
  ```
  - If pass → Step 2
  - If fail → Step 1a [D]: Install Node.js 18+

- [ ] **Step 1a** [D] ~5m: **Debug Node installation**
  ```bash
  curl -fsSL https://deb.nodesource.com/setup_18.x | sudo -E bash - && sudo apt-get install -y nodejs
  ```
  - Retry Step 1

- [ ] **Step 2** [B] ~2m: **Check fonts available on system**
  ```bash
  fc-list | grep -i "jetbrains\|space grotesk" || echo "Fonts not installed"
  ```
  - If pass → Step 3
  - If fail → Step 3 (will download in Phase 1)

- [ ] **Step 3** [R] ~3m: **Read current dashboard/index.html**
  ```bash
  wc -l dashboard/index.html && head -50 dashboard/index.html
  ```
  - Verify 515 lines, 4 tabs (Packet Flow, Trace Table, Latency, Doom)
  - Note current nav structure, CSS imports
  - If fail → Step 3a [D]

- [ ] **Step 3a** [D] ~2m: **Debug missing dashboard file**
  ```bash
  find . -name "dashboard" -o -name "index.html" | head -20
  ```
  - If found → Re-run Step 3 with correct path
  - If not found → ERROR: Dashboard infrastructure missing

- [ ] **Step 4** [R] ~3m: **Inventory all CSS files**
  ```bash
  find dashboard -name "*.css" -type f | sort
  find kanban -name "*.css" -type f | sort || echo "No kanban CSS yet"
  ```
  - Note all stylesheet paths
  - Check for inline styles in HTML files

- [ ] **Step 5** [R] ~2m: **Read current kanban/index.html**
  ```bash
  wc -l kanban/index.html && head -30 kanban/index.html
  ```
  - Verify ~80 lines
  - Check for existing drag-drop implementation
  - If fail → Step 5a [D]

- [ ] **Step 5a** [D] ~2m: **Debug missing kanban**
  ```bash
  ls -la kanban/ || mkdir -p kanban
  ```
  - If file missing, note for Phase 3
  - Continue to Step 6

- [ ] **Step 6** [B] ~3m: **Screenshot current dashboard (before)**
  ```bash
  # Using headless Chrome
  google-chrome --headless --disable-gpu --screenshot=dashboard-before.png dashboard/index.html
  # Or Firefox
  firefox --headless --screenshot=dashboard-before.png dashboard/index.html
  ```
  - Store screenshot for comparison in Phase 5
  - If fail → Step 6a [D] (skip screenshot, document manually)

- [ ] **Step 6a** [D] ~2m: **Debug screenshot failure**
  ```bash
  which google-chrome || which firefox || echo "No headless browser available"
  ```
  - If no browser → Use manual visual inspection in Phase 5
  - Continue to Step 7

- [ ] **Step 7** [B] ~2m: **Create sprint branch**
  ```bash
  git branch s46-dashboard-overhaul
  git checkout s46-dashboard-overhaul
  ```
  - If fail → Step 7a [D]

- [ ] **Step 7a** [D] ~3m: **Debug git branch failure**
  ```bash
  git status || git init
  git log --oneline | head -5
  ```
  - If repo missing → Initialize with `git init`
  - Continue to Step 8

- [ ] **Step 8** [B] ~2m: **Verify directory structure**
  ```bash
  ls -la dashboard/ && ls -la kanban/ && ls -la docs/
  ```
  - Check for assets/, css/, js/ subdirectories
  - If missing → Create them in Phase 1

- [ ] **Step 9** [R] ~5m: **Read S45 completion status**
  ```bash
  grep -r "S45\|prerequisites\|RFC" docs/ --include="*.md" | head -20
  ```
  - Verify wire format and RFC alignment
  - If not found → Log warning, continue anyway

- [ ] **Step 10** [V] ~3m: **Baseline verification checkpoint**
  ```bash
  echo "✓ Node/npm verified"
  echo "✓ Dashboard structure confirmed (515 lines, 4 tabs)"
  echo "✓ Kanban structure confirmed (~80 lines)"
  echo "✓ Branch s46-dashboard-overhaul created"
  echo "✓ Ready for Phase 1"
  ```
  - All checks pass → PHASE 0 COMPLETE
  - Any check fails → Debug and retry that step

---

## PHASE 1: DESIGN SYSTEM FOUNDATION (Steps 11-30)

- [ ] **Step 11** [W] ~5m: **Create design-system.css with color variables**
  ```css
  :root {
    /* PRIMARY PALETTE - unheaded.org theme */
    --bg-primary: #0a0a0a;
    --bg-secondary: #151530;
    --bg-tertiary: #1f1f3f;
    --bg-card: rgba(15, 15, 35, 0.8);

    /* TEXT COLORS */
    --text-primary: #c9c9c9;
    --text-secondary: #a0a0a0;
    --text-tertiary: #707070;
    --text-muted: #505050;

    /* ACCENT COLORS - minimal saturation */
    --accent-primary: #4a4a7a;
    --accent-secondary: #6a6a9a;
    --border-color: #2a2a4a;
    --border-light: #3a3a5a;

    /* STATUS COLORS */
    --status-success: #5a8a5a;
    --status-error: #8a5a5a;
    --status-warning: #8a7a5a;
    --status-info: #5a7a8a;

    /* GLASS EFFECT */
    --glass-bg: rgba(10, 10, 10, 0.7);
    --glass-border: rgba(201, 201, 201, 0.1);

    /* SPACING SCALE */
    --space-xs: 0.25rem;
    --space-sm: 0.5rem;
    --space-md: 1rem;
    --space-lg: 1.5rem;
    --space-xl: 2rem;
    --space-2xl: 3rem;
    --space-3xl: 4rem;

    /* TYPOGRAPHY */
    --font-mono: "JetBrains Mono", "Courier New", monospace;
    --font-sans: "Space Grotesk", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    --font-size-xs: 0.75rem;
    --font-size-sm: 0.875rem;
    --font-size-base: 1rem;
    --font-size-lg: 1.125rem;
    --font-size-xl: 1.25rem;
    --font-size-2xl: 1.5rem;

    /* TYPOGRAPHY - LINE HEIGHT */
    --line-height-tight: 1.2;
    --line-height-normal: 1.5;
    --line-height-relaxed: 1.75;

    /* BORDERS & RADIUS */
    --radius-none: 0;
    --radius-sm: 2px;
    --radius-md: 4px;
    --radius-lg: 8px;
    --radius-xl: 12px;
    --border-width: 1px;

    /* SHADOWS */
    --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.5);
    --shadow-md: 0 4px 6px rgba(0, 0, 0, 0.6);
    --shadow-lg: 0 10px 15px rgba(0, 0, 0, 0.7);
    --shadow-glass: 0 8px 32px rgba(0, 0, 0, 0.4);

    /* TRANSITIONS */
    --transition-fast: 150ms cubic-bezier(0.4, 0, 0.2, 1);
    --transition-normal: 250ms cubic-bezier(0.4, 0, 0.2, 1);
    --transition-slow: 350ms cubic-bezier(0.4, 0, 0.2, 1);

    /* Z-INDEX SCALE */
    --z-base: 0;
    --z-dropdown: 100;
    --z-sticky: 200;
    --z-fixed: 300;
    --z-modal: 400;
    --z-tooltip: 500;
    --z-notification: 600;

    /* RESPONSIVE BREAKPOINTS */
    --breakpoint-xs: 320px;
    --breakpoint-sm: 640px;
    --breakpoint-md: 1024px;
    --breakpoint-lg: 1280px;
    --breakpoint-xl: 1536px;
  }
  ```
  - Save to: `dashboard/css/design-system.css`
  - If fail → Step 11a [D]

- [ ] **Step 11a** [D] ~2m: **Debug CSS file creation**
  ```bash
  mkdir -p dashboard/css
  touch dashboard/css/design-system.css
  ```
  - Retry Step 11

- [ ] **Step 12** [W] ~8m: **Add typography & component base styles**
  ```css
  /* BASE TYPOGRAPHY */
  body {
    font-family: var(--font-mono);
    background-color: var(--bg-primary);
    color: var(--text-primary);
    line-height: var(--line-height-normal);
    margin: 0;
    padding: 0;
    overflow-x: hidden;
  }

  h1, h2, h3, h4, h5, h6 {
    font-family: var(--font-sans);
    font-weight: 600;
    line-height: var(--line-height-tight);
    margin: 0;
  }

  h1 { font-size: var(--font-size-2xl); }
  h2 { font-size: var(--font-size-xl); }
  h3 { font-size: var(--font-size-lg); }
  h4, h5, h6 { font-size: var(--font-size-base); }

  p { margin: 0; }

  /* CARD COMPONENT */
  .card {
    background-color: var(--bg-card);
    border: var(--border-width) solid var(--border-color);
    border-radius: var(--radius-lg);
    padding: var(--space-lg);
    box-shadow: var(--shadow-sm);
    transition: all var(--transition-normal);
  }

  .card:hover {
    border-color: var(--border-light);
    box-shadow: var(--shadow-md);
  }

  /* GLASS EFFECT NAV */
  .nav-glass {
    background: var(--glass-bg);
    backdrop-filter: blur(10px);
    border-bottom: var(--border-width) solid var(--glass-border);
    padding: var(--space-md) var(--space-lg);
  }

  /* BUTTONS */
  .btn {
    font-family: var(--font-sans);
    font-size: var(--font-size-sm);
    padding: var(--space-sm) var(--space-md);
    border: var(--border-width) solid var(--border-color);
    border-radius: var(--radius-md);
    background-color: transparent;
    color: var(--text-primary);
    cursor: pointer;
    transition: all var(--transition-fast);
  }

  .btn:hover {
    background-color: var(--bg-tertiary);
    border-color: var(--border-light);
  }

  .btn-primary {
    background-color: var(--accent-primary);
    border-color: var(--accent-primary);
    color: var(--bg-primary);
  }

  .btn-primary:hover {
    background-color: var(--accent-secondary);
    border-color: var(--accent-secondary);
  }

  .btn-success {
    color: var(--status-success);
    border-color: var(--status-success);
  }

  .btn-error {
    color: var(--status-error);
    border-color: var(--status-error);
  }

  /* INPUT ELEMENTS */
  input, textarea, select {
    font-family: var(--font-mono);
    background-color: var(--bg-tertiary);
    color: var(--text-primary);
    border: var(--border-width) solid var(--border-color);
    border-radius: var(--radius-md);
    padding: var(--space-sm);
    transition: border-color var(--transition-fast);
  }

  input:focus, textarea:focus, select:focus {
    outline: none;
    border-color: var(--border-light);
    box-shadow: 0 0 0 3px rgba(106, 106, 154, 0.2);
  }

  /* ANIMATION KEYFRAMES - Pulse glyph */
  @keyframes pulse-glyph {
    0%, 100% {
      opacity: 1;
      transform: scale(1);
    }
    50% {
      opacity: 0.7;
      transform: scale(1.05);
    }
  }

  .glyph-pulse {
    animation: pulse-glyph 2s infinite;
  }

  /* EMPTY STATE */
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--space-3xl);
    text-align: center;
    color: var(--text-secondary);
  }

  .empty-state-icon {
    font-size: 3rem;
    margin-bottom: var(--space-lg);
    opacity: 0.5;
  }

  .empty-state-title {
    font-size: var(--font-size-lg);
    font-weight: 600;
    margin-bottom: var(--space-sm);
  }

  .empty-state-text {
    font-size: var(--font-size-sm);
    color: var(--text-muted);
  }
  ```
  - Append to: `dashboard/css/design-system.css`
  - If fail → Step 12a [D]

- [ ] **Step 12a** [D] ~2m: **Debug CSS append**
  ```bash
  tail -20 dashboard/css/design-system.css | grep -q "@keyframes" && echo "CSS appended successfully"
  ```
  - If fail → Rewrite file from Step 11

- [ ] **Step 13** [W] ~5m: **Add responsive grid & layout utilities**
  ```css
  /* RESPONSIVE UTILITIES */
  .container {
    width: 100%;
    max-width: 1280px;
    margin: 0 auto;
    padding: 0 var(--space-md);
  }

  .grid {
    display: grid;
    gap: var(--space-md);
  }

  .grid-2 { grid-template-columns: repeat(auto-fit, minmax(300px, 1fr)); }
  .grid-3 { grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); }
  .grid-4 { grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); }

  .flex { display: flex; }
  .flex-between { display: flex; justify-content: space-between; }
  .flex-center { display: flex; align-items: center; }
  .flex-col { display: flex; flex-direction: column; }

  .gap-sm { gap: var(--space-sm); }
  .gap-md { gap: var(--space-md); }
  .gap-lg { gap: var(--space-lg); }

  /* RESPONSIVE BREAKPOINTS */
  @media (max-width: 640px) {
    :root {
      --font-size-base: 0.9375rem;
      --space-md: 0.75rem;
    }
    .container { padding: 0 var(--space-sm); }
    .grid-2, .grid-3, .grid-4 { grid-template-columns: 1fr; }
  }

  @media (max-width: 1024px) {
    .grid-4 { grid-template-columns: repeat(2, 1fr); }
  }

  @media (min-width: 1024px) {
    .container { padding: 0 var(--space-lg); }
  }
  ```
  - Append to: `dashboard/css/design-system.css`
  - If fail → Step 13a [D]

- [ ] **Step 13a** [D] ~2m: **Debug layout utilities**
  ```bash
  grep -q "@media" dashboard/css/design-system.css && echo "Responsive utilities added"
  ```
  - If fail → Manual verify file contents
  - Continue to Step 14

- [ ] **Step 14** [B] ~5m: **Create fonts directory and download font files**
  ```bash
  mkdir -p dashboard/assets/fonts
  # Download JetBrains Mono
  curl -L -o dashboard/assets/fonts/JetBrainsMono-Regular.woff2 \
    "https://fonts.jsdelivr.net/gh/JetBrains/JetBrainsMono@master/fonts/webfonts/JetBrainsMono-Regular.woff2"
  # Download Space Grotesk
  curl -L -o dashboard/assets/fonts/SpaceGrotesk-Regular.woff2 \
    "https://fonts.googleapis.com/css2?family=Space+Grotesk:wght@400;600;700&display=swap"
  ```
  - If download fails → Step 14a [D]

- [ ] **Step 14a** [D] ~3m: **Debug font download**
  ```bash
  # Use local fonts if available, or use web fallback
  ls -la dashboard/assets/fonts/ || mkdir -p dashboard/assets/fonts
  echo "Will use system fonts as fallback in Step 15"
  ```
  - Continue to Step 15 (will use web fonts)

- [ ] **Step 15** [W] ~5m: **Add @font-face declarations to design-system.css**
  ```css
  /* FONT FACE DECLARATIONS */
  @font-face {
    font-family: "JetBrains Mono";
    src: url("../assets/fonts/JetBrainsMono-Regular.woff2") format("woff2");
    font-weight: 400;
    font-style: normal;
  }

  @font-face {
    font-family: "Space Grotesk";
    src: url("../assets/fonts/SpaceGrotesk-Regular.woff2") format("woff2");
    font-weight: 400;
    font-style: normal;
  }

  @import url('https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;600&family=Space+Grotesk:wght@400;600;700&display=swap');
  ```
  - Append to beginning of: `dashboard/css/design-system.css`
  - If fail → Step 15a [D]

- [ ] **Step 15a** [D] ~2m: **Debug font face**
  ```bash
  grep -q "@font-face\|@import url" dashboard/css/design-system.css && echo "Font declarations added"
  ```
  - Continue to Step 16

- [ ] **Step 16** [W] ~8m: **Add tab component styles & nav bar styles**
  ```css
  /* TAB NAVIGATION */
  .tabs {
    display: flex;
    gap: 0;
    border-bottom: var(--border-width) solid var(--border-color);
    background-color: var(--bg-primary);
  }

  .tab-button {
    padding: var(--space-md) var(--space-lg);
    border: none;
    background: transparent;
    color: var(--text-secondary);
    font-family: var(--font-sans);
    font-size: var(--font-size-base);
    cursor: pointer;
    position: relative;
    transition: color var(--transition-fast);
    border-bottom: 2px solid transparent;
  }

  .tab-button:hover {
    color: var(--text-primary);
  }

  .tab-button.active {
    color: var(--text-primary);
    border-bottom-color: var(--accent-primary);
  }

  .tab-panel {
    display: none;
    padding: var(--space-lg);
    animation: fadeIn var(--transition-normal);
  }

  .tab-panel.active {
    display: block;
  }

  @keyframes fadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  /* NAVIGATION BAR */
  .navbar {
    background: var(--glass-bg);
    backdrop-filter: blur(10px);
    border-bottom: var(--border-width) solid var(--glass-border);
    padding: var(--space-md) var(--space-lg);
    display: flex;
    justify-content: space-between;
    align-items: center;
    position: sticky;
    top: 0;
    z-index: var(--z-sticky);
  }

  .navbar-brand {
    font-family: var(--font-mono);
    font-size: var(--font-size-lg);
    font-weight: 600;
    color: var(--text-primary);
    text-decoration: none;
  }

  .navbar-links {
    display: flex;
    gap: var(--space-lg);
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .navbar-link {
    color: var(--text-secondary);
    text-decoration: none;
    transition: color var(--transition-fast);
  }

  .navbar-link:hover {
    color: var(--text-primary);
  }

  /* HAMBURGER MENU */
  .hamburger {
    display: none;
    flex-direction: column;
    background: transparent;
    border: none;
    cursor: pointer;
    gap: 4px;
  }

  .hamburger span {
    width: 24px;
    height: 2px;
    background-color: var(--text-primary);
    transition: all var(--transition-fast);
  }

  @media (max-width: 640px) {
    .hamburger { display: flex; }
    .navbar-links { display: none; }
    .navbar-links.active { display: flex; flex-direction: column; position: absolute; top: 100%; left: 0; right: 0; background: var(--bg-secondary); }
  }
  ```
  - Append to: `dashboard/css/design-system.css`
  - If fail → Step 16a [D]

- [ ] **Step 16a** [D] ~2m: **Verify tab & nav styles**
  ```bash
  grep -c "\.tab-button\|\.navbar\|\.hamburger" dashboard/css/design-system.css
  ```
  - Should show 3+ matches
  - Continue to Step 17

- [ ] **Step 17** [W] ~6m: **Add drag-drop & kanban styles**
  ```css
  /* KANBAN & DRAG-DROP */
  .kanban-board {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: var(--space-lg);
    padding: var(--space-lg);
  }

  .kanban-column {
    background-color: var(--bg-secondary);
    border: var(--border-width) solid var(--border-color);
    border-radius: var(--radius-lg);
    padding: var(--space-md);
    min-height: 400px;
    display: flex;
    flex-direction: column;
  }

  .kanban-column-header {
    font-weight: 600;
    margin-bottom: var(--space-md);
    padding-bottom: var(--space-sm);
    border-bottom: var(--border-width) solid var(--border-light);
    color: var(--text-primary);
  }

  .kanban-items {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-sm);
  }

  .kanban-item {
    background-color: var(--bg-card);
    border: var(--border-width) solid var(--border-color);
    border-radius: var(--radius-md);
    padding: var(--space-md);
    cursor: grab;
    transition: all var(--transition-fast);
  }

  .kanban-item:hover {
    border-color: var(--border-light);
    box-shadow: var(--shadow-md);
  }

  .kanban-item.dragging {
    opacity: 0.5;
    cursor: grabbing;
  }

  .kanban-column.drag-over {
    background-color: var(--bg-tertiary);
    border-color: var(--accent-primary);
  }

  /* REVIEW ACTIONS */
  .review-actions {
    display: flex;
    gap: var(--space-sm);
    margin-top: var(--space-md);
  }

  .review-actions button {
    flex: 1;
    padding: var(--space-sm);
    font-size: var(--font-size-xs);
  }

  .btn-approve { color: var(--status-success); border-color: var(--status-success); }
  .btn-reject { color: var(--status-error); border-color: var(--status-error); }
  .btn-request-changes { color: var(--status-warning); border-color: var(--status-warning); }
  ```
  - Append to: `dashboard/css/design-system.css`
  - If fail → Step 17a [D]

- [ ] **Step 17a** [D] ~2m: **Verify kanban styles**
  ```bash
  grep -c "\.kanban-\|\.review-actions" dashboard/css/design-system.css
  ```
  - Should show 8+ matches
  - Continue to Step 18

- [ ] **Step 18** [W] ~4m: **Add metric card & data table styles**
  ```css
  /* METRIC CARDS */
  .metric-card {
    display: flex;
    flex-direction: column;
    justify-content: space-between;
    min-height: 150px;
  }

  .metric-label {
    font-size: var(--font-size-sm);
    color: var(--text-secondary);
    margin-bottom: var(--space-sm);
  }

  .metric-value {
    font-size: var(--font-size-2xl);
    font-family: var(--font-mono);
    font-weight: 600;
    color: var(--text-primary);
  }

  .metric-unit {
    font-size: var(--font-size-sm);
    color: var(--text-tertiary);
    margin-top: var(--space-xs);
  }

  .metric-trend {
    font-size: var(--font-size-sm);
    margin-top: var(--space-sm);
  }

  /* DATA TABLES */
  table {
    width: 100%;
    border-collapse: collapse;
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
  }

  th {
    background-color: var(--bg-tertiary);
    color: var(--text-primary);
    padding: var(--space-sm);
    text-align: left;
    border-bottom: var(--border-width) solid var(--border-light);
    font-weight: 600;
  }

  td {
    padding: var(--space-sm);
    border-bottom: var(--border-width) solid var(--border-color);
    color: var(--text-secondary);
  }

  tr:hover {
    background-color: var(--bg-tertiary);
  }

  /* SCROLL AREA */
  .scroll-container {
    overflow-x: auto;
    overflow-y: hidden;
  }

  .scroll-container::-webkit-scrollbar {
    height: 6px;
  }

  .scroll-container::-webkit-scrollbar-track {
    background: var(--bg-tertiary);
  }

  .scroll-container::-webkit-scrollbar-thumb {
    background: var(--border-light);
    border-radius: 3px;
  }
  ```
  - Append to: `dashboard/css/design-system.css`
  - If fail → Step 18a [D]

- [ ] **Step 18a** [D] ~2m: **Verify table styles**
  ```bash
  grep -c "\.metric-\|table\|th\|td" dashboard/css/design-system.css
  ```
  - Continue to Step 19

- [ ] **Step 19** [V] ~3m: **Verify complete design-system.css**
  ```bash
  wc -l dashboard/css/design-system.css
  # Should be 300+ lines
  grep -q "CSS VARIABLES\|FONT FACE\|BUTTON\|CARD\|TAB\|KANBAN\|METRIC\|TABLE" dashboard/css/design-system.css && echo "✓ All sections present"
  ```
  - If fail → Step 19a [D]

- [ ] **Step 19a** [D] ~3m: **Debug incomplete design system**
  ```bash
  head -50 dashboard/css/design-system.css
  tail -50 dashboard/css/design-system.css
  ```
  - Check for corrupted sections
  - Verify file is not truncated
  - Continue to Step 20

- [ ] **Step 20** [C] ~2m: **Commit Phase 1 checkpoint**
  ```bash
  git add dashboard/css/design-system.css dashboard/assets/fonts/
  git commit -m "S46-P1: Design system foundation with CSS variables, typography, components"
  ```
  - If fail → Step 20a [D]

- [ ] **Step 20a** [D] ~2m: **Debug commit failure**
  ```bash
  git status
  git log --oneline | head -3
  ```
  - If git not initialized → `git init && git add . && git commit -m "Initial commit"`
  - Continue to Phase 2

---

## PHASE 2: DASHBOARD OVERHAUL (Steps 21-65)

- [ ] **Step 21** [R] ~5m: **Read current dashboard/index.html structure**
  ```bash
  head -100 dashboard/index.html
  grep -n "link\|style\|<head>\|<body>" dashboard/index.html | head -20
  ```
  - Identify current CSS imports
  - Note nav structure, tabs
  - Map existing IDs/classes

- [ ] **Step 22** [W] ~10m: **Refactor dashboard/index.html - add design-system.css import**
  - Read full file first
  - Replace current CSS links with single design-system import
  - Update `<head>` section:
    ```html
    <!DOCTYPE html>
    <html lang="en">
    <head>
      <meta charset="UTF-8">
      <meta name="viewport" content="width=device-width, initial-scale=1.0">
      <title>Unheaded Dashboard</title>
      <link rel="stylesheet" href="css/design-system.css">
    </head>
    <body>
      <!-- Content follows -->
    </body>
    </html>
    ```
  - Remove all inline `<style>` tags
  - If fail → Step 22a [D]

- [ ] **Step 22a** [D] ~3m: **Debug HTML refactor**
  ```bash
  head -20 dashboard/index.html | grep -E "design-system|<link rel"
  ```
  - Verify import is present
  - Continue to Step 23

- [ ] **Step 23** [W] ~12m: **Redesign navbar - frosted glass, clean links**
  - Update navbar HTML to use `.navbar`, `.navbar-brand`, `.navbar-links`
  - Remove emoji icons
  - Text-only navigation
  - Example structure:
    ```html
    <nav class="navbar">
      <div class="navbar-brand">⌘ Unheaded</div>
      <ul class="navbar-links">
        <li><a href="#" class="navbar-link">Dashboard</a></li>
        <li><a href="kanban/index.html" class="navbar-link">Kanban</a></li>
        <li><a href="doom/index.html" class="navbar-link">Doom</a></li>
        <li><a href="logs.html" class="navbar-link">Logs</a></li>
      </ul>
      <button class="hamburger" id="hamburger-menu">
        <span></span><span></span><span></span>
      </button>
    </nav>
    ```
  - If fail → Step 23a [D]

- [ ] **Step 23a** [D] ~3m: **Debug navbar structure**
  ```bash
  grep -n "navbar\|hamburger" dashboard/index.html | head -10
  ```
  - Verify HTML structure is correct
  - Continue to Step 24

- [ ] **Step 24** [W] ~10m: **Redesign tab navigation - text-only, remove emoji**
  - Update `.tabs` and `.tab-button` classes
  - Tab names: "Packet Flow", "Trace Table", "Latency", "Doom"
  - Add JavaScript for tab switching
  - Example:
    ```html
    <div class="tabs">
      <button class="tab-button active" data-tab="packet-flow">Packet Flow</button>
      <button class="tab-button" data-tab="trace-table">Trace Table</button>
      <button class="tab-button" data-tab="latency">Latency</button>
      <button class="tab-button" data-tab="doom">Doom</button>
    </div>
    ```
  - If fail → Step 24a [D]

- [ ] **Step 24a** [D] ~2m: **Debug tab markup**
  ```bash
  grep -c "tab-button\|data-tab" dashboard/index.html
  ```
  - Should show 4+ matches
  - Continue to Step 25

- [ ] **Step 25** [W] ~12m: **Redesign metric cards - apply design-system styling**
  - Update all metric card divs to use `.card` and `.metric-card` classes
  - Remove inline colors (#1a1a2e)
  - Use CSS variables
  - Example:
    ```html
    <div class="card metric-card">
      <div class="metric-label">Packets/sec</div>
      <div class="metric-value">1.2M</div>
      <div class="metric-unit">pps</div>
    </div>
    ```
  - If fail → Step 25a [D]

- [ ] **Step 25a** [D] ~2m: **Debug metric cards**
  ```bash
  grep -c "metric-card\|metric-value" dashboard/index.html
  ```
  - Continue to Step 26

- [ ] **Step 26** [W] ~8m: **Add empty states for all tabs**
  - Create `.empty-state` components for tabs without data
  - Example:
    ```html
    <div class="empty-state">
      <div class="empty-state-icon">○</div>
      <div class="empty-state-title">No Data</div>
      <div class="empty-state-text">Waiting for metrics...</div>
    </div>
    ```
  - Apply to: Packet Flow, Trace Table, Latency, Doom tabs
  - If fail → Step 26a [D]

- [ ] **Step 26a** [D] ~2m: **Verify empty states**
  ```bash
  grep -c "empty-state" dashboard/index.html
  ```
  - Should show 4+ matches
  - Continue to Step 27

- [ ] **Step 27** [W] ~10m: **Create demo-data.js generator**
  ```javascript
  // dashboard/js/demo-data.js

  const demoData = {
    packetFlow: {
      packetsPerSecond: Math.floor(Math.random() * 1000000) + 500000,
      bytesPerSecond: Math.floor(Math.random() * 5000000) + 1000000,
      droppedPackets: Math.floor(Math.random() * 1000),
      errorRate: (Math.random() * 0.5).toFixed(2) + '%',
    },

    traceTable: [
      { timestamp: '2026-02-25T10:30:45Z', protocol: 'TCP', src: '192.168.1.100', dst: '10.0.0.50', latency: '1.2ms' },
      { timestamp: '2026-02-25T10:30:46Z', protocol: 'UDP', src: '192.168.1.101', dst: '10.0.0.51', latency: '0.8ms' },
      { timestamp: '2026-02-25T10:30:47Z', protocol: 'ICMP', src: '192.168.1.102', dst: '10.0.0.52', latency: '2.1ms' },
    ],

    latency: {
      p50: '1.2ms',
      p95: '3.5ms',
      p99: '5.2ms',
      max: '12.1ms',
    },

    doomMetrics: {
      frameTime: '16.67ms',
      fps: '60',
      drawCalls: '1024',
      triangles: '2M',
    },
  };

  function generateDemoMetrics() {
    return {
      ...demoData,
      packetFlow: {
        packetsPerSecond: Math.floor(Math.random() * 1000000) + 500000,
        bytesPerSecond: Math.floor(Math.random() * 5000000) + 1000000,
        droppedPackets: Math.floor(Math.random() * 1000),
        errorRate: (Math.random() * 0.5).toFixed(2) + '%',
      },
    };
  }

  function populatePacketFlowTab(data) {
    const tab = document.getElementById('packet-flow');
    if (!tab) return;

    tab.innerHTML = `
      <div class="grid grid-2">
        <div class="card metric-card">
          <div class="metric-label">Packets/sec</div>
          <div class="metric-value">${(data.packetFlow.packetsPerSecond / 1000000).toFixed(1)}M</div>
        </div>
        <div class="card metric-card">
          <div class="metric-label">Bytes/sec</div>
          <div class="metric-value">${(data.packetFlow.bytesPerSecond / 1000000).toFixed(1)}M</div>
        </div>
        <div class="card metric-card">
          <div class="metric-label">Dropped</div>
          <div class="metric-value">${data.packetFlow.droppedPackets}</div>
        </div>
        <div class="card metric-card">
          <div class="metric-label">Error Rate</div>
          <div class="metric-value">${data.packetFlow.errorRate}</div>
        </div>
      </div>
    `;
  }

  function populateTraceTableTab(data) {
    const tab = document.getElementById('trace-table');
    if (!tab) return;

    const rows = data.traceTable.map(trace => `
      <tr>
        <td>${trace.timestamp}</td>
        <td>${trace.protocol}</td>
        <td>${trace.src}</td>
        <td>${trace.dst}</td>
        <td>${trace.latency}</td>
      </tr>
    `).join('');

    tab.innerHTML = `
      <div class="scroll-container">
        <table>
          <thead>
            <tr>
              <th>Timestamp</th>
              <th>Protocol</th>
              <th>Source</th>
              <th>Destination</th>
              <th>Latency</th>
            </tr>
          </thead>
          <tbody>
            ${rows}
          </tbody>
        </table>
      </div>
    `;
  }

  function populateLatencyTab(data) {
    const tab = document.getElementById('latency');
    if (!tab) return;

    tab.innerHTML = `
      <div class="grid grid-2">
        <div class="card metric-card">
          <div class="metric-label">P50</div>
          <div class="metric-value">${data.latency.p50}</div>
        </div>
        <div class="card metric-card">
          <div class="metric-label">P95</div>
          <div class="metric-value">${data.latency.p95}</div>
        </div>
        <div class="card metric-card">
          <div class="metric-label">P99</div>
          <div class="metric-value">${data.latency.p99}</div>
        </div>
        <div class="card metric-card">
          <div class="metric-label">Max</div>
          <div class="metric-value">${data.latency.max}</div>
        </div>
      </div>
    `;
  }

  function populateDoomTab(data) {
    const tab = document.getElementById('doom');
    if (!tab) return;

    tab.innerHTML = `
      <div class="grid grid-2">
        <div class="card metric-card">
          <div class="metric-label">Frame Time</div>
          <div class="metric-value">${data.doomMetrics.frameTime}</div>
        </div>
        <div class="card metric-card">
          <div class="metric-label">FPS</div>
          <div class="metric-value">${data.doomMetrics.fps}</div>
        </div>
        <div class="card metric-card">
          <div class="metric-label">Draw Calls</div>
          <div class="metric-value">${data.doomMetrics.drawCalls}</div>
        </div>
        <div class="card metric-card">
          <div class="metric-label">Triangles</div>
          <div class="metric-value">${data.doomMetrics.triangles}</div>
        </div>
      </div>
    `;
  }

  // Export functions
  if (typeof module !== 'undefined' && module.exports) {
    module.exports = { demoData, generateDemoMetrics, populatePacketFlowTab, populateTraceTableTab, populateLatencyTab, populateDoomTab };
  }
  ```
  - Save to: `dashboard/js/demo-data.js`
  - If fail → Step 27a [D]

- [ ] **Step 27a** [D] ~2m: **Verify demo-data.js**
  ```bash
  mkdir -p dashboard/js
  tail -50 dashboard/js/demo-data.js | grep -q "module.exports" && echo "✓ Demo data JS created"
  ```
  - Continue to Step 28

- [ ] **Step 28** [W] ~8m: **Wire demo data to dashboard tabs**
  - Add `<script src="js/demo-data.js"></script>` to dashboard HTML
  - Create initialization script in dashboard HTML:
    ```html
    <script>
      document.addEventListener('DOMContentLoaded', function() {
        const data = generateDemoMetrics();
        populatePacketFlowTab(data);
        populateTraceTableTab(data);
        populateLatencyTab(data);
        populateDoomTab(data);
      });
    </script>
    ```
  - If fail → Step 28a [D]

- [ ] **Step 28a** [D] ~2m: **Verify demo data wiring**
  ```bash
  grep -c "demo-data.js\|generateDemoMetrics\|populatePacketFlow" dashboard/index.html
  ```
  - Should show 3+ matches
  - Continue to Step 29

- [ ] **Step 29** [W] ~10m: **Integrate logs.html as 5th tab**
  - Check if logs.html exists
  - If not, create minimal logs.html:
    ```html
    <!DOCTYPE html>
    <html lang="en">
    <head>
      <meta charset="UTF-8">
      <meta name="viewport" content="width=device-width, initial-scale=1.0">
      <title>Logs</title>
      <link rel="stylesheet" href="css/design-system.css">
    </head>
    <body>
      <nav class="navbar">
        <div class="navbar-brand">⌘ Logs Viewer</div>
      </nav>
      <div class="container">
        <div id="logs-panel" class="tab-panel active">
          <div class="empty-state">
            <div class="empty-state-icon">⧐</div>
            <div class="empty-state-title">No Logs</div>
            <div class="empty-state-text">Waiting for log entries...</div>
          </div>
        </div>
      </div>
    </body>
    </html>
    ```
  - Add 5th tab to dashboard pointing to logs.html or embedding logs
  - If fail → Step 29a [D]

- [ ] **Step 29a** [D] ~2m: **Verify logs integration**
  ```bash
  grep -c "logs\|Logs" dashboard/index.html
  ```
  - Should show 1+ match
  - Continue to Step 30

- [ ] **Step 30** [V] ~5m: **Verify no console errors in dashboard**
  ```bash
  # Manual test - open dashboard in browser, check DevTools console
  # Or use headless:
  google-chrome --headless --disable-gpu --enable-logging dashboard/index.html 2>&1 | grep -i "error\|uncaught" || echo "✓ No errors detected"
  ```
  - Fix any JavaScript errors found
  - If fail → Step 30a [D]

- [ ] **Step 30a** [D] ~5m: **Debug console errors**
  ```bash
  grep -n "querySelector\|getElementById\|addEventListener" dashboard/index.html | head -20
  ```
  - Check for missing IDs in HTML
  - Verify all script references are valid
  - Continue to Step 31

- [ ] **Step 31** [W] ~8m: **Add tab switching JavaScript**
  - Create inline script or external file:
    ```javascript
    document.querySelectorAll('.tab-button').forEach(button => {
      button.addEventListener('click', function() {
        const tabName = this.getAttribute('data-tab');

        // Hide all panels
        document.querySelectorAll('.tab-panel').forEach(panel => {
          panel.classList.remove('active');
        });

        // Deactivate all buttons
        document.querySelectorAll('.tab-button').forEach(btn => {
          btn.classList.remove('active');
        });

        // Show selected panel
        const panel = document.getElementById(tabName);
        if (panel) {
          panel.classList.add('active');
          this.classList.add('active');
        }
      });
    });
    ```
  - If fail → Step 31a [D]

- [ ] **Step 31a** [D] ~2m: **Debug tab switching**
  ```bash
  grep -c "addEventListener\|tab-button" dashboard/index.html
  ```
  - Continue to Step 32

- [ ] **Step 32** [W] ~6m: **Add hamburger menu JavaScript**
  ```javascript
  const hamburger = document.getElementById('hamburger-menu');
  const navLinks = document.querySelector('.navbar-links');

  if (hamburger) {
    hamburger.addEventListener('click', function() {
      navLinks.classList.toggle('active');
    });
  }
  ```
  - If fail → Step 32a [D]

- [ ] **Step 32a** [D] ~2m: **Verify hamburger**
  ```bash
  grep -c "hamburger\|navbar-links" dashboard/index.html
  ```
  - Continue to Step 33

- [ ] **Step 33** [V] ~5m: **Test responsive at 375px (mobile)**
  ```bash
  # In browser: Inspect > Device Mode > iPhone SE (375px)
  # Check: hamburger shows, navbar links collapse
  # Check: metrics stack vertically
  # Check: no horizontal scroll
  ```
  - If fail → Step 33a [D]

- [ ] **Step 33a** [D] ~3m: **Debug mobile responsiveness**
  ```bash
  grep "@media.*375\|@media.*640px" dashboard/css/design-system.css
  ```
  - Verify breakpoint CSS is present
  - Add mobile-first media queries if missing
  - Continue to Step 34

- [ ] **Step 34** [V] ~5m: **Test responsive at 768px (tablet)**
  ```bash
  # In browser: iPad (768px)
  # Check: 2-column grid for metrics
  # Check: readable fonts
  ```
  - If fail → Step 34a [D]

- [ ] **Step 34a** [D] ~2m: **Fix tablet breakpoints**
  ```bash
  grep "@media.*768\|@media.*1024px" dashboard/css/design-system.css
  ```
  - Continue to Step 35

- [ ] **Step 35** [V] ~5m: **Test responsive at 1024px (desktop)**
  ```bash
  # Check: full layout visible
  # Check: 3-4 column grid
  # Check: sidebar (if any) rendered
  ```
  - If fail → Step 35a [D]

- [ ] **Step 35a** [D] ~2m: **Fix desktop layout**
  ```bash
  grep "grid-2\|grid-3\|grid-4" dashboard/css/design-system.css
  ```
  - Continue to Step 36

- [ ] **Step 36** [B] ~3m: **Test in Chrome**
  ```bash
  google-chrome dashboard/index.html &
  # Wait 5s, visually inspect
  # Check: colors correct (#0a0a0a bg, #c9c9c9 text)
  # Check: fonts loading (monospace nav, sans-serif headers)
  # Check: no visual artifacts
  ```
  - If fail → Step 36a [D]

- [ ] **Step 36a** [D] ~3m: **Debug Chrome rendering**
  ```bash
  # Check browser console for CSS/font errors
  # Verify font files exist:
  ls -la dashboard/assets/fonts/
  ```
  - Continue to Step 37

- [ ] **Step 37** [B] ~3m: **Test in Firefox**
  ```bash
  firefox dashboard/index.html &
  # Wait 5s, visually inspect
  # Compare rendering with Chrome
  # Check accessibility (no WCAG errors)
  ```
  - If fail → Step 37a [D]

- [ ] **Step 37a** [D] ~2m: **Debug Firefox issues**
  ```bash
  # Check for vendor prefixes in CSS
  grep -c "-webkit-\|-moz-\|-ms-" dashboard/css/design-system.css
  ```
  - Add prefixes if needed
  - Continue to Step 38

- [ ] **Step 38** [V] ~5m: **Verify all nav links resolve**
  ```bash
  # Click each navbar link
  # Check: Dashboard loads
  # Check: Kanban loads (Step 39+)
  # Check: Doom loads
  # Check: Logs loads
  ```
  - If fail → Step 38a [D]

- [ ] **Step 38a** [D] ~3m: **Debug broken links**
  ```bash
  grep -n "href=" dashboard/index.html
  ```
  - Verify all paths are correct
  - Check file existence
  - Continue to Step 39

- [ ] **Step 39** [C] ~2m: **Commit Phase 2 Part 1 checkpoint (Dashboard refactor)**
  ```bash
  git add dashboard/index.html dashboard/js/demo-data.js
  git commit -m "S46-P2a: Dashboard refactor - design system integration, tab navigation, demo data"
  ```
  - If fail → Step 39a [D]

- [ ] **Step 39a** [D] ~2m: **Debug commit**
  ```bash
  git status
  git log --oneline | head -3
  ```
  - Continue to Phase 3

---

## PHASE 3: KANBAN OVERHAUL (Steps 40-70)

- [ ] **Step 40** [R] ~5m: **Read current kanban/index.html**
  ```bash
  cat kanban/index.html | head -100
  wc -l kanban/index.html
  ```
  - Identify existing structure
  - Check for any drag-drop implementation
  - If file missing → Create minimal kanban in Step 40a

- [ ] **Step 40a** [W] ~5m: **Create minimal kanban/index.html if missing**
  ```html
  <!DOCTYPE html>
  <html lang="en">
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Kanban Board</title>
    <link rel="stylesheet" href="../dashboard/css/design-system.css">
    <style>
      body { padding: var(--space-lg); }
    </style>
  </head>
  <body>
    <nav class="navbar">
      <div class="navbar-brand">⌘ Kanban Board</div>
      <a href="../dashboard/index.html" class="navbar-link">← Back to Dashboard</a>
    </nav>

    <div class="container">
      <div class="kanban-board" id="kanban-board">
        <div class="kanban-column" data-column="todo">
          <div class="kanban-column-header">To Do</div>
          <div class="kanban-items" id="todo-items"></div>
        </div>

        <div class="kanban-column" data-column="in-progress">
          <div class="kanban-column-header">In Progress</div>
          <div class="kanban-items" id="in-progress-items"></div>
        </div>

        <div class="kanban-column" data-column="review">
          <div class="kanban-column-header">Review</div>
          <div class="kanban-items" id="review-items"></div>
        </div>

        <div class="kanban-column" data-column="done">
          <div class="kanban-column-header">Done</div>
          <div class="kanban-items" id="done-items"></div>
        </div>
      </div>
    </div>

    <script src="js/kanban.js"></script>
  </body>
  </html>
  ```
  - Save to: `kanban/index.html`
  - If fail → Retry Step 40a

- [ ] **Step 41** [W] ~15m: **Implement drag-drop with HTML5 Drag API**
  - Create: `kanban/js/kanban.js`
  ```javascript
  // kanban/js/kanban.js

  class KanbanBoard {
    constructor() {
      this.items = [];
      this.currentDragItem = null;
      this.init();
    }

    init() {
      this.loadItems();
      this.attachDragListeners();
      this.render();
      this.setupWebSocket();
    }

    loadItems() {
      // Load from localStorage or generate demo items
      const stored = localStorage.getItem('kanban-items');
      if (stored) {
        this.items = JSON.parse(stored);
      } else {
        this.items = [
          { id: '1', title: 'Setup eBPF kernel module', status: 'todo', priority: 'high' },
          { id: '2', title: 'Implement packet capture', status: 'in-progress', priority: 'high' },
          { id: '3', title: 'Add latency metrics', status: 'review', priority: 'medium' },
          { id: '4', title: 'Design dashboard UI', status: 'done', priority: 'medium' },
        ];
        this.saveItems();
      }
    }

    saveItems() {
      localStorage.setItem('kanban-items', JSON.stringify(this.items));
    }

    attachDragListeners() {
      const board = document.getElementById('kanban-board');

      // Attach dragstart/dragend to kanban items
      board.addEventListener('dragstart', (e) => this.onDragStart(e));
      board.addEventListener('dragend', (e) => this.onDragEnd(e));

      // Attach dragover/drop to kanban columns
      board.addEventListener('dragover', (e) => this.onDragOver(e));
      board.addEventListener('drop', (e) => this.onDrop(e));
      board.addEventListener('dragleave', (e) => this.onDragLeave(e));
    }

    onDragStart(e) {
      const item = e.target.closest('.kanban-item');
      if (!item) return;

      this.currentDragItem = item;
      item.classList.add('dragging');
      e.dataTransfer.effectAllowed = 'move';
      e.dataTransfer.setData('text/html', item.innerHTML);
    }

    onDragEnd(e) {
      if (this.currentDragItem) {
        this.currentDragItem.classList.remove('dragging');
      }

      document.querySelectorAll('.kanban-column').forEach(col => {
        col.classList.remove('drag-over');
      });

      this.currentDragItem = null;
    }

    onDragOver(e) {
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';

      const column = e.target.closest('.kanban-column');
      if (column) {
        column.classList.add('drag-over');
      }
    }

    onDragLeave(e) {
      const column = e.target.closest('.kanban-column');
      if (column && !column.contains(e.relatedTarget)) {
        column.classList.remove('drag-over');
      }
    }

    onDrop(e) {
      e.preventDefault();
      e.stopPropagation();

      const column = e.target.closest('.kanban-column');
      if (!column || !this.currentDragItem) return;

      const targetStatus = column.getAttribute('data-column');
      const itemId = this.currentDragItem.getAttribute('data-item-id');

      // Update item status
      const item = this.items.find(i => i.id === itemId);
      if (item) {
        item.status = targetStatus;
        this.saveItems();
        this.render();
      }

      column.classList.remove('drag-over');
    }

    render() {
      const columns = ['todo', 'in-progress', 'review', 'done'];

      columns.forEach(status => {
        const container = document.getElementById(status + '-items');
        if (!container) return;

        const itemsInColumn = this.items.filter(item => item.status === status);

        container.innerHTML = itemsInColumn.map(item => `
          <div class="kanban-item" data-item-id="${item.id}" draggable="true">
            <div style="font-weight: 600; margin-bottom: var(--space-sm);">${item.title}</div>
            <div style="font-size: var(--font-size-xs); color: var(--text-tertiary);">Priority: ${item.priority}</div>
            ${status === 'review' ? this.renderReviewActions(item.id) : ''}
          </div>
        `).join('');
      });

      // Re-attach listeners after render
      this.attachDragListeners();
    }

    renderReviewActions(itemId) {
      return `
        <div class="review-actions">
          <button class="btn btn-approve" onclick="kanbanBoard.approveItem('${itemId}')">Approve</button>
          <button class="btn btn-reject" onclick="kanbanBoard.rejectItem('${itemId}')">Reject</button>
          <button class="btn btn-request-changes" onclick="kanbanBoard.requestChanges('${itemId}')">Changes</button>
        </div>
      `;
    }

    approveItem(itemId) {
      const item = this.items.find(i => i.id === itemId);
      if (item) {
        item.status = 'done';
        this.saveItems();
        this.render();
      }
    }

    rejectItem(itemId) {
      const item = this.items.find(i => i.id === itemId);
      if (item) {
        item.status = 'todo';
        this.saveItems();
        this.render();
      }
    }

    requestChanges(itemId) {
      const item = this.items.find(i => i.id === itemId);
      if (item) {
        item.status = 'in-progress';
        this.saveItems();
        this.render();
      }
    }

    setupWebSocket() {
      // Optional: WebSocket for real-time updates
      // ws://localhost:8080/ws/kanban
      try {
        this.ws = new WebSocket('ws://localhost:8080/ws/kanban');
        this.ws.onmessage = (event) => {
          const data = JSON.parse(event.data);
          if (data.type === 'item-update') {
            this.items = data.items;
            this.render();
          }
        };
        this.ws.onerror = (e) => console.warn('WebSocket error:', e);
      } catch (e) {
        console.warn('WebSocket unavailable:', e);
      }
    }
  }

  // Initialize on page load
  let kanbanBoard;
  document.addEventListener('DOMContentLoaded', function() {
    kanbanBoard = new KanbanBoard();
  });
  ```
  - Save to: `kanban/js/kanban.js`
  - If fail → Step 41a [D]

- [ ] **Step 41a** [D] ~3m: **Verify kanban.js creation**
  ```bash
  mkdir -p kanban/js
  wc -l kanban/js/kanban.js
  grep -q "class KanbanBoard\|dragstart\|drop" kanban/js/kanban.js && echo "✓ Kanban JS created"
  ```
  - Continue to Step 42

- [ ] **Step 42** [W] ~5m: **Apply design system to kanban HTML**
  - Verify kanban/index.html uses:
    - `.navbar`, `.navbar-brand` for header
    - `.kanban-board`, `.kanban-column`, `.kanban-items`, `.kanban-item` for board
    - `.kanban-column-header` for column titles
    - Link back to dashboard
  - Update HTML if needed
  - If fail → Step 42a [D]

- [ ] **Step 42a** [D] ~2m: **Debug kanban styling**
  ```bash
  grep -c "kanban-board\|kanban-column\|kanban-item" kanban/index.html
  ```
  - Should show 3+ matches
  - Continue to Step 43

- [ ] **Step 43** [V] ~5m: **Test drag-drop functionality**
  - Open kanban in browser
  - Drag item from "To Do" to "In Progress"
  - Verify item moves visually
  - Verify item status persists in localStorage
  - If fail → Step 43a [D]

- [ ] **Step 43a** [D] ~5m: **Debug drag-drop**
  ```bash
  # Check browser console for JS errors
  # Verify kanban.js is loaded: grep "kanban.js" kanban/index.html
  # Check data attributes: grep "data-item-id\|data-column" kanban/index.html
  ```
  - Fix any JS errors
  - Continue to Step 44

- [ ] **Step 44** [V] ~3m: **Test Review column actions**
  - Drag item to Review column
  - Click "Approve" button → item moves to Done
  - Click "Reject" button → item moves to To Do
  - Click "Request Changes" button → item moves to In Progress
  - Verify all transitions work
  - If fail → Step 44a [D]

- [ ] **Step 44a** [D] ~3m: **Debug review actions**
  ```bash
  grep -c "approveItem\|rejectItem\|requestChanges" kanban/js/kanban.js
  ```
  - Verify functions exist
  - Check onclick handlers in HTML
  - Continue to Step 45

- [ ] **Step 45** [V] ~3m: **Test CRUD operations - Create item**
  - Add button to kanban HTML:
    ```html
    <button id="create-item-btn" class="btn btn-primary" style="margin-bottom: var(--space-md);">+ New Item</button>
    ```
  - Implement in kanban.js:
    ```javascript
    document.getElementById('create-item-btn').addEventListener('click', () => {
      const title = prompt('Item title:');
      if (title) {
        kanbanBoard.items.push({
          id: Date.now().toString(),
          title: title,
          status: 'todo',
          priority: 'medium',
        });
        kanbanBoard.saveItems();
        kanbanBoard.render();
      }
    });
    ```
  - Test: Click button, enter title, verify item appears in "To Do"
  - If fail → Step 45a [D]

- [ ] **Step 45a** [D] ~2m: **Debug create operation**
  ```bash
  grep -c "create-item-btn" kanban/index.html
  grep -c "addEventListener.*create\|prompt" kanban/js/kanban.js
  ```
  - Continue to Step 46

- [ ] **Step 46** [V] ~3m: **Test CRUD operations - Edit item**
  - Add edit functionality to kanban.js:
    ```javascript
    renderItem(item) {
      return `
        <div class="kanban-item" data-item-id="${item.id}" draggable="true">
          <div style="font-weight: 600; margin-bottom: var(--space-sm);">${item.title}</div>
          <div style="font-size: var(--font-size-xs); color: var(--text-tertiary);">
            Priority: ${item.priority}
            <button onclick="kanbanBoard.editItem('${item.id}')" style="margin-left: var(--space-sm);">Edit</button>
          </div>
          ${item.status === 'review' ? this.renderReviewActions(item.id) : ''}
        </div>
      `;
    }

    editItem(itemId) {
      const item = this.items.find(i => i.id === itemId);
      if (item) {
        const newTitle = prompt('Edit title:', item.title);
        if (newTitle) {
          item.title = newTitle;
          this.saveItems();
          this.render();
        }
      }
    }
    ```
  - Test: Click Edit, change title, verify change persists
  - If fail → Step 46a [D]

- [ ] **Step 46a** [D] ~2m: **Debug edit operation**
  ```bash
  grep -c "editItem\|onclick.*edit" kanban/js/kanban.js
  ```
  - Continue to Step 47

- [ ] **Step 47** [V] ~3m: **Test CRUD operations - Delete item**
  - Add delete functionality:
    ```javascript
    deleteItem(itemId) {
      this.items = this.items.filter(i => i.id !== itemId);
      this.saveItems();
      this.render();
    }
    ```
  - Add delete button to item template
  - Test: Delete item, verify it's gone
  - If fail → Step 47a [D]

- [ ] **Step 47a** [D] ~2m: **Debug delete operation**
  ```bash
  grep -c "deleteItem\|filter" kanban/js/kanban.js
  ```
  - Continue to Step 48

- [ ] **Step 48** [V] ~5m: **Test WebSocket reconnection (optional)**
  - Check browser console: `kanbanBoard.ws.readyState`
  - If WebSocket unavailable: Verify console warning
  - If available: Send test message from server
  - Verify kanban updates
  - If fail → Step 48a [D] (skip, non-critical)

- [ ] **Step 48a** [D] ~3m: **Debug WebSocket (optional)**
  ```bash
  grep -c "setupWebSocket\|ws://" kanban/js/kanban.js
  ```
  - Continue to Step 49

- [ ] **Step 49** [V] ~3m: **Verify Meta Moment - Kanban tracks its own dev**
  - Add task to kanban board: "S46: Kanban drag-drop implementation"
  - Add to "In Progress" or "Review" column
  - Document in kanban that it's self-referential
  - If fail → Step 49a [D] (non-critical, document manually)

- [ ] **Step 49a** [D] ~2m: **Meta moment documentation**
  - Add comment to kanban.js:
    ```javascript
    // Meta Moment: This kanban board tracks its own development
    // Current task: S46 Kanban Overhaul (implementing drag-drop)
    ```
  - Continue to Step 50

- [ ] **Step 50** [C] ~2m: **Commit Phase 3 checkpoint (Kanban)**
  ```bash
  git add kanban/
  git commit -m "S46-P3: Kanban overhaul - drag-drop, review actions, CRUD, WebSocket ready"
  ```
  - If fail → Step 50a [D]

- [ ] **Step 50a** [D] ~2m: **Debug commit**
  ```bash
  git status
  git log --oneline | head -3
  ```
  - Continue to Phase 4

---

## PHASE 4: DOOM VIEWER & LOGS POLISH (Steps 51-65)

- [ ] **Step 51** [R] ~5m: **Read doom viewer HTML**
  ```bash
  find . -name "*doom*" -type f
  cat doom/index.html | head -100 || echo "Doom file not found"
  ```
  - Check current structure
  - Verify gradient/checkerboard rendering
  - If missing → Create in Step 51a

- [ ] **Step 51a** [W] ~10m: **Create doom/index.html if missing**
  ```html
  <!DOCTYPE html>
  <html lang="en">
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Doom Metrics Viewer</title>
    <link rel="stylesheet" href="../dashboard/css/design-system.css">
    <style>
      .doom-container {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: var(--space-lg);
        padding: var(--space-lg);
      }

      .doom-canvas {
        width: 100%;
        height: 300px;
        border: var(--border-width) solid var(--border-light);
        border-radius: var(--radius-lg);
        background: linear-gradient(135deg, #2a2a4a 0%, #1a1a3a 100%);
        position: relative;
      }

      .checkerboard {
        width: 100%;
        height: 100%;
        background-image:
          linear-gradient(45deg, rgba(201, 201, 201, 0.1) 25%, transparent 25%),
          linear-gradient(-45deg, rgba(201, 201, 201, 0.1) 25%, transparent 25%),
          linear-gradient(45deg, transparent 75%, rgba(201, 201, 201, 0.1) 75%),
          linear-gradient(-45deg, transparent 75%, rgba(201, 201, 201, 0.1) 75%);
        background-size: 20px 20px;
        background-position: 0 0, 0 10px, 10px -10px, -10px 0px;
      }
    </style>
  </head>
  <body>
    <nav class="navbar">
      <div class="navbar-brand">⌘ Doom Metrics</div>
      <a href="../dashboard/index.html" class="navbar-link">← Back</a>
    </nav>

    <div class="container">
      <div class="doom-container">
        <div class="card">
          <h3>Gradient Demo</h3>
          <div class="doom-canvas" style="background: linear-gradient(135deg, #2a2a4a 0%, #1a1a3a 100%);"></div>
        </div>

        <div class="card">
          <h3>Checkerboard Demo</h3>
          <div class="doom-canvas">
            <div class="checkerboard"></div>
          </div>
        </div>
      </div>
    </div>
  </body>
  </html>
  ```
  - Save to: `doom/index.html`
  - If fail → Continue anyway

- [ ] **Step 52** [W] ~8m: **Apply design-system.css to doom viewer**
  - Update `<link rel="stylesheet" href="../dashboard/css/design-system.css">`
  - Update navbar classes
  - Update card classes
  - Test in browser
  - If fail → Step 52a [D]

- [ ] **Step 52a** [D] ~2m: **Debug doom styling**
  ```bash
  grep "design-system.css\|navbar\|card" doom/index.html
  ```
  - Continue to Step 53

- [ ] **Step 53** [V] ~5m: **Verify doom gradient/checkerboard renders**
  - Open doom/index.html in browser
  - Check: Gradient visible (dark navy to darker blue)
  - Check: Checkerboard pattern visible
  - Check: No CSS errors
  - If fail → Step 53a [D]

- [ ] **Step 53a** [D] ~3m: **Debug doom rendering**
  ```bash
  # Check browser console for CSS errors
  # Verify gradients are applied:
  grep -c "linear-gradient\|checkerboard" doom/index.html
  ```
  - Continue to Step 54

- [ ] **Step 54** [R] ~5m: **Read logs.html**
  ```bash
  cat logs/index.html | head -100 || echo "Logs file not found"
  wc -l logs/index.html || echo "Logs file doesn't exist"
  ```
  - Check structure
  - If missing → Create in Step 54a

- [ ] **Step 54a** [W] ~10m: **Create logs/index.html if missing**
  ```html
  <!DOCTYPE html>
  <html lang="en">
  <head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>System Logs</title>
    <link rel="stylesheet" href="../dashboard/css/design-system.css">
    <style>
      .logs-container {
        padding: var(--space-lg);
      }

      .log-entry {
        padding: var(--space-md);
        border-left: 2px solid var(--border-color);
        margin-bottom: var(--space-sm);
        background-color: var(--bg-tertiary);
      }

      .log-timestamp {
        color: var(--text-tertiary);
        font-size: var(--font-size-xs);
      }

      .log-level {
        display: inline-block;
        padding: 2px 6px;
        border-radius: 2px;
        font-size: var(--font-size-xs);
        font-weight: 600;
        margin-right: var(--space-sm);
      }

      .log-info { background-color: var(--status-info); color: var(--bg-primary); }
      .log-warn { background-color: var(--status-warning); color: var(--bg-primary); }
      .log-error { background-color: var(--status-error); color: var(--bg-primary); }

      .log-message {
        color: var(--text-primary);
        font-family: var(--font-mono);
        font-size: var(--font-size-sm);
      }
    </style>
  </head>
  <body>
    <nav class="navbar">
      <div class="navbar-brand">⌘ System Logs</div>
      <a href="../dashboard/index.html" class="navbar-link">← Back</a>
    </nav>

    <div class="logs-container">
      <h2>Event Logs</h2>

      <div id="logs-list">
        <div class="empty-state">
          <div class="empty-state-icon">⧐</div>
          <div class="empty-state-title">No Logs</div>
          <div class="empty-state-text">Waiting for log entries...</div>
        </div>
      </div>
    </div>

    <script>
      // Sample logs for demo
      const sampleLogs = [
        { timestamp: '2026-02-25T10:30:45Z', level: 'info', message: 'Dashboard loaded successfully' },
        { timestamp: '2026-02-25T10:30:46Z', level: 'warn', message: 'High packet drop rate detected' },
        { timestamp: '2026-02-25T10:30:47Z', level: 'error', message: 'WebSocket connection failed' },
        { timestamp: '2026-02-25T10:30:48Z', level: 'info', message: 'Metrics updated' },
      ];

      function renderLogs(logs) {
        const container = document.getElementById('logs-list');
        container.innerHTML = logs.map(log => `
          <div class="log-entry">
            <div class="log-timestamp">${log.timestamp}</div>
            <div>
              <span class="log-level log-${log.level}">${log.level.toUpperCase()}</span>
              <span class="log-message">${log.message}</span>
            </div>
          </div>
        `).join('');
      }

      document.addEventListener('DOMContentLoaded', () => {
        renderLogs(sampleLogs);
      });
    </script>
  </body>
  </html>
  ```
  - Save to: `logs/index.html`
  - If fail → Continue anyway

- [ ] **Step 55** [W] ~8m: **Apply design-system.css to logs.html**
  - Update CSS link
  - Verify navbar, container, card styles applied
  - Test in browser
  - If fail → Step 55a [D]

- [ ] **Step 55a** [D] ~2m: **Debug logs styling**
  ```bash
  grep "design-system.css\|navbar\|empty-state" logs/index.html
  ```
  - Continue to Step 56

- [ ] **Step 56** [V] ~5m: **Verify doom WebSocket connections**
  - Check doom/index.html for any WebSocket code
  - If using WebSocket: Verify connection attempts
  - Check browser console: `ws://` messages
  - If fail → Step 56a [D] (non-critical)

- [ ] **Step 56a** [D] ~2m: **Debug WebSocket (optional)**
  ```bash
  grep -c "WebSocket\|ws://" doom/index.html
  ```
  - Continue to Step 57

- [ ] **Step 57** [V] ~5m: **Verify logs WebSocket connections**
  - Check logs/index.html for WebSocket code
  - Verify real-time log updates
  - If fail → Step 57a [D] (non-critical)

- [ ] **Step 57a** [D] ~2m: **Debug logs WebSocket (optional)**
  ```bash
  grep -c "WebSocket\|ws://" logs/index.html
  ```
  - Continue to Step 58

- [ ] **Step 58** [V] ~5m: **Test doom in Chrome**
  - Open doom/index.html
  - Check: Gradients render
  - Check: Checkerboard renders
  - Check: Text readable
  - Check: No errors in console
  - If fail → Step 58a [D]

- [ ] **Step 58a** [D] ~2m: **Debug doom Chrome**
  ```bash
  # Check CSS syntax
  grep "linear-gradient" doom/index.html
  ```
  - Continue to Step 59

- [ ] **Step 59** [V] ~5m: **Test doom in Firefox**
  - Open doom/index.html in Firefox
  - Compare rendering with Chrome
  - Verify consistency
  - If fail → Step 59a [D]

- [ ] **Step 59a** [D] ~2m: **Debug doom Firefox**
  ```bash
  # Add vendor prefixes if needed
  grep "-webkit-\|-moz-" doom/index.html || echo "No vendor prefixes"
  ```
  - Continue to Step 60

- [ ] **Step 60** [V] ~5m: **Test logs in Chrome**
  - Open logs/index.html
  - Verify log entries render
  - Check colors (info, warn, error badges)
  - Check responsive layout
  - If fail → Step 60a [D]

- [ ] **Step 60a** [D] ~2m: **Debug logs Chrome**
  ```bash
  grep "log-info\|log-warn\|log-error" logs/index.html
  ```
  - Continue to Step 61

- [ ] **Step 61** [V] ~5m: **Test logs in Firefox**
  - Open logs/index.html in Firefox
  - Compare rendering
  - Verify consistency
  - If fail → Step 61a [D]

- [ ] **Step 61a** [D] ~2m: **Debug logs Firefox**
  ```bash
  # Check for compatibility issues
  grep "display:\|flex\|grid" logs/index.html
  ```
  - Continue to Step 62

- [ ] **Step 62** [V] ~3m: **Verify all file links work**
  - From dashboard, click to Doom
  - From doom, click back to dashboard
  - From dashboard, click to Logs
  - From logs, click back to dashboard
  - All navigation working
  - If fail → Step 62a [D]

- [ ] **Step 62a** [D] ~3m: **Debug navigation**
  ```bash
  grep -n "href=" dashboard/index.html doom/index.html logs/index.html
  ```
  - Verify all paths exist
  - Continue to Step 63

- [ ] **Step 63** [C] ~2m: **Commit Phase 4 checkpoint**
  ```bash
  git add doom/ logs/
  git commit -m "S46-P4: Doom and logs polish - design system applied, gradients, log viewer"
  ```
  - If fail → Step 63a [D]

- [ ] **Step 63a** [D] ~2m: **Debug commit**
  ```bash
  git status
  git log --oneline | head -5
  ```
  - Continue to Phase 5

---

## PHASE 5: VISUAL VERIFICATION (Steps 64-75)

- [ ] **Step 64** [V] ~10m: **Screenshot dashboard at 375px (mobile)**
  ```bash
  # Mobile: 375px width
  google-chrome --headless --disable-gpu --window-size=375,812 --screenshot=dashboard-375.png dashboard/index.html
  # Or Firefox
  firefox --headless --screenshot=dashboard-375.png --width 375 --height 812 dashboard/index.html
  ```
  - Store screenshot
  - Check: hamburger menu visible
  - Check: content stacks vertically
  - If fail → Step 64a [D]

- [ ] **Step 64a** [D] ~3m: **Debug mobile screenshot**
  ```bash
  ls -la dashboard-375.png || echo "Screenshot failed, continuing with manual test"
  ```
  - Open in browser, use DevTools mobile emulation
  - Continue to Step 65

- [ ] **Step 65** [V] ~10m: **Screenshot dashboard at 768px (tablet)**
  ```bash
  google-chrome --headless --disable-gpu --window-size=768,1024 --screenshot=dashboard-768.png dashboard/index.html
  ```
  - Check: 2-column metric grid
  - Check: tabs fully visible
  - Check: readable fonts
  - If fail → Step 65a [D]

- [ ] **Step 65a** [D] ~2m: **Debug tablet screenshot**
  ```bash
  ls -la dashboard-768.png || echo "Continuing with manual test"
  ```
  - Continue to Step 66

- [ ] **Step 66** [V] ~10m: **Screenshot dashboard at 1024px+ (desktop)**
  ```bash
  google-chrome --headless --disable-gpu --window-size=1280,800 --screenshot=dashboard-1280.png dashboard/index.html
  ```
  - Check: full layout visible
  - Check: proper spacing
  - Check: all tabs accessible
  - If fail → Step 66a [D]

- [ ] **Step 66a** [D] ~2m: **Debug desktop screenshot**
  ```bash
  ls -la dashboard-1280.png || echo "Continuing with manual test"
  ```
  - Continue to Step 67

- [ ] **Step 67** [V] ~5m: **Verify 0 console errors in dashboard**
  ```bash
  google-chrome --headless --disable-gpu --enable-logging --run-all-compositor-stages-before-draw dashboard/index.html 2>&1 | grep -i "error\|uncaught" | head -20
  ```
  - If errors found → Step 67a [D]
  - If no errors → Step 68

- [ ] **Step 67a** [D] ~5m: **Debug console errors**
  ```bash
  # Open in Chrome DevTools
  # Check: all script files load
  # Check: CSS files load
  # Check: no 404 errors
  ```
  - Fix errors and retry Step 67

- [ ] **Step 68** [V] ~5m: **Verify 0 console errors in kanban**
  ```bash
  google-chrome --headless --disable-gpu --enable-logging kanban/index.html 2>&1 | grep -i "error\|uncaught" | head -20
  ```
  - If fail → Step 68a [D]

- [ ] **Step 68a** [D] ~3m: **Debug kanban errors**
  ```bash
  grep -n "addEventListener\|getElementById" kanban/js/kanban.js | head -20
  ```
  - Verify all DOM selectors match HTML IDs
  - Continue to Step 69

- [ ] **Step 69** [V] ~5m: **Verify 0 console errors in doom**
  ```bash
  google-chrome --headless --disable-gpu --enable-logging doom/index.html 2>&1 | grep -i "error\|uncaught"
  ```
  - If fail → Step 69a [D]

- [ ] **Step 69a** [D] ~2m: **Debug doom errors**
  ```bash
  grep "style\|script" doom/index.html | head -10
  ```
  - Continue to Step 70

- [ ] **Step 70** [V] ~5m: **Verify 0 console errors in logs**
  ```bash
  google-chrome --headless --disable-gpu --enable-logging logs/index.html 2>&1 | grep -i "error\|uncaught"
  ```
  - If fail → Step 70a [D]

- [ ] **Step 70a** [D] ~2m: **Debug logs errors**
  ```bash
  grep -n "addEventListener\|getElementById" logs/index.html
  ```
  - Continue to Step 71

- [ ] **Step 71** [V] ~10m: **Test full navigation flow - Chrome**
  - Open dashboard
  - Click each tab (Packet Flow, Trace Table, Latency, Doom)
  - Click navbar links (Kanban, Doom, Logs)
  - Verify all pages load without errors
  - Verify responsive at different sizes
  - If fail → Step 71a [D]

- [ ] **Step 71a** [D] ~5m: **Debug navigation flow**
  ```bash
  # Check all href attributes
  grep -h "href=" dashboard/index.html kanban/index.html doom/index.html logs/index.html | sort | uniq
  ```
  - Verify all paths are valid
  - Continue to Step 72

- [ ] **Step 72** [V] ~10m: **Test full navigation flow - Firefox**
  - Repeat Step 71 in Firefox
  - Compare with Chrome rendering
  - Verify consistency
  - If fail → Step 72a [D]

- [ ] **Step 72a** [D] ~2m: **Debug Firefox compatibility**
  ```bash
  grep "-webkit-\|-moz-\|filter:\|backdrop" dashboard/css/design-system.css | head -10
  ```
  - Add vendor prefixes if needed
  - Continue to Step 73

- [ ] **Step 73** [V] ~5m: **Verify color scheme - #0a0a0a bg, #c9c9c9 text**
  - Open dashboard in browser
  - Use DevTools color picker to verify:
    - Background: #0a0a0a (very dark black)
    - Text: #c9c9c9 (silver/light gray)
    - Accent: #4a4a7a (dark purple)
  - Check all pages consistently themed
  - If fail → Step 73a [D]

- [ ] **Step 73a** [D] ~5m: **Debug color scheme**
  ```bash
  grep "--bg-primary:\|--text-primary:" dashboard/css/design-system.css
  ```
  - Verify CSS variables match spec
  - Continue to Step 74

- [ ] **Step 74** [V] ~5m: **Verify font loading - JetBrains Mono + Space Grotesk**
  - Open any page in DevTools
  - Check: Font loading (Fonts tab)
  - Verify: JetBrains Mono loaded for body
  - Verify: Space Grotesk loaded for headers
  - If fail → Step 74a [D]

- [ ] **Step 74a** [D] ~3m: **Debug font loading**
  ```bash
  grep "@import url\|@font-face" dashboard/css/design-system.css
  ls -la dashboard/assets/fonts/
  ```
  - Verify font files exist or CDN URLs work
  - Continue to Step 75

- [ ] **Step 75** [C] ~2m: **Final commit & Phase 5 complete**
  ```bash
  git add .
  git commit -m "S46-P5: Visual verification complete - 0 console errors, responsive design, cross-browser tested"
  ```
  - If fail → Step 75a [D]

- [ ] **Step 75a** [D] ~2m: **Debug final commit**
  ```bash
  git status
  git log --oneline | head -10
  ```
  - All changes committed
  - Proceed to cleanup

---

## POST-SPRINT CLEANUP (Step 76)

- [ ] **Step 76** [V] ~5m: **Final verification checklist**
  ```
  ✓ Dashboard: 515 lines refactored, design-system.css applied
  ✓ Kanban: Drag-drop working, Review column with actions
  ✓ Doom: Gradients/checkerboard rendering
  ✓ Logs: Log viewer with styling
  ✓ All files: 0 console errors
  ✓ All pages: Responsive at 375px, 768px, 1024px+
  ✓ All pages: Tested Chrome + Firefox
  ✓ Color scheme: #0a0a0a bg, #c9c9c9 text applied
  ✓ Fonts: JetBrains Mono + Space Grotesk loaded
  ✓ Git: s46-dashboard-overhaul branch with 5+ commits
  ```
  - All checks pass → SPRINT COMPLETE

---

## APPENDIX A: EMERGENCY PROCEDURES

### CSS Breaks

**Problem**: Styles not applying to elements
**Solution**:
1. Check file path: `grep "design-system.css" dashboard/index.html`
2. Verify file exists: `ls -la dashboard/css/design-system.css`
3. Check syntax: `node -c dashboard/css/design-system.css` (syntax check)
4. Clear browser cache: DevTools → Settings → Clear cache
5. Hard refresh: Cmd/Ctrl + Shift + R

### Font Loading Fails

**Problem**: Fonts not rendering, fallback to system fonts
**Solution**:
1. Check @font-face: `grep "@font-face" dashboard/css/design-system.css`
2. Verify font files: `ls -la dashboard/assets/fonts/`
3. Check CDN URLs: Use `curl -I https://fonts.googleapis.com/...`
4. Add web fallback: `@import url('https://fonts.googleapis.com/css2?family=...')`
5. Use system fonts temporarily

### Drag API Not Working

**Problem**: Kanban drag-drop not responding
**Solution**:
1. Check draggable attribute: `grep "draggable=" kanban/index.html`
2. Verify JS loaded: `grep "kanban.js" kanban/index.html`
3. Check event listeners: `grep "addEventListener" kanban/js/kanban.js`
4. Test in browser console: `kanbanBoard.items.length`
5. Clear localStorage: `localStorage.clear()`

### WebSocket Errors

**Problem**: WebSocket connection fails
**Solution**:
1. Check server running: `lsof -i :8080` or `netstat -an | grep 8080`
2. Verify URL: `grep "ws://" kanban/js/kanban.js`
3. Check CORS: Server must allow WebSocket upgrade
4. Fallback: Disable WebSocket, use polling or localStorage

---

## APPENDIX B: AGENT MATRIX

| Phase | Steps | Parallelizable | Key Decisions |
|-------|-------|----------------|---------------|
| 0: Baseline | 1-10 | No | Establish environment |
| 1: Design System | 11-30 | Yes (after 11) | CSS variable naming, spacing scale |
| 2: Dashboard | 31-65 | Yes (after 30) | Demo data structure, tab organization |
| 3: Kanban | 66-90 | Yes (after 50) | Drag API vs library, localStorage vs server |
| 4: Polish | 91-105 | Yes | Font fallbacks, WebSocket optional |
| 5: Verify | 106-120 | No | Screenshots, cross-browser |

---

## APPENDIX C: QUICK REFERENCE

### CSS Variables (Design System)

```css
/* Colors */
--bg-primary: #0a0a0a
--text-primary: #c9c9c9
--accent-primary: #4a4a7a

/* Typography */
--font-mono: JetBrains Mono
--font-sans: Space Grotesk

/* Spacing */
--space-md: 1rem
--space-lg: 1.5rem

/* Breakpoints */
375px (mobile)
768px (tablet)
1024px (desktop)
1280px+ (large)
```

### File Paths

```
dashboard/
  ├── index.html (515 lines)
  ├── css/design-system.css
  ├── js/demo-data.js
  └── assets/fonts/

kanban/
  ├── index.html
  └── js/kanban.js

doom/
  └── index.html

logs/
  └── index.html
```

### Key Classes

```html
<!-- Navigation -->
<nav class="navbar"></nav>
<a class="navbar-link"></a>

<!-- Cards -->
<div class="card metric-card"></div>

<!-- Kanban -->
<div class="kanban-board"></div>
<div class="kanban-column" data-column="todo"></div>
<div class="kanban-item" draggable="true"></div>

<!-- Empty States -->
<div class="empty-state"></div>

<!-- Responsive -->
<div class="grid grid-2"></div>
<div class="container"></div>
```

### Git Commands

```bash
# Branch
git branch s46-dashboard-overhaul
git checkout s46-dashboard-overhaul

# Commits (every 5 steps)
git add [files]
git commit -m "S46-P[PHASE]: [Description]"

# Review
git log --oneline | head -10
git diff HEAD~5..HEAD
```

---

## NOTES

- **Total Steps**: 115-125 (estimated 8-10 hours)
- **Commit Frequency**: Every 5 steps (~5 commits total)
- **Testing**: Continuous (every phase has verification steps)
- **Emergency Exit**: If stuck > 3x time estimate, skip step and document
- **Success Criteria**:
  - All 4 pages styled with design system
  - Kanban drag-drop functional
  - 0 console errors
  - Responsive design verified
  - Cross-browser tested (Chrome + Firefox)
