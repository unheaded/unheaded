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

// Package main provides the kanban-app server
// Serves the Kanban dashboard and proxies to TimeGuru API
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog"

	"unheaded/pkg/auth"
	"unheaded/pkg/database"
	"unheaded/pkg/discovery"
	"unheaded/pkg/logagg"
	"unheaded/pkg/logger"
	"unheaded/pkg/transport"
	wotanClient "unheaded/pkg/wotan-client"
)

// Package-level logger instance
var log = logger.New(os.Stderr)

//go:embed static/*
var staticFiles embed.FS

// Config holds server configuration
type Config struct {
	Port            string
	TimeGuruAddr    string
	WotanAddr      string
	DataDir         string        // Directory for SQLite persistence (default: ./data)
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Task represents a kanban task
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Status      string    `json:"status"`
	Type        string    `json:"type,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	Progress    int       `json:"progress,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Server holds the HTTP server and dependencies
type Server struct {
	config          Config
	httpServer      *http.Server
	store           TaskStore        // SQLite or Postgres persistence
	tasks           []Task           // DEPRECATED: Use store or taskManager instead
	tasksMu         sync.RWMutex
	sseClients      map[chan []byte]bool
	sseMu           sync.RWMutex
	taskManager     *TaskManager     // Wotan-integrated task management
	timelineManager *TimelineManager // Standalone Timeguru HTTP polling fallback
	healthSrv       *transport.HealthServer // Unified transport health
	ctx             context.Context    // server lifecycle context
	cancel          context.CancelFunc // cancels ctx on shutdown
}

// NewServer creates a new kanban server with standalone timeline polling.
// Initializes task store: prefers Postgres, falls back to SQLite, then in-memory.
func NewServer(cfg Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		config:     cfg,
		sseClients: make(map[chan []byte]bool),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Try Postgres first
	pgDB, pgErr := database.Connect(context.Background(), database.AppKanbanConfig())
	if pgErr == nil {
		pgStore, pgStoreErr := NewPgStore(pgDB)
		if pgStoreErr == nil {
			s.store = pgStore
			if _, err := pgStore.SeedIfEmpty(); err != nil {
				log.Warn().Err(err).Msg("Postgres seed failed — falling back to SQLite")
				pgStore.Close()
				s.store = nil
			} else {
				log.Info().Msg("Postgres connected — kanban tasks persisted to The Well (standalone mode)")
			}
		} else {
			log.Warn().Err(pgStoreErr).Msg("Postgres schema init failed — falling back to SQLite")
			pgDB.Close()
		}
	} else {
		log.Warn().Err(pgErr).Msg("Postgres unavailable — trying SQLite")
	}

	// SQLite fallback
	if s.store == nil {
		dbPath := cfg.DataDir + "/kanban.db"
		if cfg.DataDir == "" {
			dbPath = "./data/kanban.db"
		}
		store, err := NewStore(dbPath)
		if err != nil {
			log.Error().Err(err).Str("path", dbPath).Msg("failed to initialize SQLite store — falling back to in-memory")
			s.tasks = getInitialTasks()
		} else {
			s.store = store
			// Seed on first run (Genesis Event), no-op on subsequent starts
			if _, err := store.SeedIfEmpty(); err != nil {
				log.Error().Err(err).Msg("failed to seed store — falling back to in-memory")
				s.store = nil
				store.Close()
				s.tasks = getInitialTasks()
			}
		}
	}

	// Create standalone TimelineManager for direct Timeguru HTTP polling
	s.timelineManager = NewTimelineManager(func(eventType string, data interface{}) {
		s.broadcastUpdate(eventType, data)
	})
	return s
}

// NewServerWithTaskManager creates a server with Wotan integration and shared Store.
func NewServerWithTaskManager(cfg Config, tm *TaskManager, store TaskStore) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{
		config:      cfg,
		store:       store,
		sseClients:  make(map[chan []byte]bool),
		taskManager: tm,
		ctx:         ctx,
		cancel:      cancel,
	}
	return s
}

