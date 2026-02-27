import { useState, useCallback, useEffect } from "react";

const SEVERITY = {
  S1: { label: "INFO", color: "#3b82f6", bg: "#1e3a5f" },
  S2: { label: "WARNING", color: "#f59e0b", bg: "#4a3500" },
  S3: { label: "CITATION", color: "#f97316", bg: "#4a2000" },
  S4: { label: "HALT", color: "#ef4444", bg: "#4a0000" },
};

const VIOLATION_TYPES = [
  "SCOPE CREEP",
  "TANGENT ALERT",
  "GOLD PLATE",
  "STALL DETECTED",
  "GATE SKIP",
  "PLAN DEVIATION",
  "COMMIT OVERDUE",
  "JURISDICTION",
  "SPEC VIOLATION",
  "STUCK LOOP",
];

const initialPhases = [
  {
    id: "p1",
    name: "ENV VERIFY",
    steps: [
      { id: "s1", text: "Check kernel version", tag: "B", status: "done", time: "~30s" },
      { id: "s2", text: "Verify required tools", tag: "V", status: "done", time: "~30s" },
      { id: "s3", text: "Install missing deps", tag: "D", status: "done", time: "~2m" },
      { id: "s4", text: "PHASE 1 EXIT GATE", tag: "V", status: "done", time: "~30s", isGate: true },
    ],
    gateStatus: "pass",
  },
  {
    id: "p2",
    name: "NAMESPACE ASSEMBLY",
    steps: [
      { id: "s5", text: "Create monad0 namespace", tag: "S|B", status: "done", time: "~30s" },
      { id: "s6", text: "Verify namespace exists", tag: "V", status: "done", time: "~30s" },
      { id: "s7", text: "Create veth pairs", tag: "S|B", status: "active", time: "~1m" },
      { id: "s8", text: "COMMIT CHECKPOINT", tag: "C", status: "pending", time: "~30s" },
      { id: "s9", text: "PHASE 2 EXIT GATE", tag: "V", status: "pending", time: "~30s", isGate: true },
    ],
    gateStatus: "pending",
  },
  {
    id: "p3",
    name: "BPF PROGRAM LOAD",
    steps: [
      { id: "s10", text: "Compile XDP program", tag: "B", status: "pending", time: "~2m" },
      { id: "s11", text: "Verify BPF bytecode", tag: "V", status: "pending", time: "~1m" },
      { id: "s12", text: "Attach to interface", tag: "S|B", status: "pending", time: "~1m" },
      { id: "s13", text: "PHASE 3 EXIT GATE", tag: "V", status: "pending", time: "~30s", isGate: true },
    ],
    gateStatus: "locked",
  },
  {
    id: "p4",
    name: "FIRST PACKET",
    steps: [
      { id: "s14", text: "Record initial CPU state", tag: "B", status: "pending", time: "~30s" },
      { id: "s15", text: "Inject single packet", tag: "S|B", status: "pending", time: "~1m" },
      { id: "s16", text: "Verify PC advanced", tag: "V", status: "pending", time: "~30s" },
      { id: "s17", text: "PHASE 4 EXIT GATE", tag: "V", status: "pending", time: "~30s", isGate: true },
    ],
    gateStatus: "locked",
  },
];

const initialCitations = [
  {
    id: "c1",
    time: "14:23",
    severity: "S2",
    type: "SCOPE CREEP",
    desc: '"While we\'re at it" — CSS gradient cleanup',
    resolution: "Parked → backlog",
  },
  {
    id: "c2",
    time: "14:41",
    severity: "S1",
    type: "COMMIT OVERDUE",
    desc: "6 steps since last commit (cadence: 4)",
    resolution: "Committed immediately",
  },
];

function Badge({ flashing }) {
  return (
    <div className="flex items-center gap-2">
      <div className="relative">
        <div
          className={`w-3 h-3 rounded-full ${flashing ? "bg-red-500" : "bg-green-500"}`}
          style={
            flashing
              ? { animation: "pulse 1s ease-in-out infinite", boxShadow: "0 0 8px rgba(239,68,68,0.6)" }
              : {}
          }
        />
      </div>
      <span className="text-xs font-mono text-gray-400">
        {flashing ? "ENFORCING" : "ALL CLEAR"}
      </span>
    </div>
  );
}

function TagBadge({ tag }) {
  const colors = {
    B: "bg-blue-900 text-blue-300",
    V: "bg-green-900 text-green-300",
    D: "bg-yellow-900 text-yellow-300",
    S: "bg-red-900 text-red-300",
    C: "bg-purple-900 text-purple-300",
    "S|B": "bg-red-900 text-orange-300",
    P: "bg-cyan-900 text-cyan-300",
  };
  return (
    <span className={`px-1.5 py-0.5 rounded text-xs font-mono ${colors[tag] || "bg-gray-800 text-gray-400"}`}>
      [{tag}]
    </span>
  );
}

