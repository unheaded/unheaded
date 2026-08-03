// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis.
//
// eslint flat config — deliberately minimal.
//
// The tree carries ~21K lines of JS across 40 files, all of it browser code for
// the dashboard and kanban front ends, and none of it had ever been linted:
// there was no eslint config and no package.json anywhere in the repo before
// 2026-08-03.
//
// This config uses `eslint:recommended` and nothing else. No stylistic plugins,
// no framework presets, no import resolver. The reason is that CLAUDE.md picked
// vanilla JS specifically to avoid a front-end toolchain ("Vanilla JS — no
// framework overhead, full control"), and pulling a few hundred dev
// dependencies in to lint it would invert that trade. `recommended` catches the
// class that actually matters here — undeclared variables, unreachable code,
// duplicate object keys, empty blocks — without acquiring an opinion about
// semicolons.
//
// Environment: browser globals, `script` source type. Verified before choosing:
// no file in the tree uses `import`/`export`, and none uses `require`/
// `module.exports`. These are <script>-tag files, not modules.
//
// Baseline when this landed is recorded in .github/workflows/static-analysis.yml
// beside the job, the same way the shellcheck and ruff baselines are.

export default [
    {
        files: ["**/*.js"],
        ignores: [
            "**/node_modules/**",
            "llama.cpp/**",
            // NOTE the leading **/: cargo writes a target/ inside every crate,
            // not just at the repo root, and crates/upc-api/target/doc holds
            // generated rustdoc JS. A bare "target/**" matches only the root
            // one and let ~250 findings from generated, untracked files into
            // the first measurement of this surface.
            "**/target/**",
            "**/vendor/**",
            // This config is itself ESM; the linted tree is <script>-tag code.
            "eslint.config.js",
        ],
        languageOptions: {
            ecmaVersion: 2022,
            sourceType: "script",
            globals: {
                // Browser runtime surface actually used by these files.
                window: "readonly",
                document: "readonly",
                console: "readonly",
                fetch: "readonly",
                setTimeout: "readonly",
                clearTimeout: "readonly",
                setInterval: "readonly",
                clearInterval: "readonly",
                requestAnimationFrame: "readonly",
                cancelAnimationFrame: "readonly",
                localStorage: "readonly",
                sessionStorage: "readonly",
                location: "readonly",
                navigator: "readonly",
                history: "readonly",
                alert: "readonly",
                confirm: "readonly",
                prompt: "readonly",
                WebSocket: "readonly",
                EventSource: "readonly",
                URL: "readonly",
                URLSearchParams: "readonly",
                Event: "readonly",
                CustomEvent: "readonly",
                FormData: "readonly",
                Blob: "readonly",
                Image: "readonly",
                performance: "readonly",
                crypto: "readonly",
                getComputedStyle: "readonly",
                MutationObserver: "readonly",
                IntersectionObserver: "readonly",
                ResizeObserver: "readonly",
                AbortController: "readonly",
                TextDecoder: "readonly",
                TextEncoder: "readonly",
                atob: "readonly",
                btoa: "readonly",
                HTMLElement: "readonly",
                HTMLInputElement: "readonly",
                HTMLAnchorElement: "readonly",
                HTMLLinkElement: "readonly",

                // Cross-file globals. These pages load several <script> tags that
                // share one global scope, so a symbol defined in api.js is used in
                // app.js. eslint lints one file at a time and cannot see that, so
                // each has to be declared here or it reads as 96 undefined-variable
                // errors that are not bugs. Each name below was verified to have
                // exactly one defining file.
                API: "readonly",              // cmd/kanban-app/static/js/api.js
                App: "readonly",              // cmd/kanban-app/static/js/app.js
                Board: "readonly",            // cmd/kanban-app/static/js/board.js
                Cards: "readonly",            // cmd/kanban-app/static/js/cards.js
                WebSocketClient: "readonly",  // cmd/kanban-app/static/js/websocket.js
                //
                // Declaring these costs 5 no-redeclare findings — one per
                // defining file, which now declares a name the config also
                // declares. That is left in the baseline deliberately rather
                // than suppressed: it is accurate, and it is the signal that
                // would tell you if one of these ever gained a second definer.
            },
        },
        rules: {
            // eslint:recommended, expressed explicitly because flat config has
            // no `extends`. Kept in one place so the ratchet has something to
            // point at when a rule is excluded by ID.
            "constructor-super": "error",
            "for-direction": "error",
            "getter-return": "error",
            "no-async-promise-executor": "error",
            "no-case-declarations": "error",
            "no-class-assign": "error",
            "no-compare-neg-zero": "error",
            "no-cond-assign": "error",
            "no-const-assign": "error",
            "no-constant-binary-expression": "error",
            "no-constant-condition": "error",
            "no-control-regex": "error",
            "no-debugger": "error",
            "no-delete-var": "error",
            "no-dupe-args": "error",
            "no-dupe-class-members": "error",
            "no-dupe-else-if": "error",
            "no-dupe-keys": "error",
            "no-duplicate-case": "error",
            "no-empty": "error",
            "no-empty-character-class": "error",
            "no-empty-pattern": "error",
            "no-ex-assign": "error",
            "no-extra-boolean-cast": "error",
            "no-fallthrough": "error",
            "no-func-assign": "error",
            "no-global-assign": "error",
            "no-import-assign": "error",
            "no-invalid-regexp": "error",
            "no-irregular-whitespace": "error",
            "no-loss-of-precision": "error",
            "no-misleading-character-class": "error",
            "no-new-native-nonconstructor": "error",
            "no-obj-calls": "error",
            "no-octal": "error",
            "no-prototype-builtins": "error",
            "no-redeclare": "error",
            "no-regex-spaces": "error",
            "no-self-assign": "error",
            "no-setter-return": "error",
            "no-shadow-restricted-names": "error",
            "no-sparse-arrays": "error",
            "no-this-before-super": "error",
            "no-undef": "error",
            "no-unexpected-multiline": "error",
            "no-unreachable": "error",
            "no-unsafe-finally": "error",
            "no-unsafe-negation": "error",
            "no-unsafe-optional-chaining": "error",
            "no-unused-labels": "error",
            "no-unused-private-class-members": "error",
            "no-unused-vars": "error",
            "no-useless-backreference": "error",
            "no-useless-catch": "error",
            "no-useless-escape": "error",
            "no-with": "error",
            "require-yield": "error",
            "use-isnan": "error",
            "valid-typeof": "error",
        },
    },
];