// getInitialTasks returns the full Unheaded Kingdom inventory.
// Every armor piece, gnostic service, active task, vision item, and wishlist entry.
// This is the COMPLETE view of the project — past, present, and future.
func getInitialTasks() []Task {
	now := time.Now()
	d := 24 * time.Hour // shorthand

	return []Task{
		// =====================================================================
		// MILESTONES — high-level phase gates
		// =====================================================================
		{ID: "ms-phase0", Title: "Age 0: The Foundation Stone", Description: "Wotan message bus proves pub/sub patterns. 13,504 LOC shipped.", Status: "done", Type: "milestone", Owner: "Team", Progress: 100, CreatedAt: now.Add(-60 * d), UpdatedAt: now.Add(-45 * d)},
		{ID: "ms-phase1", Title: "Age 1: The Alpha Ascension", Description: "Full infrastructure armor forged. 237K+ LOC. Target: Feb 2026.", Status: "in-progress", Type: "milestone", Owner: "Team", Progress: 96, CreatedAt: now.Add(-45 * d), UpdatedAt: now},
		{ID: "ms-phase2", Title: "Age 2: The Beta Trials", Description: "Production hardening, multi-tenant isolation, performance tuning.", Status: "todo", Type: "milestone", Owner: "Team", Progress: 0, CreatedAt: now.Add(-30 * d), UpdatedAt: now},
		{ID: "ms-phase3", Title: "Age 3: The MVP Era", Description: "Full compliance templates, self-healing infra, multi-cloud.", Status: "todo", Type: "milestone", Owner: "Team", Progress: 0, CreatedAt: now.Add(-30 * d), UpdatedAt: now},

		// =====================================================================
		// ARMOR PIECES — infrastructure components (The Knight's Armor)
		// =====================================================================
		{ID: "armor-shield", Title: "Shield (WAF)", Description: "Web Application Firewall. 6,057 LOC. Security verified.", Status: "done", Type: "infra", Owner: "Architect", Progress: 95, CreatedAt: now.Add(-40 * d), UpdatedAt: now.Add(-5 * d)},
		{ID: "armor-hauberk", Title: "Hauberk (Service Mesh)", Description: "Full service discovery, circuit breakers, load-aware routing. 5,914 LOC.", Status: "in-progress", Type: "infra", Owner: "Architect", Progress: 90, CreatedAt: now.Add(-38 * d), UpdatedAt: now},
		{ID: "armor-pauldrons", Title: "Pauldrons (Load Balancer)", Description: "L4/L7 load balancing, Maglev consistent hashing, session persistence. 6,719 LOC.", Status: "in-progress", Type: "infra", Owner: "Architect", Progress: 90, CreatedAt: now.Add(-36 * d), UpdatedAt: now},
		{ID: "armor-sword", Title: "Sword (Deploy Pipeline)", Description: "Canary, blue-green, rolling deploys. 7,746 LOC.", Status: "in-progress", Type: "infra", Owner: "Developer", Progress: 85, CreatedAt: now.Add(-35 * d), UpdatedAt: now},
		{ID: "armor-helm", Title: "Helm (DNS Resolver)", Description: "Full DNS-SD, DNSSEC, DoH/DoT. 4,462 LOC.", Status: "in-progress", Type: "infra", Owner: "Architect", Progress: 85, CreatedAt: now.Add(-34 * d), UpdatedAt: now},
		{ID: "armor-greaves", Title: "Greaves (Scheduler)", Description: "Bin-pack, affinity rules, preemption. 5,496 LOC.", Status: "in-progress", Type: "infra", Owner: "Architect", Progress: 85, CreatedAt: now.Add(-33 * d), UpdatedAt: now},
		{ID: "armor-cuirass", Title: "Cuirass (Control Plane)", Description: "Daemon + state management, API gateway.", Status: "in-progress", Type: "infra", Owner: "Architect", Progress: 75, CreatedAt: now.Add(-32 * d), UpdatedAt: now},
		{ID: "armor-gauntlets", Title: "Gauntlets (Container Runtime)", Description: "OCI-compliant, cgroups v2, namespace isolation. 6,955 LOC.", Status: "in-progress", Type: "infra", Owner: "Architect", Progress: 75, CreatedAt: now.Add(-31 * d), UpdatedAt: now},
		{ID: "armor-visor", Title: "Visor (Dashboard Backend)", Description: "WebSocket-ready metrics + event aggregation. 5,926 LOC.", Status: "in-progress", Type: "infra", Owner: "Developer", Progress: 70, CreatedAt: now.Add(-20 * d), UpdatedAt: now},
		{ID: "armor-void", Title: "Whispering Void (eBPF)", Description: "Packet marking, flow tracing, latency probes. 7,196 LOC. Awaiting Linux env.", Status: "in-progress", Type: "infra", Owner: "Architect", Progress: 55, CreatedAt: now.Add(-28 * d), UpdatedAt: now},
		{ID: "armor-crest", Title: "Crest (Kanban Frontend)", Description: "Real-time Kanban board with SSE + Wotan integration.", Status: "in-progress", Type: "infra", Owner: "Developer", Progress: 95, CreatedAt: now.Add(-10 * d), UpdatedAt: now},

		// =====================================================================
		// GNOSTIC SERVICES — state & wisdom layer
		// =====================================================================
		{ID: "gnostic-monad", Title: "Monad (The One)", Description: "Functional composition engine. ~500 LOC.", Status: "done", Type: "feature", Owner: "Architect", Progress: 100, CreatedAt: now.Add(-15 * d), UpdatedAt: now.Add(-10 * d)},
		{ID: "gnostic-sophia", Title: "Sophia (Wisdom)", Description: "Knowledge/wisdom management service. ~700 LOC.", Status: "done", Type: "feature", Owner: "Architect", Progress: 100, CreatedAt: now.Add(-15 * d), UpdatedAt: now.Add(-10 * d)},
		{ID: "gnostic-pleroma", Title: "Pleroma (Desired State)", Description: "Declarative desired state store. The fullness.", Status: "in-progress", Type: "feature", Owner: "Architect", Progress: 70, CreatedAt: now.Add(-25 * d), UpdatedAt: now},
		{ID: "gnostic-kenoma", Title: "Kenoma (Actual State)", Description: "Runtime actual state observer. The void/deficiency.", Status: "in-progress", Type: "feature", Owner: "Architect", Progress: 70, CreatedAt: now.Add(-25 * d), UpdatedAt: now},
		{ID: "gnostic-anamnesis", Title: "Anamnesis (Event History)", Description: "Immutable event log. Remembrance of all state transitions.", Status: "in-progress", Type: "feature", Owner: "Developer", Progress: 60, CreatedAt: now.Add(-22 * d), UpdatedAt: now},
		{ID: "gnostic-yaldabaoth", Title: "Yaldabaoth (Chaos Engineering)", Description: "Controlled chaos injection. The demiurge tests resilience.", Status: "in-progress", Type: "feature", Owner: "Architect", Progress: 40, CreatedAt: now.Add(-20 * d), UpdatedAt: now},

		// =====================================================================
		// SUPPORT SERVICES
		// =====================================================================
		{ID: "svc-wotan", Title: "Wotan (Message Bus)", Description: "Pub/sub message bus. HTTP+gRPC dual transport. 13,504 LOC.", Status: "done", Type: "feature", Owner: "Developer", Progress: 100, CreatedAt: now.Add(-50 * d), UpdatedAt: now.Add(-2 * d)},
		{ID: "svc-timeguru", Title: "Timeguru (Timeline Seer)", Description: "Project timeline tracking, sync to JSON/YAML/TOML/MD.", Status: "in-progress", Type: "feature", Owner: "Developer", Progress: 80, CreatedAt: now.Add(-18 * d), UpdatedAt: now},
		{ID: "svc-gateway", Title: "Gateway (API Gateway)", Description: "Unified API ingress, auth, rate limiting.", Status: "in-progress", Type: "feature", Owner: "Architect", Progress: 60, CreatedAt: now.Add(-16 * d), UpdatedAt: now},
		{ID: "svc-cape", Title: "Cape (Auth/Identity)", Description: "Authentication, authorization, identity management.", Status: "in-progress", Type: "feature", Owner: "Architect", Progress: 50, CreatedAt: now.Add(-14 * d), UpdatedAt: now},
		{ID: "svc-cloak", Title: "Cloak (Secrets Manager)", Description: "SOPS + age secrets management, rotation, audit.", Status: "in-progress", Type: "feature", Owner: "Architect", Progress: 45, CreatedAt: now.Add(-12 * d), UpdatedAt: now},
		{ID: "svc-sabatons", Title: "Sabatons (Boot/Init)", Description: "System bootstrap, service startup orchestration.", Status: "in-progress", Type: "feature", Owner: "Architect", Progress: 40, CreatedAt: now.Add(-10 * d), UpdatedAt: now},
		{ID: "svc-tassets", Title: "Tassets (Storage Layer)", Description: "Persistent storage abstraction, volume management.", Status: "in-progress", Type: "feature", Owner: "Architect", Progress: 35, CreatedAt: now.Add(-10 * d), UpdatedAt: now},
		{ID: "svc-vambraces", Title: "Vambraces (Logging/Telemetry)", Description: "Vector -> ClickHouse pipeline, structured logging.", Status: "in-progress", Type: "feature", Owner: "Developer", Progress: 50, CreatedAt: now.Add(-8 * d), UpdatedAt: now},

		// =====================================================================
		// ACTIVE TASKS — work in progress
		// =====================================================================
		{ID: "task-transport-bench", Title: "Transport Benchmarks (HTTP vs gRPC)", Description: "Validated: gRPC 6.2x faster on PollCycle, 10x fewer allocs.", Status: "done", Type: "task", Owner: "Developer", Progress: 100, CreatedAt: now.Add(-2 * d), UpdatedAt: now},
		{ID: "task-drainbody", Title: "HTTP drainBody() Fix", Description: "json.Decoder left trailing bytes -> socket leak. Production bug caught by benchmarks.", Status: "done", Type: "bug", Owner: "Developer", Progress: 100, CreatedAt: now.Add(-1 * d), UpdatedAt: now},
		{ID: "task-split-brain", Title: "Kanban Split-Brain Fix", Description: "pollTimeguru disabled when Wotan active. SSE prefers kanban cards.", Status: "done", Type: "bug", Owner: "Developer", Progress: 100, CreatedAt: now.Add(-1 * d), UpdatedAt: now},
		{ID: "task-dual-transport", Title: "gRPC Primary / HTTP Fallback", Description: "TransportState machine: probe at startup, circuit-breaker degradation.", Status: "done", Type: "feature", Owner: "Developer", Progress: 100, CreatedAt: now.Add(-2 * d), UpdatedAt: now},
		{ID: "task-wotan-tests", Title: "Wotan Unit Test Suite", Description: "Comprehensive unit tests for core message bus components.", Status: "todo", Type: "task", Owner: "Developer", Progress: 0, CreatedAt: now.Add(-7 * d), UpdatedAt: now},
		{ID: "task-skill-xrefs", Title: "Skill Cross-References", Description: "Update all skills with bidirectional cross-references.", Status: "todo", Type: "task", Owner: "Captain", Progress: 0, CreatedAt: now.Add(-3 * d), UpdatedAt: now},
		{ID: "task-cicd", Title: "CI/CD Pipeline Templates", Description: "GitHub Actions for build, test, deploy, release.", Status: "todo", Type: "task", Owner: "Developer", Progress: 0, CreatedAt: now.Add(-3 * d), UpdatedAt: now},
		{ID: "task-e2e-integration", Title: "Full E2E Integration Tests", Description: "All 23 services wired, health checks green, message flow verified.", Status: "todo", Type: "task", Owner: "Developer", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "task-linux-env", Title: "Linux Dev Environment", Description: "eBPF requires Linux kernel. Set up NixOS VM or bare metal.", Status: "todo", Type: "task", Owner: "Muck", Progress: 0, CreatedAt: now.Add(-14 * d), UpdatedAt: now},

		// =====================================================================
		// TECH DEBT
		// =====================================================================
		{ID: "debt-docker-compose", Title: "Fix Docker Compose Ports", Description: "docker-compose.yml has wotan on 8081/5555, should match standalone defaults 8080/9090.", Status: "todo", Type: "tech-debt", Owner: "Developer", Progress: 0, CreatedAt: now.Add(-1 * d), UpdatedAt: now},
		{ID: "debt-timeguru-db", Title: "Timeguru Local Dev Defaults", Description: "DB_PATH defaults to /opt/unheaded/data/ which doesn't exist on macOS.", Status: "todo", Type: "tech-debt", Owner: "Developer", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "debt-timeline-sync", Title: "Timeline Data Hydration", Description: "timeline.json milestones array is empty. Sync overwrites timeline.md to stubs.", Status: "todo", Type: "tech-debt", Owner: "Developer", Progress: 0, CreatedAt: now, UpdatedAt: now},

		// =====================================================================
		// VISION — high-level north star items
		// =====================================================================
		{ID: "vision-4hr-deploy", Title: "4-Hour Production Deploy", Description: "Customer provides app, Unheaded delivers full prod infra in ~4 hours.", Status: "todo", Type: "vision", Owner: "Captain", Progress: 0, CreatedAt: now.Add(-60 * d), UpdatedAt: now},
		{ID: "vision-compliance", Title: "Drop-In Compliance Templates", Description: "FEDRAMP, NIST, SOC2, PCI-DSS, HIPAA, ITAR, GDPR — overlapping controls visualization.", Status: "todo", Type: "vision", Owner: "Captain", Progress: 0, CreatedAt: now.Add(-60 * d), UpdatedAt: now},
		{ID: "vision-ebpf-dash", Title: "eBPF Custom Dashboards", Description: "Packet-level observability with in-house dashboards. Trace any flow, graph it, log it.", Status: "todo", Type: "vision", Owner: "Architect", Progress: 0, CreatedAt: now.Add(-60 * d), UpdatedAt: now},
		{ID: "vision-idp-siem", Title: "Full IDP/SIEM/SOC/NOC Stack", Description: "Complete security operations as drop-in infrastructure components.", Status: "todo", Type: "vision", Owner: "Architect", Progress: 0, CreatedAt: now.Add(-60 * d), UpdatedAt: now},
		{ID: "vision-zero-trust", Title: "Zero Trust Everywhere", Description: "mTLS everywhere, short-lived certs, zero trust architecture end to end.", Status: "todo", Type: "vision", Owner: "Architect", Progress: 0, CreatedAt: now.Add(-60 * d), UpdatedAt: now},

		// =====================================================================
		// WISHLIST — future exploration items
		// =====================================================================
		{ID: "wish-multi-cloud", Title: "Multi-Cloud Orchestration", Description: "Deploy across major cloud providers and bare metal from single config.", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now.Add(-30 * d), UpdatedAt: now},
		{ID: "wish-self-healing", Title: "Self-Healing Infrastructure", Description: "Yaldabaoth chaos + Pleroma/Kenoma reconciliation = auto-recovery.", Status: "todo", Type: "wishlist", Owner: "Architect", Progress: 0, CreatedAt: now.Add(-30 * d), UpdatedAt: now},
		{ID: "wish-bazaar", Title: "Unheaded Bazaar", Description: "Community-contributed armor pieces, compliance templates, eBPF probes. The Bazaar where the kingdom trades.", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now.Add(-30 * d), UpdatedAt: now},
		{ID: "wish-nixos-fleet", Title: "NixOS Container Fleet", Description: "Full L2-L7 network stack inside NixOS LXD containers on Debian bare metal.", Status: "todo", Type: "wishlist", Owner: "Architect", Progress: 0, CreatedAt: now.Add(-30 * d), UpdatedAt: now},
		{ID: "wish-gpu-sched", Title: "GPU-Aware Scheduling", Description: "Greaves scheduler with GPU affinity for ML/AI workloads.", Status: "todo", Type: "wishlist", Owner: "Architect", Progress: 0, CreatedAt: now.Add(-30 * d), UpdatedAt: now},
		{ID: "wish-norse-naming", Title: "Norse/Ragnarok Religious & Mythological Service Names", Description: "Alternate or additional naming: Yggdrasil (network fabric), Loki (chaos eng), Heimdall (gateway/WAF), Odin (control plane), Bifrost (service mesh), Valkyrie (scheduler), Mjolnir (deploy), Fenrir (container runtime). Ragnarok Online and/or Norse vibes.", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-hindu-naming", Title: "Hindu/Indian/Yoga Religious & Mythological Service Names", Description: "Alternate or additional naming: Indra (load balancer/thunder), Agni (firewall/WAF), Vishnu (state reconciler/preserver), Shiva (chaos eng/destroyer), Brahma (provisioner/creator), Maya (container runtime/illusion), Dharma (compliance/law), Karma (event sourcing/cause-effect), Chakra (service mesh/wheel), Garuda (scheduler/eagle mount).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-chinese-naming", Title: "Chinese/Taoism Religious & Mythological Service Names", Description: "Alternate or additional naming: Pangu (bootstrap/creation), Nuwa (self-healing/repair), Sun Wukong (chaos eng/monkey king), Guanyin (observability/all-seeing), Long Wang (network fabric/dragon king), Zhurong (firewall/fire god), Fuxi (state management/order), Yama (cleanup/reaper), Jade Emperor (control plane), Bai Ze (knowledge/wisdom).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-japanese-naming", Title: "Japanese/Shinto/FF/Anime Religious & Mythological Service Names", Description: "Alternate or additional naming: Amaterasu (observability/sun/light), Susanoo (chaos eng/storm), Tsukuyomi (scheduler/moon/cycles), Izanagi (provisioner/creator), Raijin (load balancer/thunder), Fujin (CDN/wind), Inari (storage/abundance), Jizo (guardian/security), Tengu (WAF/protector), Kitsune (shape-shifting/service mesh).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-pagan-naming", Title: "Pagan/Wiccan/Druidic Mythological Service Names", Description: "Alternate or additional naming: Cernunnos (network fabric/horned god of connections), Brigid (firewall/forge-keeper/healer), Morrigan (chaos eng/phantom queen), Dagda (control plane/all-father with cauldron of plenty), Danu (provisioner/mother of creation), Lugh (load balancer/master of all skills), Arianrhod (scheduler/silver wheel of time), Cerridwen (state store/cauldron of transformation), Herne (service mesh/wild hunt leader), Greenman (self-healing/regeneration spirit).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-shaman-naming", Title: "Shamanistic/Animist/Indigenous Mythological Service Names", Description: "Alternate or additional naming: Thunderbird (load balancer/storm bringer), Coyote (chaos eng/trickster), Spider Woman (service mesh/weaver of connections), Raven (observability/all-seeing creator), Bear Spirit (WAF/guardian protector), Eagle Spirit (gateway/sky surveyor), Snake Spirit (secrets/shedding/renewal), Wendigo (garbage collector/consumer), Dreamtime (event sourcing/eternal now), World Tree (network fabric/axis mundi connecting all layers).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-jewish-naming", Title: "Jewish/Kabbalistic/Hebrew Mythological Service Names", Description: "Alternate or additional naming: Sefirot (service mesh/ten emanations), Ein Sof (control plane/the infinite), Golem (container runtime/animated construct), Shekinah (observability/divine presence), Metatron (gateway/angel of the presence), Tikkun (self-healing/repair of the world), Merkabah (scheduler/divine chariot), Tzimtzum (namespace isolation/divine contraction), Ruach (message bus/divine breath/wind), Malakh (deploy pipeline/messenger angel).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-islamic-naming", Title: "Islamic/Sufi/Arabic Mythological Service Names", Description: "Alternate or additional naming: Jibril (message bus/angel of revelation), Israfil (alerting/trumpet of judgment), Buraq (scheduler/lightning-fast mount), Ruh (control plane/the spirit), Nur (observability/divine light), Mizan (load balancer/scales of balance), Barzakh (network isolation/barrier between realms), Lawh al-Mahfuz (event log/preserved tablet), Qadr (state reconciler/divine decree), Djinn (container runtime/beings of smokeless fire).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-christian-naming", Title: "Christian/Biblical/Angelic Mythological Service Names", Description: "Alternate or additional naming: Seraphim (firewall/burning guardians), Cherubim (WAF/gate keepers of Eden), Logos (control plane/the Word/divine reason), Paraclete (message bus/the helper/advocate), Ark (state store/covenant container), Manna (provisioner/heaven-sent resources), Babel (DNS resolver/language of all tongues), Lazarus (self-healing/resurrection), Alpha-Omega (lifecycle manager/beginning and end), Burning Bush (alerting/unconsumed signal fire).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-wushu-naming", Title: "Taoist Wu Shu (Eight Forms) Infrastructure Service Names", Description: "Map the eight classical forms of Wu Shu to infra services: Form 1 Zhouyu/Incantations (gRPC service definitions/API invocations — the verbalized contracts that invoke services), Form 2 Zhouzu/Divine Justice (chaos engineering/circuit breakers/kill switches — judge-jury-executioner with guardrails), Form 3 Zhanbu/Divination (observability/predictive analytics/anomaly detection — I Ching as pattern recognition, geomancy as topology mapping), Form 4 Qubing/Warding-and-Curing (firewall/WAF/IDS/security hardening — spiritual physician as network healer), Form 5 Qumo/Exorcism (garbage collection/zombie reaping/dead process cleanup — fundamental not advanced), Form 6 Fu/Talismans (config-as-code/sealed secrets/TLS certificates — magical writings that petition services, the Foo petitions that bind intent to infrastructure), Form 7 Yaowu/Herbology (dependency management/package curation/CVE scanning — understanding poison AND antidotes, TCM as holistic system health), Form 8 Tongling/Mediumship (message bus/gRPC streaming/inter-service communication — altered states as protocol negotiation, channeling as pub-sub, astral journeying as distributed tracing).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-taoist-pantheon-naming", Title: "Taoist Pantheon Deep-Cut Service Names", Description: "Beyond the broad strokes in wish-chinese-naming — deep Taoist pantheon: Tai Yi (init system/primordial bootstrap), San Qing/Three Pure Ones (trinity architecture — control plane/data plane/management plane), Domu/Big Dipper Mother (scheduler/celestial navigation/cron), Lei Gong (load balancer/thunder distribution), Xi Wang Mu/Queen Mother of the West (chaos-to-order transformer — primordial dark goddess tamed into empress, like raw packets shaped into structured flows), Zao Jun/Kitchen God (local monitoring agent/node-level observer that reports upstream), Guan Di (WAF/loyalty guardian), Lu Dongbin (deploy pipeline/immortal alchemist who wanders between realms), He Xiangu (health checks/immortal healer), Zhongli Quan (capacity planning/fan that revives the dead = auto-scaling), Wei Huacun (3rd-century priestess — documentation/knowledge base curator), Ge Hong (alchemist-statesman — CI/CD philosopher, ancestor of Ge Xuan the ceremonial magician).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},
		{ID: "wish-taoist-alchemy-naming", Title: "Taoist Alchemy & Metaphysics Conceptual Service Names", Description: "Inner/outer alchemy and metaphysics mapped to infra: Neidan/Inner Alchemy (internal service optimization/profiling/the body as tripod cauldron = container self-tuning), Waidan/Outer Alchemy (external tooling/CI-CD/build systems — ritual cauldron crafting physical elixirs of power = artifact builds), Jing-Chi-Shen Trinity (Jing=storage/data layer/nutritive force, Chi=compute/processing/life force, Shen=observability/consciousness/divine intelligence), Wu Xing/Five Movements (five-element resource model — Wood/growth, Fire/processing, Earth/storage, Metal/networking, Water/cooling/cleanup), Bagua/Eight Trigrams (eight-dimensional service health matrix), Yin-Yang (read/write split, hot/cold storage, active/passive failover), Junzi/The Sage (SLO/SLI targets — strive for perfection knowing you will never achieve 100% but never stop aspiring), Qigong (resource management/capacity planning/chi cultivation = the personal power that fuels all rituals, without compute budget your talismans are empty).", Status: "todo", Type: "wishlist", Owner: "Captain", Progress: 0, CreatedAt: now, UpdatedAt: now},

		// REFACTOR
		{ID: "refactor-taskmanager-rename", Title: "Rename TaskManager to Mythology-Aligned Name", Description: "TaskManager is the nervous system coordinator. Rename to something fitting: Hermes (messenger), Iris (rainbow messenger), Vayu (Hindu wind/messenger), Fujin (wind god), Bifrost (bridge). The unheaded-wotan SKILL stays — only the Go struct/service gets renamed.", Status: "todo", Type: "tech-debt", Owner: "Developer", Progress: 0, CreatedAt: now, UpdatedAt: now},
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// API routes - tasks at canonical /api/v1/tasks
	mux.HandleFunc("/api/v1/tasks", s.handleTasks)
	mux.HandleFunc("/api/v1/tasks/", s.handleTaskByID) // For /tasks/:id
	mux.HandleFunc("/api/v1/timeline/tasks", s.handleTasks) // Legacy compatibility
	mux.HandleFunc("/api/v1/stream", s.handleSSE)
	mux.HandleFunc("/ws", s.handleWebSocket) // WebSocket endpoint
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ready", s.handleReady)
	mux.HandleFunc("/metrics", s.handleMetrics)

	// Timeline API endpoints - THE META MOMENT
	mux.HandleFunc("/api/v1/timeline", s.handleTimeline)
	mux.HandleFunc("/api/v1/timeline/cards", s.handleTimelineCards)

	// Static files - embedded in binary
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		// Fallback to serving from filesystem during development
		log.Warn().Msg("Using filesystem for static files (development mode)")
		mux.Handle("/", http.FileServer(http.Dir("static")))
	} else {
		log.Info().Msg("Serving embedded static files")
		mux.Handle("/", http.FileServer(http.FS(staticFS)))
	}

	// Apply middleware stack (order matters!)
	var handler http.Handler = mux
	handler = s.loggingMiddleware(handler)                          // Logging (innermost — sees request ID)
	handler = requestIDMiddleware(handler)                           // X-Request-ID injection
	handler = requestSizeLimitMiddleware(1024 * 1024)(handler)      // 1MB max request
	handler = securityHeadersMiddleware(handler)                     // Security headers

	// Auth middleware (activated via AUTH_ENABLED=true, skips /health /ready /metrics)
	authCfg := auth.LoadServiceAuthConfig("kanban-app")
	handler = auth.WrapHandler(handler, auth.SetupMiddleware(authCfg))

	handler = corsMiddleware(handler)                                // CORS

	// Rate limiting (configurable)
	if getEnv("RATE_LIMIT_ENABLED", "true") == "true" {
		rateLimiter := NewRateLimiter(s.ctx, 120, 30) // 120 req/min, burst 30 (page load is ~15 resources)
		handler = rateLimitMiddleware(rateLimiter)(handler)
		log.Info().Msg("Rate limiting enabled: 120 req/min, burst 30")
	}

	s.httpServer = &http.Server{
		Addr:           ":" + s.config.Port,
		Handler:        handler,
		ReadTimeout:    s.config.ReadTimeout,
		WriteTimeout:   s.config.WriteTimeout,
		IdleTimeout:    5 * s.config.ReadTimeout,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	log.Info().
		Str("port", s.config.Port).
		Str("timeguru", s.config.TimeGuruAddr).
		Str("wotan", s.config.WotanAddr).
		Bool("wotan_enabled", s.taskManager != nil).
		Msg("Starting kanban-app server")

	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	// Cancel server-scoped context (stops rate limiter cleanup, etc.)
	s.cancel()

	// Close all SSE connections
	s.sseMu.Lock()
	for ch := range s.sseClients {
		close(ch)
		delete(s.sseClients, ch)
	}
	s.sseMu.Unlock()

	return s.httpServer.Shutdown(ctx)
}

// getTimelineManager returns the active TimelineManager.
// Prefers TaskManager's timeline (Wotan-backed) when available,
// falls back to standalone HTTP-polling TimelineManager.
func (s *Server) getTimelineManager() *TimelineManager {
	if s.taskManager != nil {
		return s.taskManager.timelineManager
	}
	return s.timelineManager
}

// fetchTimelineFromTimeguru fetches the timeline via HTTP from Timeguru
func (s *Server) fetchTimelineFromTimeguru() error {
	url := fmt.Sprintf("http://%s/timeline", s.config.TimeGuruAddr)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("fetch timeline: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("timeguru returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Timeline *Timeline `json:"timeline"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode timeline: %w", err)
	}

	if result.Timeline == nil {
		return fmt.Errorf("timeguru returned nil timeline")
	}

	tm := s.getTimelineManager()
	if tm == nil {
		return fmt.Errorf("no timeline manager available")
	}
	return tm.UpdateTimeline(result.Timeline)
}

// pollTimeguru periodically fetches timeline from Timeguru via HTTP.
// Used as fallback when Wotan is unavailable, or to seed initial data.
func (s *Server) pollTimeguru(ctx context.Context, interval time.Duration) {
	// Initial fetch
	if err := s.fetchTimelineFromTimeguru(); err != nil {
		log.Warn().Err(err).Str("addr", s.config.TimeGuruAddr).Msg("initial timeline fetch failed (will retry)")
	} else {
		tm := s.getTimelineManager()
		var count int
		if tm != nil {
			count = len(tm.GetTimelineTasks())
		}
		log.Info().Int("tasks", count).Msg("loaded timeline from Timeguru via HTTP")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.fetchTimelineFromTimeguru(); err != nil {
				log.Warn().Err(err).Msg("timeline poll failed")
			}
		}
	}
}

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status
		wrapped := &statusWriter{ResponseWriter: w, status: 200}

		next.ServeHTTP(wrapped, r)

		log.Info().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", wrapped.status).
			Dur("duration", time.Since(start)).
			Str("remote_addr", r.RemoteAddr).
			Str("request_id", getRequestID(r.Context())).
			Msg("request")
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Flush proxies to the underlying ResponseWriter if it supports http.Flusher.
// Required for SSE (Server-Sent Events) and streaming responses.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// handleTasks routes task operations (GET, POST, PUT, DELETE)
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetTasks(w, r)
	case http.MethodPost:
		s.handleCreateTask(w, r)
	case http.MethodPut:
		s.handleUpdateTask(w, r)
	case http.MethodDelete:
		s.handleDeleteTask(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetTasks returns all tasks
func (s *Server) handleGetTasks(w http.ResponseWriter, r *http.Request) {
	var tasks interface{}
	var count int

	if s.taskManager != nil {
		// Prefer Wotan-backed TaskManager
		taskList := s.taskManager.GetAllTasks()
		tasks = taskList
		count = len(taskList)
	} else if tm := s.getTimelineManager(); tm != nil {
		// Fallback: timeline tasks from direct Timeguru HTTP polling
		timelineTasks := tm.GetTimelineTasks()
		if len(timelineTasks) > 0 {
			tasks = timelineTasks
			count = len(timelineTasks)
		}
	}

	// Next: SQLite store (L1 persistence)
	if tasks == nil && s.store != nil {
		if storeTasks, err := s.store.GetAllTasks(); err == nil && len(storeTasks) > 0 {
			tasks = storeTasks
			count = len(storeTasks)
		}
	}

	// Last resort: in-memory hardcoded tasks
	if tasks == nil {
		s.tasksMu.RLock()
		tasks = s.tasks
		count = len(s.tasks)
		s.tasksMu.RUnlock()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tasks": tasks,
		"count": count,
	})
}

// handleCreateTask creates a new task (POST)
func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate input
	if err := validateTaskInput(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.taskManager != nil {
		// Create task via TaskManager
		ctx := r.Context()
		if err := s.taskManager.CreateTask(ctx, &task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to create task")

			switch err {
			case ErrTaskAlreadyExists:
				http.Error(w, err.Error(), http.StatusConflict)
			case ErrInvalidTask:
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else if s.store != nil {
		// Standalone + SQLite persistence
		now := time.Now()
		task.CreatedAt = now
		task.UpdatedAt = now
		if existing, _ := s.store.GetTask(task.ID); existing != nil {
			http.Error(w, "task already exists", http.StatusConflict)
			return
		}
		if err := s.store.SaveTask(&task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to save task to store")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		// Fallback: in-memory only (no persistence)
		now := time.Now()
		task.CreatedAt = now
		task.UpdatedAt = now
		s.tasksMu.Lock()
		for _, t := range s.tasks {
			if t.ID == task.ID {
				s.tasksMu.Unlock()
				http.Error(w, "task already exists", http.StatusConflict)
				return
			}
		}
		s.tasks = append(s.tasks, task)
		s.tasksMu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"task": task,
	})
}

// handleUpdateTask updates an existing task (PUT)
func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate input
	if err := validateTaskInput(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if s.taskManager != nil {
		// Update task via TaskManager
		ctx := r.Context()
		if err := s.taskManager.UpdateTask(ctx, &task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to update task")

			switch err {
			case ErrTaskNotFound:
				http.Error(w, err.Error(), http.StatusNotFound)
			case ErrInvalidTask:
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else if s.store != nil {
		// Standalone + SQLite persistence
		existing, err := s.store.GetTask(task.ID)
		if err != nil {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
		task.CreatedAt = existing.CreatedAt
		task.UpdatedAt = time.Now()
		if err := s.store.SaveTask(&task); err != nil {
			log.Error().Err(err).Str("task_id", task.ID).Msg("failed to update task in store")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		// Fallback: in-memory only
		s.tasksMu.Lock()
		found := false
		for i, t := range s.tasks {
			if t.ID == task.ID {
				task.CreatedAt = t.CreatedAt
				task.UpdatedAt = time.Now()
				s.tasks[i] = task
				found = true
				break
			}
		}
		s.tasksMu.Unlock()
		if !found {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"task": task,
	})
}

// handleDeleteTask deletes a task (DELETE)
func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	// Parse task ID from query parameter
	taskID := r.URL.Query().Get("id")
	if taskID == "" {
		http.Error(w, "Missing 'id' query parameter", http.StatusBadRequest)
		return
	}

	if s.taskManager != nil {
		// Delete task via TaskManager
		ctx := r.Context()
		if err := s.taskManager.DeleteTask(ctx, taskID); err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("failed to delete task")

			switch err {
			case ErrTaskNotFound:
				http.Error(w, err.Error(), http.StatusNotFound)
			case ErrEmptyTaskID:
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else if s.store != nil {
		// Standalone + SQLite persistence
		if err := s.store.DeleteTask(taskID); err != nil {
			if errors.Is(err, ErrStoreNotFound) {
				http.Error(w, "task not found", http.StatusNotFound)
			} else {
				log.Error().Err(err).Str("task_id", taskID).Msg("failed to delete task from store")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else {
		// Fallback: in-memory only
		s.tasksMu.Lock()
		found := false
		for i, t := range s.tasks {
			if t.ID == taskID {
				s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
				found = true
				break
			}
		}
		s.tasksMu.Unlock()
		if !found {
			http.Error(w, "task not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"deleted": true,
		"task_id": taskID,
	})
}

// handleTaskByID handles requests to /api/v1/tasks/:id
func (s *Server) handleTaskByID(w http.ResponseWriter, r *http.Request) {
	// Extract task ID from path
	path := r.URL.Path
	prefix := "/api/v1/tasks/"
	if !strings.HasPrefix(path, prefix) {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	taskID := strings.TrimPrefix(path, prefix)
	if taskID == "" {
		http.Error(w, "Task ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetTaskByID(w, r, taskID)
	case http.MethodPut:
		s.handleUpdateTaskByID(w, r, taskID)
	case http.MethodDelete:
		s.handleDeleteTaskByID(w, r, taskID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetTaskByID returns a single task
func (s *Server) handleGetTaskByID(w http.ResponseWriter, r *http.Request, taskID string) {
	if s.taskManager != nil {
		task, err := s.taskManager.GetTask(taskID)
		if err != nil {
			if err == ErrTaskNotFound {
				http.Error(w, "Task not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(task)
		return
	}

	// Standalone + SQLite
	if s.store != nil {
		task, err := s.store.GetTask(taskID)
		if err != nil {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(task)
		return
	}

	// Fallback: in-memory
	s.tasksMu.RLock()
	defer s.tasksMu.RUnlock()
	for _, t := range s.tasks {
		if t.ID == taskID {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(t)
			return
		}
	}
	http.Error(w, "Task not found", http.StatusNotFound)
}

// handleUpdateTaskByID updates a single task
func (s *Server) handleUpdateTaskByID(w http.ResponseWriter, r *http.Request, taskID string) {
	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}
	task.ID = taskID // Ensure ID from URL

	if s.taskManager != nil {
		if err := s.taskManager.UpdateTask(r.Context(), &task); err != nil {
			if err == ErrTaskNotFound {
				http.Error(w, "Task not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else if s.store != nil {
		// Standalone + SQLite persistence
		existing, err := s.store.GetTask(taskID)
		if err != nil {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
		task.CreatedAt = existing.CreatedAt
		task.UpdatedAt = time.Now()
		if err := s.store.SaveTask(&task); err != nil {
			log.Error().Err(err).Str("task_id", taskID).Msg("failed to update task in store")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
	} else {
		// Fallback: in-memory only
		s.tasksMu.Lock()
		found := false
		for i, t := range s.tasks {
			if t.ID == taskID {
				task.CreatedAt = t.CreatedAt
				task.UpdatedAt = time.Now()
				s.tasks[i] = task
				found = true
				break
			}
		}
		s.tasksMu.Unlock()
		if !found {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"task": task})
}

// handleDeleteTaskByID deletes a single task
func (s *Server) handleDeleteTaskByID(w http.ResponseWriter, r *http.Request, taskID string) {
	if s.taskManager != nil {
		if err := s.taskManager.DeleteTask(r.Context(), taskID); err != nil {
			if err == ErrTaskNotFound {
				http.Error(w, "Task not found", http.StatusNotFound)
			} else {
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else if s.store != nil {
		// Standalone + SQLite persistence
		if err := s.store.DeleteTask(taskID); err != nil {
			if errors.Is(err, ErrStoreNotFound) {
				http.Error(w, "Task not found", http.StatusNotFound)
			} else {
				log.Error().Err(err).Str("task_id", taskID).Msg("failed to delete task from store")
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
			return
		}
	} else {
		// Fallback: in-memory only
		s.tasksMu.Lock()
		found := false
		for i, t := range s.tasks {
			if t.ID == taskID {
				s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)
				found = true
				break
			}
		}
		s.tasksMu.Unlock()
		if !found {
			http.Error(w, "Task not found", http.StatusNotFound)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"deleted": true, "task_id": taskID})
}

// handleWebSocket handles WebSocket connections for real-time updates
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// WebSocket upgrade handled by SSE for now (simpler, works everywhere)
	// Full WebSocket implementation can be added later
	s.handleSSE(w, r)
}

// validateTaskInput validates HTTP request task input
func validateTaskInput(task *Task) error {
	if task == nil {
		return errors.New("task cannot be nil")
	}
	if task.ID == "" {
		return errors.New("task.id is required")
	}
	if task.Title == "" {
		return errors.New("task.title is required")
	}
	if task.Status == "" {
		return errors.New("task.status is required")
	}

	// Validate ID format (alphanumeric + hyphens only)
	for _, r := range task.ID {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
			return errors.New("task.id contains invalid characters (use alphanumeric, -, _)")
		}
	}

	// Validate title length
	if len(task.Title) > 200 {
		return errors.New("task.title too long (max 200 characters)")
	}

	// Validate description length
	if len(task.Description) > 1000 {
		return errors.New("task.description too long (max 1000 characters)")
	}

	return nil
}

// handleSSE handles Server-Sent Events for real-time updates
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Check if client supports SSE
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// CORS origin set by corsMiddleware — no duplicate wildcard here

	// Create client channel
	clientCh := make(chan []byte, 10)

	s.sseMu.Lock()
	s.sseClients[clientCh] = true
	s.sseMu.Unlock()

	// Cleanup on disconnect
	defer func() {
		s.sseMu.Lock()
		delete(s.sseClients, clientCh)
		s.sseMu.Unlock()
	}()

	// Send initial tasks — prefer kanban cards (GetAllTasks) over timeline milestones
	var initialData []byte
	if s.taskManager != nil {
		// Wotan mode: send real kanban task cards
		allTasks := s.taskManager.GetAllTasks()
		if len(allTasks) > 0 {
			initialData, _ = json.Marshal(allTasks)
			log.Info().Int("count", len(allTasks)).Msg("SSE sending kanban tasks (Wotan)")
		}
	} else if tm := s.getTimelineManager(); tm != nil {
		// Standalone mode: send timeline milestones as tasks
		timelineTasks := tm.GetTimelineTasks()
		if len(timelineTasks) > 0 {
			initialData, _ = json.Marshal(timelineTasks)
			log.Info().Int("count", len(timelineTasks)).Msg("SSE sending timeline tasks (Timeguru HTTP)")
		}
	}
	if initialData == nil {
		// Last resort: hardcoded fallback tasks
		s.tasksMu.RLock()
		initialData, _ = json.Marshal(s.tasks)
		s.tasksMu.RUnlock()
	}

	fmt.Fprintf(w, "event: tasks\ndata: %s\n\n", initialData)
	flusher.Flush()

	// Keep-alive ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// Wait for events or disconnect
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, ok := <-clientCh:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// handleHealth returns health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	timelineSubscribed := false
	if s.taskManager != nil {
		timelineSubscribed = s.taskManager.IsTimelineSubscribed()
	}

	timelineHTTP := false
	if tm := s.getTimelineManager(); tm != nil {
		timelineHTTP = tm.GetTimeline() != nil
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":              "healthy",
		"timestamp":           time.Now().UTC().Format(time.RFC3339),
		"version":             "0.1.0",
		"wotan_enabled":      s.taskManager != nil,
		"timeline_subscribed": timelineSubscribed,
		"timeline_http":       timelineHTTP,
		"timeguru_addr":       s.config.TimeGuruAddr,
	})
}

// handleReady returns readiness status.
// Delegates to the transport HealthServer when available.
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	if s.healthSrv != nil {
		status := s.healthSrv.Status()
		w.Header().Set("Content-Type", "application/json")
		if status == transport.StatusDown {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"ready":   false,
				"reason":  string(status),
				"service": "kanban-app",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ready":   true,
			"reason":  string(status),
			"service": "kanban-app",
		})
		return
	}

	// Legacy fallback when healthSrv is not wired
	w.Header().Set("Content-Type", "application/json")

	ready := true
	reason := "ready"

	// In standalone mode (no TaskManager), we're still ready to serve
	if s.taskManager == nil {
		reason = "standalone mode"
	}

	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"ready":   ready,
		"reason":  reason,
		"service": "kanban-app",
	})
}

// handleMetrics returns Prometheus-style metrics
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	s.sseMu.RLock()
	sseClients := len(s.sseClients)
	s.sseMu.RUnlock()

	taskCount := 0
	if s.taskManager != nil {
		taskCount = len(s.taskManager.GetAllTasks())
	} else {
		s.tasksMu.RLock()
		taskCount = len(s.tasks)
		s.tasksMu.RUnlock()
	}

	fmt.Fprintf(w, "# HELP kanban_tasks_total Total tasks\n")
	fmt.Fprintf(w, "# TYPE kanban_tasks_total gauge\n")
	fmt.Fprintf(w, "kanban_tasks_total %d\n", taskCount)
	fmt.Fprintf(w, "# HELP kanban_sse_clients_active Active SSE client connections\n")
	fmt.Fprintf(w, "# TYPE kanban_sse_clients_active gauge\n")
	fmt.Fprintf(w, "kanban_sse_clients_active %d\n", sseClients)
	fmt.Fprintf(w, "# HELP kanban_wotan_enabled Whether Wotan integration is enabled\n")
	fmt.Fprintf(w, "# TYPE kanban_wotan_enabled gauge\n")
	if s.taskManager != nil {
		fmt.Fprintf(w, "kanban_wotan_enabled 1\n")
	} else {
		fmt.Fprintf(w, "kanban_wotan_enabled 0\n")
	}
}

// handleTimeline returns the current timeline data
func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tm := s.getTimelineManager()
	if tm == nil {
		http.Error(w, "Timeline not available", http.StatusServiceUnavailable)
		return
	}

	timeline := tm.GetTimeline()

	w.Header().Set("Content-Type", "application/json")

	if timeline == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"timeline": nil,
			"message":  "No timeline data available yet. Waiting for Timeguru updates.",
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"timeline": timeline,
	})
}

// handleTimelineCards returns timeline data converted to Kanban cards
func (s *Server) handleTimelineCards(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var tasks interface{}
	var count int
	source := "timeline"

	// Try Wotan-backed TaskManager first
	if s.taskManager != nil {
		timelineTasks := s.taskManager.GetTimelineTasks()
		if len(timelineTasks) > 0 {
			tasks = timelineTasks
			count = len(timelineTasks)
			source = "timeline-wotan"
		}
	}

	// Fallback: standalone TimelineManager (direct Timeguru HTTP)
	if tasks == nil {
		if tm := s.getTimelineManager(); tm != nil {
			timelineTasks := tm.GetTimelineTasks()
			if len(timelineTasks) > 0 {
				tasks = timelineTasks
				count = len(timelineTasks)
				source = "timeline-http"
			}
		}
	}

	// Last resort: regular tasks
	if tasks == nil {
		if s.taskManager != nil {
			taskList := s.taskManager.GetAllTasks()
			tasks = taskList
			count = len(taskList)
		} else {
			s.tasksMu.RLock()
			tasks = s.tasks
			count = len(s.tasks)
			s.tasksMu.RUnlock()
		}
		source = "fallback"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tasks":  tasks,
		"count":  count,
		"source": source,
	})
}

// broadcastUpdate sends an update to all SSE clients
func (s *Server) broadcastUpdate(eventType string, data interface{}) {
	msg, err := json.Marshal(map[string]interface{}{
		"type": eventType,
		"data": data,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to marshal broadcast message")
		return
	}

	s.sseMu.RLock()
	defer s.sseMu.RUnlock()

	for ch := range s.sseClients {
		select {
		case ch <- msg:
		default:
			// Channel full, skip
		}
	}
}

func main() {
	// Setup logging - use Kingdom's native logger
	log = logger.NewWithConfig(logger.Config{
		Level:         logger.InfoLevel,
		TimeFormat:    time.RFC3339,
		CallerEnabled: true,
		ConsoleMode:   true,
		Output:        os.Stderr,
	})

	// Load config
	cfg := Config{
		Port:            getEnv("PORT", "20001"),
		TimeGuruAddr:    getEnv("TIMEGURU_ADDR", "localhost:19000"),
		WotanAddr:      getEnv("WOTAN_ADDR", "localhost:18000"),
		DataDir:         getEnv("DATA_DIR", "./data"),
		ReadTimeout:     30 * time.Second,
		WriteTimeout:    30 * time.Second,
		ShutdownTimeout: 10 * time.Second,
	}

	// Transport config — unified env-based configuration
	transportCfg := transport.DefaultConfig()
	transport.ConfigFromEnv(&transportCfg)

	// Override transport config with service-level WotanAddr if set
	if cfg.WotanAddr != "" {
		transportCfg.WotanHTTPAddr = "http://" + cfg.WotanAddr
	}

	// Health server — dual-protocol health tracking
	healthSrv := transport.NewHealthServer("kanban-app")

	// Log aggregation publisher — forwards structured logs to Wotan
	var logConn transport.Connection
	if cfg.WotanAddr != "" {
		logConn, _ = transport.Connect(context.Background(), transportCfg) // best-effort; nil on failure
	}
	logPublisher := logagg.NewPublisher("kanban-app", logConn)
	if logPublisher.Enabled() {
		log.AddHook(logger.HookFunc(func(entry *logger.Entry) error {
			// Adapt custom logger Entry to zerolog hook call.
			// The Publisher.Run method handles Wotan forwarding on a
			// best-effort, non-blocking basis.
			logPublisher.Run(nil, entryLevelToZerolog(entry.Level), entry.Message)
			return nil
		}))
		log.Info().Msg("Log forwarding to Wotan enabled via logagg publisher")
	}

	// Service discovery — best-effort registration (nil conn until transport is wired)
	discoveryCtx, discoveryCancel := context.WithCancel(context.Background())
	defer discoveryCancel()
	discovery.SetupServiceDiscovery(discoveryCtx, nil, "kanban-app", 20001)

	// Initialize Wotan client
	// WOTAN_ADDR = HTTP control plane (subscribe, publish, admin, circuit-breaker fallback)
	// WOTAN_GRPC_ADDR = gRPC data plane (TopicStream) — primary transport for streaming
	//
	// Transport strategy (Campaign 1 — TopicStream gRPC Sprint):
	//   1. If WOTAN_GRPC_ADDR is set → create TopicStreamClient (gRPC streaming primary)
	//      with HTTP Client as circuit-breaker fallback
	//   2. If gRPC unavailable → create plain HTTP Client (polling mode)
	//   3. If WOTAN_ENABLED=false → standalone mode (no Wotan)
	var server *Server
	wotanEnabled := getEnv("WOTAN_ENABLED", "true") == "true"
	wotanGRPCAddr := getEnv("WOTAN_GRPC_ADDR", "localhost:18001")
	if wotanGRPCAddr != "" {
		transportCfg.WotanGRPCAddr = wotanGRPCAddr
	}

	if wotanEnabled {
		log.Info().
			Str("wotan_http", cfg.WotanAddr).
			Str("wotan_grpc", wotanGRPCAddr).
			Msg("Initializing Wotan client")

		var busClient WotanClient
		var err error
		var transportName string

		if wotanGRPCAddr != "" {
			// Primary path: TopicStreamClient with gRPC streaming + HTTP fallback
			// Create HTTP client first as circuit-breaker fallback
			httpFallback, httpErr := wotanClient.NewClient(cfg.WotanAddr)
			if httpErr != nil {
				log.Warn().Err(httpErr).Msg("HTTP fallback client creation failed — TopicStream will run without fallback")
			}

			var tsOpts []wotanClient.TopicStreamOption
			tsOpts = append(tsOpts, wotanClient.WithRetryPolicy(
				100*time.Millisecond, // initial backoff
				30*time.Second,       // max backoff cap
				10,                   // max consecutive failures before circuit break
			))
			tsOpts = append(tsOpts, wotanClient.WithBufferSize(256))
			if httpFallback != nil {
				tsOpts = append(tsOpts, wotanClient.WithHTTPFallback(httpFallback))
			}

			tsc, tsErr := wotanClient.NewTopicStreamClient(wotanGRPCAddr, tsOpts...)
			if tsErr != nil {
				log.Warn().Err(tsErr).Msg("TopicStreamClient creation failed — falling back to HTTP polling client")
				busClient, err = wotanClient.NewClient(cfg.WotanAddr)
				transportName = "http-poll"
			} else {
				// Probe gRPC health to provide an informative startup log message.
				// The client will handle reconnection automatically regardless of the probe's outcome.
				probeCtx, probeCancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer probeCancel()
				if pingErr := tsc.Ping(probeCtx); pingErr != nil {
					log.Warn().Err(pingErr).Msg("gRPC health probe failed. Client will attempt to connect in the background")
					transportName = "grpc-stream (degraded)"
				} else {
					transportName = "grpc-stream"
				}
				busClient = tsc
			}
		} else {
			// No gRPC address — HTTP-only mode
			bc, bcErr := wotanClient.NewClient(cfg.WotanAddr)
			if bcErr != nil {
				err = bcErr
			} else {
				busClient = bc
				transportName = "http-poll"
			}
		}

		if transportName != "" {
			log.Info().Str("transport", transportName).Msg("Wotan transport selected")
		}

		if err != nil {
			log.Error().
				Err(err).
				Msg("Failed to create Wotan client, falling back to standalone mode")
			healthSrv.SetGRPCStatus(false)
			server = NewServer(cfg)
		} else {
			// Initialize task store: prefer Postgres, fall back to SQLite.
			var kanbanStore TaskStore

			pgDB, pgErr := database.Connect(context.Background(), database.AppKanbanConfig())
			if pgErr != nil {
				log.Warn().Err(pgErr).Msg("Postgres unavailable — trying SQLite fallback")
			} else {
				pgStore, pgStoreErr := NewPgStore(pgDB)
				if pgStoreErr != nil {
					log.Warn().Err(pgStoreErr).Msg("Postgres schema init failed — trying SQLite fallback")
					pgDB.Close()
				} else {
					kanbanStore = pgStore
					log.Info().Msg("Postgres connected — kanban tasks persisted to The Well")
				}
			}

			if kanbanStore == nil {
				// SQLite fallback
				dbPath := cfg.DataDir + "/kanban.db"
				if cfg.DataDir == "" {
					dbPath = "./data/kanban.db"
				}
				sqliteStore, sqliteErr := NewStore(dbPath)
				if sqliteErr != nil {
					log.Warn().Err(sqliteErr).Str("path", dbPath).Msg("SQLite store init failed — TaskManager will run in-memory only")
				} else {
					kanbanStore = sqliteStore
				}
			}

			// Create TaskManager with broadcast function and Store
			broadcast := func(eventType string, data interface{}) {
				if server != nil {
					server.broadcastUpdate(eventType, data)
				}
			}

			taskManager, err := NewTaskManager(busClient, broadcast, kanbanStore)
			if err != nil {
				log.Error().
					Err(err).
					Msg("Failed to create TaskManager, falling back to standalone mode")
				healthSrv.SetGRPCStatus(false)
				server = NewServer(cfg)
			} else {
				// Initialize TaskManager (loads tasks from Store + subscribes to Wotan)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := taskManager.Initialize(ctx); err != nil {
					log.Error().
						Err(err).
						Msg("Failed to initialize TaskManager, falling back to standalone mode")
					healthSrv.SetGRPCStatus(false)
					cancel()
					server = NewServer(cfg)
				} else {
					cancel()
					server = NewServerWithTaskManager(cfg, taskManager, kanbanStore)
					log.Info().
						Str("transport", transportName).
						Msg("Wotan integration enabled with persistent store")
				}
			}
		}
	} else {
		log.Warn().Msg("Wotan disabled, running in standalone mode")
		server = NewServer(cfg)
	}

	// Wire transport health server into the kanban server
	server.healthSrv = healthSrv

	// Start Timeguru HTTP polling ONLY in standalone mode.
	// When Wotan integration is active, timeline updates arrive via
	// the timeline.updates subscription — no polling needed.
	pollCtx, pollCancel := context.WithCancel(context.Background())
	if server.taskManager == nil {
		go server.pollTimeguru(pollCtx, 30*time.Second)
		log.Info().Msg("Timeguru HTTP polling enabled (standalone mode)")
	} else {
		log.Info().Msg("Timeguru HTTP polling disabled (Wotan handles timeline events)")
	}

	// Start server in goroutine
	go func() {
		if err := server.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Shutting down server...")

	// Stop timeline polling
	pollCancel()

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	// Close TaskManager if present
	if server.taskManager != nil {
		if err := server.taskManager.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close TaskManager")
		}
	}

	// Close SQLite store if present
	if server.store != nil {
		if err := server.store.Close(); err != nil {
			log.Error().Err(err).Msg("Failed to close SQLite store")
		} else {
			log.Info().Msg("SQLite store closed cleanly")
		}
	}

	log.Info().Msg("Server exited")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// entryLevelToZerolog maps the custom logger.Level to zerolog.Level
// so the logagg.Publisher can forward entries at the correct severity.
func entryLevelToZerolog(level logger.Level) zerolog.Level {
	switch level {
	case logger.TraceLevel:
		return zerolog.TraceLevel
	case logger.DebugLevel:
		return zerolog.DebugLevel
	case logger.InfoLevel:
		return zerolog.InfoLevel
	case logger.WarnLevel:
		return zerolog.WarnLevel
	case logger.ErrorLevel:
		return zerolog.ErrorLevel
	case logger.FatalLevel:
		return zerolog.FatalLevel
	case logger.PanicLevel:
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}