function StepRow({ step, onToggle }) {
  const statusIcon =
    step.status === "done" ? "✓" : step.status === "active" ? "▶" : step.status === "stuck" ? "⚠" : "○";
  const statusColor =
    step.status === "done"
      ? "text-green-400"
      : step.status === "active"
      ? "text-yellow-300"
      : step.status === "stuck"
      ? "text-red-400"
      : "text-gray-600";

  return (
    <div
      className={`flex items-center gap-2 px-2 py-1 rounded cursor-pointer hover:bg-gray-800 ${
        step.isGate ? "border-l-2 border-yellow-500 bg-gray-900" : ""
      }`}
      onClick={() => onToggle(step.id)}
    >
      <span className={`font-mono text-sm ${statusColor}`}>{statusIcon}</span>
      <TagBadge tag={step.tag} />
      <span className={`text-sm flex-1 ${step.status === "done" ? "text-gray-500 line-through" : "text-gray-200"}`}>
        {step.text}
      </span>
      <span className="text-xs text-gray-600 font-mono">{step.time}</span>
    </div>
  );
}

function PhaseCard({ phase }) {
  const gateColors = {
    pass: { border: "border-green-600", bg: "bg-green-950", badge: "bg-green-700 text-green-100", label: "GATE PASS" },
    pending: { border: "border-yellow-600", bg: "bg-yellow-950", badge: "bg-yellow-700 text-yellow-100", label: "GATE PENDING" },
    fail: { border: "border-red-600", bg: "bg-red-950", badge: "bg-red-700 text-red-100", label: "GATE FAIL" },
    locked: { border: "border-gray-700", bg: "bg-gray-950", badge: "bg-gray-700 text-gray-400", label: "LOCKED" },
  };
  const g = gateColors[phase.gateStatus];

  const done = phase.steps.filter((s) => s.status === "done").length;
  const total = phase.steps.length;
  const pct = Math.round((done / total) * 100);

  return (
    <div className={`rounded-lg border ${g.border} ${g.bg} p-3 min-w-64`}>
      <div className="flex items-center justify-between mb-2">
        <h3 className="text-sm font-bold text-gray-200 font-mono">{phase.name}</h3>
        <span className={`text-xs px-2 py-0.5 rounded font-mono ${g.badge}`}>{g.label}</span>
      </div>
      <div className="w-full bg-gray-800 rounded-full h-1.5 mb-2">
        <div
          className="h-1.5 rounded-full transition-all duration-500"
          style={{
            width: `${pct}%`,
            backgroundColor: phase.gateStatus === "pass" ? "#22c55e" : phase.gateStatus === "fail" ? "#ef4444" : "#f59e0b",
          }}
        />
      </div>
      <div className="text-xs text-gray-500 mb-2 font-mono">
        {done}/{total} steps ({pct}%)
      </div>
      <div className="space-y-0.5">
        {phase.steps.map((s) => (
          <StepRow key={s.id} step={s} onToggle={() => {}} />
        ))}
      </div>
    </div>
  );
}

function CitationRow({ citation }) {
  const sev = SEVERITY[citation.severity];
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 rounded" style={{ backgroundColor: sev.bg + "40" }}>
      <span className="text-xs font-mono text-gray-500">{citation.time}</span>
      <span
        className="text-xs font-bold px-1.5 py-0.5 rounded font-mono"
        style={{ color: sev.color, backgroundColor: sev.bg }}
      >
        {citation.severity}
      </span>
      <span className="text-xs font-mono" style={{ color: sev.color }}>
        {citation.type}
      </span>
      <span className="text-xs text-gray-400 flex-1">{citation.desc}</span>
      <span className="text-xs text-gray-600">{citation.resolution}</span>
    </div>
  );
}

function MarshalPrompt({ onSubmit }) {
  const [input, setInput] = useState("");
  const [response, setResponse] = useState(null);

  const handleSubmit = useCallback(() => {
    if (!input.trim()) return;
    const lower = input.toLowerCase();
    let resp = null;

    // Scope creep detection
    const scopeTriggers = ["while we're at it", "one more thing", "actually let's also", "since we're here", "quick detour", "before we move on"];
    const scopeHit = scopeTriggers.find((t) => lower.includes(t));
    if (scopeHit) {
      resp = {
        type: "SCOPE CHECK",
        severity: "S2",
        message: `Detected scope expansion trigger: "${scopeHit}"`,
        ruling: "ADJACENT — Park it. Add to backlog. Stay on mission.",
        redirect: "Return to current active step.",
      };
    }

    // Stall detection
    if (!resp && (lower.includes("stuck") || lower.includes("been trying") || lower.includes("40 minutes") || lower.includes("not working"))) {
      resp = {
        type: "STALL DETECTED",
        severity: "S3",
        message: "Stall pattern detected. Multiple attempts without resolution.",
        ruling: "Invoke Skip Protocol. Commit current state. Move to next non-blocked step.",
        redirect: "Skip Protocol → Next actionable step.",
      };
    }

    // Gold plate detection
    if (!resp && (lower.includes("polish") || lower.includes("refactor") || lower.includes("make it better") || lower.includes("one more tweak"))) {
      resp = {
        type: "GOLD PLATE",
        severity: "S2",
        message: "Gold-plating detected. The gate has passed.",
        ruling: "SHIP IT. Move on. Perfect is the enemy of shipped.",
        redirect: "Proceed to next phase.",
      };
    }

    // Tangent
    if (!resp && (lower.includes("curious") || lower.includes("wonder") || lower.includes("side") || lower.includes("tangent") || lower.includes("what if we"))) {
      resp = {
        type: "TANGENT ALERT",
        severity: "S2",
        message: "Tangent detected. Current topic diverges from active phase.",
        ruling: "Time-box 5 min or kill. The highway has exits. This isn't one of them.",
        redirect: "Return to active battle plan step.",
      };
    }

    // Default — all clear
    if (!resp) {
      resp = {
        type: "ALL CLEAR",
        severity: "S1",
        message: "No violations detected. You're in your lane.",
        ruling: "Proceed.",
        redirect: "Continue current step.",
      };
    }

    setResponse(resp);
    if (onSubmit) onSubmit(resp);
  }, [input, onSubmit]);

  const sev = response ? SEVERITY[response.severity] : null;

  return (
    <div className="rounded-lg border border-gray-700 bg-gray-900 p-3">
      <div className="flex items-center gap-2 mb-2">
        <span className="text-yellow-500 text-sm">⚡</span>
        <span className="text-xs font-mono text-gray-400">MARSHAL PROMPT — Paste what you're about to do</span>
      </div>
      <div className="flex gap-2">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === "Enter" && handleSubmit()}
          placeholder='e.g. "while we\'re at it, let me fix the CSS..."'
          className="flex-1 bg-gray-800 border border-gray-700 rounded px-3 py-1.5 text-sm text-gray-200 font-mono focus:outline-none focus:border-yellow-600"
        />
        <button
          onClick={handleSubmit}
          className="bg-yellow-700 hover:bg-yellow-600 text-yellow-100 px-4 py-1.5 rounded text-sm font-mono font-bold transition-colors"
        >
          CHECK
        </button>
      </div>
      {response && (
        <div className="mt-3 rounded p-3 border" style={{ borderColor: sev.color + "60", backgroundColor: sev.bg + "30" }}>
          <div className="flex items-center gap-2 mb-1">
            <span className="text-xs font-bold px-2 py-0.5 rounded font-mono" style={{ color: sev.color, backgroundColor: sev.bg }}>
              {response.severity}
            </span>
            <span className="text-sm font-mono font-bold" style={{ color: sev.color }}>
              MARSHAL — {response.type}
            </span>
          </div>
          <p className="text-xs text-gray-300 mb-1">{response.message}</p>
          <p className="text-xs text-gray-400">
            <strong style={{ color: sev.color }}>RULING:</strong> {response.ruling}
          </p>
          <p className="text-xs text-gray-500 mt-1">↩ {response.redirect}</p>
        </div>
      )}
    </div>
  );
}

function ShiftReport({ citations, phases }) {
  const totalSteps = phases.reduce((a, p) => a + p.steps.length, 0);
  const doneSteps = phases.reduce((a, p) => a + p.steps.filter((s) => s.status === "done").length, 0);
  const compliance = totalSteps > 0 ? Math.round((doneSteps / totalSteps) * 100) : 0;

  return (
    <div className="rounded-lg border border-gray-700 bg-gray-900 p-3">
      <h3 className="text-sm font-mono font-bold text-gray-300 mb-2">MARSHAL SHIFT REPORT</h3>
      <div className="grid grid-cols-4 gap-2 text-center mb-2">
        <div className="bg-gray-800 rounded p-2">
          <div className="text-lg font-bold text-green-400">{doneSteps}</div>
          <div className="text-xs text-gray-500">Steps Done</div>
        </div>
        <div className="bg-gray-800 rounded p-2">
          <div className="text-lg font-bold text-yellow-400">{totalSteps - doneSteps}</div>
          <div className="text-xs text-gray-500">Remaining</div>
        </div>
        <div className="bg-gray-800 rounded p-2">
          <div className="text-lg font-bold text-orange-400">{citations.length}</div>
          <div className="text-xs text-gray-500">Citations</div>
        </div>
        <div className="bg-gray-800 rounded p-2">
          <div className="text-lg font-bold text-blue-400">{compliance}%</div>
          <div className="text-xs text-gray-500">Compliance</div>
        </div>
      </div>
    </div>
  );
}

export default function MarshalGate() {
  const [phases] = useState(initialPhases);
  const [citations, setCitations] = useState(initialCitations);
  const [activeTab, setActiveTab] = useState("board");

  const handlePromptResult = useCallback(
    (resp) => {
      if (resp.severity !== "S1") {
        setCitations((prev) => [
          ...prev,
          {
            id: `c${Date.now()}`,
            time: new Date().toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit", hour12: false }),
            severity: resp.severity,
            type: resp.type,
            desc: resp.message,
            resolution: "Pending",
          },
        ]);
      }
    },
    []
  );

  return (
    <div className="min-h-screen bg-gray-950 text-gray-200 p-4">
      <style>{`
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.3; }
        }
      `}</style>

      {/* Header */}
      <div className="flex items-center justify-between mb-4">
        <div className="flex items-center gap-3">
          <div className="text-2xl">🛡️</div>
          <div>
            <h1 className="text-lg font-bold font-mono text-yellow-500">UNHEADED MARSHAL</h1>
            <p className="text-xs text-gray-500 font-mono">THE BADGE. THE LAW. THE LANE ENFORCER.</p>
          </div>
        </div>
        <Badge flashing={citations.length > 0} />
      </div>

      {/* Tabs */}
      <div className="flex gap-1 mb-4 border-b border-gray-800 pb-1">
        {["board", "citations", "prompt"].map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-3 py-1.5 text-xs font-mono rounded-t transition-colors ${
              activeTab === tab ? "bg-gray-800 text-yellow-500 border border-gray-700 border-b-gray-800" : "text-gray-500 hover:text-gray-300"
            }`}
          >
            {tab === "board" ? "BATTLE BOARD" : tab === "citations" ? `CITATIONS (${citations.length})` : "MARSHAL PROMPT"}
          </button>
        ))}
      </div>

      {/* Board Tab */}
      {activeTab === "board" && (
        <div>
          <ShiftReport citations={citations} phases={phases} />
          <div className="mt-4 flex gap-3 overflow-x-auto pb-4">
            {phases.map((p) => (
              <PhaseCard key={p.id} phase={p} />
            ))}
          </div>
        </div>
      )}

      {/* Citations Tab */}
      {activeTab === "citations" && (
        <div className="space-y-1">
          <div className="text-xs font-mono text-gray-500 mb-2">ENFORCEMENT LOG — {citations.length} citations issued</div>
          {citations.map((c) => (
            <CitationRow key={c.id} citation={c} />
          ))}
          {citations.length === 0 && <div className="text-center text-gray-600 py-8 font-mono">No citations. All clear.</div>}
        </div>
      )}

      {/* Prompt Tab */}
      {activeTab === "prompt" && (
        <div className="space-y-3">
          <div className="text-xs font-mono text-gray-500">
            Test the Marshal — paste what you're about to say/do. The Marshal will check for violations.
          </div>
          <MarshalPrompt onSubmit={handlePromptResult} />
          <div className="rounded border border-gray-800 bg-gray-900 p-3">
            <div className="text-xs font-mono text-gray-500 mb-2">TRY THESE:</div>
            <div className="space-y-1">
              {[
                '"While we\'re at it, let me fix the dashboard CSS gradient..."',
                '"I\'ve been stuck on this BPF verifier for 40 minutes..."',
                '"Just curious, what if we used a different hashing algorithm?"',
                '"One more tweak to make this function cleaner..."',
              ].map((ex, i) => (
                <div key={i} className="text-xs text-gray-600 font-mono">
                  → {ex}
                </div>
              ))}
            </div>
          </div>
        </div>
      )}

      {/* Footer */}
      <div className="mt-6 border-t border-gray-800 pt-3 flex items-center justify-between">
        <span className="text-xs text-gray-600 font-mono">Marshal v1.0 — Forged Feb 26, 2026</span>
        <span className="text-xs text-gray-700 font-mono">STAY IN YOUR LANE. THE FINISH LINE IS AHEAD.</span>
      </div>
    </div>
  );
}
