// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

// Package claudebridge runs conversation turns against the Claude API
// (Messages API, official Go SDK) with tools drawn from zhentools
// Providers. Every tool call Claude makes passes through a Gate (the
// Champion's B2 gate in production): Claude is a model, not a trusted
// caller of Kingdom tools. Every request is held against a hard daily
// spend cap before it is sent and audited after. See ADR-104.
package claudebridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"unheaded/pkg/champion"
	"unheaded/pkg/claudebridge/privacy"
	"unheaded/pkg/zhentools"
)

// Defaults. The model and effort follow the claude-api skill: Claude Opus
// 5.5, whose effort default is medium, set explicitly so a server-side
// default change cannot move the bill.
const (
	DefaultModel              = "claude-opus-5-5"
	DefaultEffort             = "medium"
	DefaultMaxTokens          = 16000
	DefaultMaxIterations      = 16
	DefaultMaxToolResultBytes = 64 << 10
	// MinTaskBudget is the API's minimum task_budget.total.
	MinTaskBudget = 20000
)

// DefaultSystem is the operator prompt. It is fixed for a Bridge's
// lifetime so the prompt cache and thinking blocks stay valid.
const DefaultSystem = `You are working for Stevie Bellis on Unheaded, a GPL-3.0 configuration management automation platform, through Zhen's bridge to the Claude API.
Tools come from the Kingdom (the Champion and Zhen's MCP server). Every tool call you make is checked by the Champion gate. You are not a trusted caller: mutating tools (writes, patches, Kanban changes) are refused pending the user's out-of-band confirmation, and a refusal comes back as a tool error that says why. When a call is refused, do not retry it under another name or wording; say what you wanted to do and that it needs confirmation.
Answer plainly and briefly.`

var (
	// ErrMaxTokens: the response hit max_tokens; tool calls in it were not run.
	ErrMaxTokens = errors.New("response stopped at max_tokens")
	// ErrIterationLimit: the tool loop ran out of iterations.
	ErrIterationLimit = errors.New("tool loop hit its iteration limit")
	// ErrContextWindow: the conversation no longer fits the model's context.
	ErrContextWindow = errors.New("conversation exceeds the model context window")
)

// RefusalError is a stop_reason "refusal": the safety classifiers (or
// the model) declined, and any configured fallback declined too.
type RefusalError struct {
	Category    string
	Explanation string
	ServedBy    string
}

func (e *RefusalError) Error() string {
	s := "Claude declined the request (stop_reason refusal"
	if e.Category != "" {
		s += ", category " + e.Category
	}
	return s + ")"
}

// Config configures a Bridge. Gate, Ledger and Prices are required: there
// is no way to build a Bridge without a tool gate or a spend cap.
type Config struct {
	Model     string // default DefaultModel
	Effort    string // low|medium|high|xhigh|max; default DefaultEffort
	MaxTokens int64  // per request; default DefaultMaxTokens
	// MinMaxTokens, when > 0, lets a request whose worst case at
	// MaxTokens no longer fits what the Ledger has left go out with the
	// largest max_tokens that does fit, down to MinMaxTokens; below that
	// it is refused before sending. 0 (the default): never shrink, refuse.
	// Routed Claude legs set it so a small family cap buys more than one
	// request (S-CHAMPION-HARDENING Step 114, PLANS F-23).
	MinMaxTokens int64
	TaskBudget   int64 // advisory token budget per request (beta); 0 = off, else >= MinTaskBudget
	// DisableFallbacks turns off the server-side refusal fallback
	// (fallbacks: "default"), which is on by default.
	DisableFallbacks   bool
	System             string // default DefaultSystem
	MaxIterations      int    // API calls per Send; default DefaultMaxIterations
	MaxToolResultBytes int    // tool output sent to Claude is cut here; default DefaultMaxToolResultBytes

	Providers []NamedProvider
	Gate      Gate
	Prices    *PriceTable
	Ledger    *Ledger
	Auditor   Auditor // default: discard
	// Confirmer decides calls the gate refused pending confirmation
	// (Champion Rule 2) out of band. nil or DenyConfirmer: refused, as
	// before Phase 2. See confirm.go.
	Confirmer Confirmer
	// Privacy classifies, redacts and (for large ops payloads)
	// summarises the user prompt and every tool result before they are
	// sent (S-TWO-BRAINS Phase 3). nil: no filtering (library tests);
	// zhen-claude always sets it. ProjectRoot is the sandbox it
	// classifies file reads against.
	Privacy     *privacy.Filter
	ProjectRoot string
	// Warn, if set, receives operator warnings (never content).
	Warn func(msg string)

	// Credentials reports which credential the client will use and fails
	// when there is none. Default CredentialSource.
	Credentials func() (string, error)
	// ClientOptions are appended to the SDK client's options (tests use
	// option.WithBaseURL to point at a fake server).
	ClientOptions []option.RequestOption
	// OnText, if set, receives assistant text as it streams.
	OnText func(string)
	// OnToolResult, if set, receives every tool call with its input and
	// the text sent back to Claude. Unlike the Auditor it carries
	// content (file bodies, answers), so it is for opt-in capture only,
	// never for logs. Called concurrently when a turn runs tools in
	// parallel.
	OnToolResult func(ToolTrace)
}

// ToolTrace is one tool call with its content, for OnToolResult.
type ToolTrace struct {
	Record ToolRecord
	Input  json.RawMessage
	Output string // as sent to Claude, after truncation
}

// Bridge holds the client, the tool set and the cost controls. Safe for
// concurrent use by several Conversations.
type Bridge struct {
	cfg              Config
	client           anthropic.Client
	tools            *toolset
	credentialSource string
}

// New validates cfg, resolves credentials (failing loudly when there are
// none) and lists every provider's tools once.
func New(ctx context.Context, cfg Config) (*Bridge, error) {
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	if cfg.Effort == "" {
		cfg.Effort = DefaultEffort
	}
	switch cfg.Effort {
	case "low", "medium", "high", "xhigh", "max":
	default:
		return nil, fmt.Errorf("claudebridge: effort %q is not one of low, medium, high, xhigh, max", cfg.Effort)
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = DefaultMaxTokens
	}
	if cfg.MaxTokens < 1 || cfg.MaxTokens > 128000 {
		return nil, fmt.Errorf("claudebridge: max_tokens %d out of range 1..128000", cfg.MaxTokens)
	}
	if cfg.MinMaxTokens < 0 || cfg.MinMaxTokens > cfg.MaxTokens {
		return nil, fmt.Errorf("claudebridge: min max_tokens %d out of range 0..%d", cfg.MinMaxTokens, cfg.MaxTokens)
	}
	if cfg.TaskBudget != 0 && cfg.TaskBudget < MinTaskBudget {
		return nil, fmt.Errorf("claudebridge: task budget %d is below the API minimum %d", cfg.TaskBudget, MinTaskBudget)
	}
	if cfg.System == "" {
		cfg.System = DefaultSystem
	}
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = DefaultMaxIterations
	}
	if cfg.MaxToolResultBytes == 0 {
		cfg.MaxToolResultBytes = DefaultMaxToolResultBytes
	}
	if cfg.MaxIterations < 1 || cfg.MaxToolResultBytes < 64 {
		return nil, errors.New("claudebridge: MaxIterations and MaxToolResultBytes must be positive")
	}
	if cfg.Gate == nil {
		return nil, errors.New("claudebridge: a Gate is required; Claude's tool calls are never run ungated")
	}
	if cfg.Ledger == nil || cfg.Prices == nil {
		return nil, errors.New("claudebridge: a Ledger and a PriceTable are required; there is no uncapped mode")
	}
	if cfg.Auditor == nil {
		cfg.Auditor = SlogAuditor{Log: slog.New(slog.NewJSONHandler(io.Discard, nil))}
	}
	if cfg.Credentials == nil {
		cfg.Credentials = CredentialSource
	}
	src, err := cfg.Credentials()
	if err != nil {
		return nil, fmt.Errorf("claudebridge: %w", err)
	}
	ts, err := buildToolset(ctx, cfg.Providers)
	if err != nil {
		return nil, fmt.Errorf("claudebridge: %w", err)
	}
	return &Bridge{
		cfg:              cfg,
		client:           anthropic.NewClient(cfg.ClientOptions...),
		tools:            ts,
		credentialSource: src,
	}, nil
}

// System is the system prompt every request carries.
func (b *Bridge) System() string { return b.cfg.System }

// Model is the requested model ID.
func (b *Bridge) Model() string { return b.cfg.Model }

// CredentialSource names the credential in use (never its value).
func (b *Bridge) CredentialSource() string { return b.credentialSource }

// ToolNames lists the tools offered to Claude, in request order.
func (b *Bridge) ToolNames() []string { return append([]string(nil), b.tools.names...) }

// Conversation is one append-only message history. Not safe for
// concurrent Send calls.
type Conversation struct {
	b        *Bridge
	id       string
	messages []anthropic.BetaMessageParam

	family      string // task family for the audit (S-TWO-BRAINS routing)
	routeReason string // llm_requests.route_reason for every request

	redactions atomic.Int64 // since the last request: prompt + tool results
}

// SetRoute labels every later request of this conversation, in the audit
// records, with a task family and the reason it reached Claude.
func (c *Conversation) SetRoute(family, reason string) {
	c.family, c.routeReason = family, reason
}

// NewConversation starts an empty conversation.
func (b *Bridge) NewConversation() *Conversation {
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	return &Conversation{b: b, id: hex.EncodeToString(raw[:])}
}

// ID identifies the conversation in audit records.
func (c *Conversation) ID() string { return c.id }

// Reply is the outcome of one Send.
type Reply struct {
	Text       string // assistant text of the last response
	StopReason string
	ServedBy   string
	Requests   int
	Cost       NanoUSD
	// Token totals over the turn's requests.
	InputTokens, OutputTokens int64
}

// Send adds prompt as a user turn and runs the tool loop until Claude
// ends its turn. On any error the turn is rolled back, so the history
// stays a valid, append-only conversation for the next Send.
func (c *Conversation) Send(ctx context.Context, prompt string) (*Reply, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("claudebridge: empty prompt")
	}
	if f := c.b.cfg.Privacy; f != nil {
		red, counts, err := f.Prompt(ctx, prompt)
		if err != nil {
			return nil, fmt.Errorf("claudebridge: privacy filter refused the prompt (nothing sent): %w", err)
		}
		if n := counts.Total(); n > 0 {
			c.redactions.Add(int64(n))
			if c.b.cfg.Warn != nil {
				c.b.cfg.Warn(fmt.Sprintf("redacted %d value(s) from the prompt before sending (%s)", n, counts))
			}
		}
		prompt = red
	}
	start := len(c.messages)
	c.messages = append(c.messages, anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(prompt)))
	reply, err := c.loop(ctx)
	if err != nil {
		c.messages = c.messages[:start]
	}
	return reply, err
}

// RequestJSON returns the body the next request would send for prompt,
// without sending it or changing the conversation (dry run).
func (c *Conversation) RequestJSON(prompt string) ([]byte, error) {
	msgs := append(append([]anthropic.BetaMessageParam(nil), c.messages...),
		anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(prompt)))
	params := c.b.params(msgs)
	body, err := json.MarshalIndent(params, "", "  ")
	if err != nil {
		return nil, err
	}
	return body, nil
}

// Betas returns the beta headers every request carries.
func (b *Bridge) Betas() []string {
	var out []string
	for _, x := range b.betas() {
		out = append(out, string(x))
	}
	return out
}

func (b *Bridge) betas() []anthropic.AnthropicBeta {
	var betas []anthropic.AnthropicBeta
	if !b.cfg.DisableFallbacks {
		betas = append(betas, anthropic.AnthropicBetaServerSideFallback2026_07_01)
	}
	if b.cfg.TaskBudget > 0 {
		betas = append(betas, anthropic.AnthropicBetaTaskBudgets2026_03_13)
	}
	return betas
}

func (b *Bridge) params(msgs []anthropic.BetaMessageParam) anthropic.BetaMessageNewParams {
	p := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(b.cfg.Model),
		MaxTokens: b.cfg.MaxTokens,
		Messages:  msgs,
		System:    []anthropic.BetaTextBlockParam{{Text: b.cfg.System}},
		// Adaptive thinking; on Claude Opus 5.5 it cannot be disabled and
		// effort is the control.
		Thinking:     anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{}},
		OutputConfig: anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffort(b.cfg.Effort)},
		Tools:        b.tools.params,
		// Automatic caching of the stable prefix (tools, system, history):
		// the tool loop resends it on every iteration.
		CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
		Betas:        b.betas(),
	}
	if b.cfg.TaskBudget > 0 {
		p.OutputConfig.TaskBudget = anthropic.BetaTokenTaskBudgetParam{Total: b.cfg.TaskBudget}
	}
	if !b.cfg.DisableFallbacks {
		p.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()}
	}
	return p
}

// reservation is the worst-case cost of sending params: the input
// estimated generously from the body size (about two bytes per token,
// against roughly four for English text) at the input or cache-write
// rate, plus params' max_tokens at the output rate; doubled when a refusal
// fallback may run the request a second time. Rates are the routed
// model's when no fallback can hand the request to another model, else
// the table's highest.
func (b *Bridge) reservation(params anthropic.BetaMessageNewParams) (NanoUSD, error) {
	estIn, err := promptEstimate(params)
	if err != nil {
		return 0, err
	}
	inRate, outRate, hops := b.requestRates()
	return WorstCase(estIn, inRate, params.MaxTokens, outRate, hops), nil
}

// promptEstimate is the generous input-token estimate for params.
func promptEstimate(params anthropic.BetaMessageNewParams) (int64, error) {
	body, err := json.Marshal(params)
	if err != nil {
		return 0, fmt.Errorf("encode request: %w", err)
	}
	return int64(len(body))/2 + 1, nil
}

// requestRates are the rates a request's worst case is priced at, and how
// many times it may be billed.
func (b *Bridge) requestRates() (inRate, outRate, hops int64) {
	p := b.cfg.Prices.Max()
	hops = 2
	if b.cfg.DisableFallbacks {
		// Only the routed model can serve (and bill) the request; a
		// model missing from the table is priced at the maximum.
		p, _ = b.cfg.Prices.Lookup(b.cfg.Model)
		hops = 1
	}
	return max(p.Input, p.CacheWrite5m), p.Output, hops
}

// fitToLedger lowers params' max_tokens to what the ledger has left when
// MinMaxTokens allows it and the worst case at the configured max_tokens
// does not fit. It returns the (possibly unchanged) worst case. A
// shrunken request never prices above what it found left: max_tokens only
// gets shorter, so the body (and its prompt estimate) does not grow.
func (b *Bridge) fitToLedger(params *anthropic.BetaMessageNewParams, need NanoUSD) (NanoUSD, error) {
	if b.cfg.MinMaxTokens <= 0 {
		return need, nil
	}
	left, err := b.cfg.Ledger.Remaining()
	if err != nil || need <= left {
		return need, nil // Reserve reports a ledger error itself
	}
	estIn, err := promptEstimate(*params)
	if err != nil {
		return 0, err
	}
	inRate, outRate, hops := b.requestRates()
	n := FitMaxTokens(left, estIn, inRate, outRate, hops, params.MaxTokens, b.cfg.MinMaxTokens)
	if n == 0 {
		return need, nil // Reserve refuses with the full worst case
	}
	params.MaxTokens = n
	return b.reservation(*params)
}

func (c *Conversation) loop(ctx context.Context) (*Reply, error) {
	reply := &Reply{}
	for iter := 1; iter <= c.b.cfg.MaxIterations; iter++ {
		msg, err := c.request(ctx, iter, reply)
		if err != nil {
			return reply, err
		}
		reply.StopReason = string(msg.StopReason)
		reply.ServedBy = string(msg.Model)
		reply.Text = assistantText(msg)

		switch msg.StopReason {
		case anthropic.BetaStopReasonRefusal:
			// Discard any partial output; do not run tools from it.
			return reply, &RefusalError{
				Category:    string(msg.StopDetails.Category),
				Explanation: msg.StopDetails.Explanation,
				ServedBy:    string(msg.Model),
			}
		case anthropic.BetaStopReasonMaxTokens:
			return reply, ErrMaxTokens
		case anthropic.BetaStopReasonModelContextWindowExceeded:
			return reply, ErrContextWindow
		case anthropic.BetaStopReasonPauseTurn:
			// A server-side loop paused; resend as is and it resumes.
			c.messages = append(c.messages, msg.ToParam())
			continue
		case anthropic.BetaStopReasonToolUse:
			c.messages = append(c.messages, msg.ToParam())
			results := c.runTools(ctx, msg)
			if len(results) == 0 {
				return reply, errors.New("claudebridge: stop_reason tool_use with no tool_use blocks")
			}
			// All results for the turn go back in one user message.
			c.messages = append(c.messages, anthropic.NewBetaUserMessage(results...))
			continue
		default: // end_turn, stop_sequence
			c.messages = append(c.messages, msg.ToParam())
			return reply, nil
		}
	}
	return reply, ErrIterationLimit
}

// request sends one streamed request, holding its worst-case cost
// against the daily cap first and recording the real cost after.
func (c *Conversation) request(ctx context.Context, iter int, reply *Reply) (*anthropic.BetaMessage, error) {
	b := c.b
	params := b.params(c.messages)
	rec := RequestRecord{Conversation: c.id, Iteration: iter, Model: b.cfg.Model,
		Backend: BackendClaude, TaskFamily: c.family, RouteReason: c.routeReason,
		Redactions: int(c.redactions.Swap(0))}

	need, err := b.reservation(params)
	if err != nil {
		return nil, err
	}
	if need, err = b.fitToLedger(&params, need); err != nil {
		return nil, err
	}
	rec.Reserved = need
	res, err := b.cfg.Ledger.Reserve(need)
	if err != nil {
		rec.Err = err.Error()
		b.cfg.Auditor.Request(ctx, rec)
		return nil, err
	}

	t0 := time.Now()
	msg, streamErr := b.stream(ctx, params)
	rec.Duration = time.Since(t0)
	rec.Sent = true

	var cost NanoUSD
	switch {
	case streamErr == nil:
		var unknown []string
		cost, unknown = b.cfg.Prices.UsageCost(string(msg.Model), msg.Usage)
		rec.UnpricedModels = unknown
	case msg.ID != "":
		// The stream broke after message_start: input, and maybe some
		// output, is billed but usage is incomplete. Count the worst case.
		cost = need
	}
	day, commitErr := res.Commit(cost)
	rec.Cost, rec.DaySpent = cost, day
	reply.Cost += cost
	reply.Requests++
	reply.InputTokens += msg.Usage.InputTokens + msg.Usage.CacheReadInputTokens + msg.Usage.CacheCreationInputTokens
	reply.OutputTokens += msg.Usage.OutputTokens

	rec.MessageID = msg.ID
	rec.ServedBy = string(msg.Model)
	rec.StopReason = string(msg.StopReason)
	if msg.StopReason == anthropic.BetaStopReasonRefusal {
		rec.RefusalCategory = string(msg.StopDetails.Category)
	}
	rec.InputTokens = msg.Usage.InputTokens
	rec.OutputTokens = msg.Usage.OutputTokens
	rec.CacheRead = msg.Usage.CacheReadInputTokens
	rec.CacheWrite = msg.Usage.CacheCreationInputTokens
	if streamErr != nil {
		rec.Err = streamErr.Error()
	} else if commitErr != nil {
		rec.Err = commitErr.Error()
	}
	b.cfg.Auditor.Request(ctx, rec)

	if streamErr != nil {
		return nil, fmt.Errorf("claudebridge: request: %w", streamErr)
	}
	if commitErr != nil {
		// The cap can no longer be trusted; stop rather than run on.
		return nil, fmt.Errorf("claudebridge: record spend: %w", commitErr)
	}
	return msg, nil
}

func (b *Bridge) stream(ctx context.Context, params anthropic.BetaMessageNewParams) (*anthropic.BetaMessage, error) {
	s := b.client.Beta.Messages.NewStreaming(ctx, params)
	defer func() { _ = s.Close() }()
	msg := &anthropic.BetaMessage{}
	for s.Next() {
		ev := s.Current()
		if err := msg.Accumulate(ev); err != nil {
			return msg, fmt.Errorf("accumulate stream: %w", err)
		}
		if b.cfg.OnText != nil && ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta" {
			b.cfg.OnText(ev.Delta.Text)
		}
	}
	if err := s.Err(); err != nil {
		return msg, err
	}
	if msg.ID == "" || msg.StopReason == "" {
		return msg, errors.New("stream ended without a complete message")
	}
	return msg, nil
}

func assistantText(msg *anthropic.BetaMessage) string {
	var sb strings.Builder
	for _, block := range msg.Content {
		if t, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return sb.String()
}

// runTools runs every tool_use block in msg concurrently, each through
// the gate, and returns one tool_result per call in the order Claude
// asked. A failure is an is_error result, never a dropped one. Calls the
// gate refused pending confirmation are then put to the Confirmer one at
// a time, in the order Claude asked, so the human sees one prompt after
// another.
func (c *Conversation) runTools(ctx context.Context, msg *anthropic.BetaMessage) []anthropic.BetaContentBlockParamUnion {
	var uses []anthropic.BetaContentBlockUnion
	for _, block := range msg.Content {
		if block.Type == "tool_use" {
			uses = append(uses, block)
		}
	}
	results := make([]anthropic.BetaContentBlockParamUnion, len(uses))
	pending := make([]*toolRun, len(uses))
	var wg sync.WaitGroup
	for i, u := range uses {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], pending[i] = c.runTool(ctx, u.ID, u.Name, u.Input, string(msg.Model))
		}()
	}
	wg.Wait()
	for i, run := range pending {
		if run != nil {
			results[i] = c.confirm(ctx, run)
		}
	}
	return results
}

// toolRun is one tool call in flight: its audit record and what the
// Confirmer needs if the gate refused it pending confirmation.
type toolRun struct {
	b       *Bridge
	rec     ToolRecord
	t0      time.Time
	id      string
	input   json.RawMessage
	owner   owner
	refused *RefusedError
	model   string
}

func (r *toolRun) finish(ctx context.Context, text string, isError bool) anthropic.BetaContentBlockParamUnion {
	text, r.rec.Truncated = truncateUTF8(text, r.b.cfg.MaxToolResultBytes)
	if text == "" {
		text = "(no output)"
	}
	r.rec.IsError = isError
	r.rec.OutputBytes = len(text)
	r.rec.Duration = time.Since(r.t0)
	r.b.cfg.Auditor.ToolCall(ctx, r.rec)
	if r.b.cfg.OnToolResult != nil {
		r.b.cfg.OnToolResult(ToolTrace{Record: r.rec, Input: r.input, Output: text})
	}
	return anthropic.NewBetaToolResultBlock(r.id, text, isError)
}

// confirmable reports whether a refusal may go to the Confirmer: a
// Confirmer other than DenyConfirmer is set, and the tool is one of the
// Champion's own (MCP mutating tools stay refused).
func (b *Bridge) confirmable(p zhentools.Provider) bool {
	if b.cfg.Confirmer == nil {
		return false
	}
	if _, deny := b.cfg.Confirmer.(DenyConfirmer); deny {
		return false
	}
	_, ok := p.(*champion.ToolProvider)
	return ok
}

// runTool gates and runs one call. It returns either the tool_result or,
// for a refusal that can be confirmed out of band, a toolRun for confirm.
func (c *Conversation) runTool(ctx context.Context, id, name string, input json.RawMessage, model string) (anthropic.BetaContentBlockParamUnion, *toolRun) {
	b := c.b
	sum := sha256.Sum256(input)
	run := &toolRun{b: b, t0: time.Now(), id: id, input: input, model: model, rec: ToolRecord{
		Backend:      BackendClaude, // the bridge only ever runs Claude's tool calls
		Conversation: c.id,
		ToolUseID:    id,
		Name:         name,
		InputBytes:   len(input),
		InputSHA256:  hex.EncodeToString(sum[:]),
		Model:        model,
	}}

	own, ok := b.tools.owners[name]
	if !ok {
		run.rec.Decision, run.rec.Reason = "unknown_tool", "no provider offers this tool"
		return run.finish(ctx, fmt.Sprintf("unknown tool %q", name), true), nil
	}
	run.owner = own
	run.rec.Provider = own.provider
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(input, &obj); err != nil || obj == nil {
		run.rec.Decision, run.rec.Reason = "bad_input", "input is not a JSON object"
		return run.finish(ctx, "tool input must be a JSON object", true), nil
	}

	res, err := b.cfg.Gate.Execute(ctx, ToolCall{
		ID:        id,
		Name:      name,
		Input:     input,
		Provider:  own.provider,
		EmittedBy: "claude-api/" + model + "/" + c.id,
	}, own.p)
	var refused *RefusedError
	switch {
	case errors.As(err, &refused):
		run.rec.Decision, run.rec.Reason, run.rec.Pending = "refused", refused.Reason, refused.PendingConfirmation
		if refused.PendingConfirmation && b.confirmable(own.p) {
			run.refused = refused
			return anthropic.BetaContentBlockParamUnion{}, run
		}
		return run.finish(ctx, refused.Error(), true), nil
	case err != nil:
		run.rec.Decision, run.rec.Reason = "error", firstLine(err.Error())
		text, isErr := c.filter(ctx, run, "tool failed: "+err.Error(), true)
		return run.finish(ctx, text, isErr), nil
	case res.IsError:
		run.rec.Decision, run.rec.Reason = "error", firstLine(res.Text)
		text, isErr := c.filter(ctx, run, res.Text, true)
		return run.finish(ctx, text, isErr), nil
	default:
		run.rec.Decision = "allowed"
		text, isErr := c.filter(ctx, run, res.Text, false)
		return run.finish(ctx, text, isErr), nil
	}
}

// confirm puts a refused-pending call to the Confirmer and records each
// step. The text Claude gets never carries a code or a token.
func (c *Conversation) confirm(ctx context.Context, run *toolRun) anthropic.BetaContentBlockParamUnion {
	var args map[string]any
	if err := json.Unmarshal(run.input, &args); err != nil {
		run.rec.Decision = "bad_input"
		return run.finish(ctx, "tool input must be a JSON object", true)
	}
	req := ConfirmRequest{
		Conversation: c.id, ToolUseID: run.id, Tool: run.rec.Name, Args: args,
		EmittedBy: "claude-api/" + run.model + "/" + c.id, Model: run.model,
		Reason: run.refused.Reason, Provider: run.owner.p,
	}
	out := c.b.cfg.Confirmer.Confirm(ctx, req)
	c.auditConfirm(ctx, run, out)
	run.rec.Decision = out.Decision
	run.rec.Pending = out.Decision == "queued"
	if out.Decision != "approved" {
		run.rec.Reason = firstLine(out.Text)
	}
	text := out.Text
	if text == "" {
		text = run.refused.Error()
	}
	isErr := out.IsError
	if out.Decision == "approved" {
		text, isErr = c.filter(ctx, run, text, isErr)
	}
	return run.finish(ctx, text, isErr)
}

// filter passes a tool's output through the privacy tier. Whatever goes
// wrong, Claude gets an is_error saying the result was withheld, never
// the raw text.
func (c *Conversation) filter(ctx context.Context, run *toolRun, text string, isErr bool) (string, bool) {
	f := c.b.cfg.Privacy
	if f == nil {
		return text, isErr
	}
	var args map[string]any
	_ = json.Unmarshal(run.input, &args)
	out, o, err := f.ToolResult(ctx, run.rec.Name, args, c.b.cfg.ProjectRoot, text)
	run.rec.Tier, run.rec.PrivacyMode = o.Tier.String(), string(o.Mode)
	n := o.Counts.Total()
	run.rec.Redactions = n
	c.redactions.Add(int64(n))
	switch {
	case errors.Is(err, privacy.ErrWithheld):
		run.rec.Decision, run.rec.Reason = "withheld", "user data"
		return "withheld: user data (zero user data access); this result never leaves the host", true
	case errors.Is(err, privacy.ErrStrictWithheld):
		run.rec.Decision, run.rec.Reason = "withheld", "privacy mode strict"
		return "withheld: Kingdom ops data is not sent in privacy mode strict", true
	case err != nil:
		run.rec.Decision, run.rec.Reason = "withheld", "privacy filter failed"
		return "withheld: the privacy filter could not check this result, so it was not sent", true
	}
	if o.Summarised || o.SummaryUnavailable {
		run.rec.Truncated = true
	}
	return out, isErr
}

func (c *Conversation) auditConfirm(ctx context.Context, run *toolRun, out ConfirmOutcome) {
	ca, ok := c.b.cfg.Auditor.(ConfirmAuditor)
	if !ok {
		return
	}
	base := ConfirmRecord{Conversation: c.id, Model: run.model, Tool: run.rec.Name, QueueID: out.QueueID, CallSHA256: out.SHA256}
	switch out.Decision {
	case "queued":
		base.Action = "confirm_issue"
		ca.Confirmation(ctx, base)
	case "approved", "denied", "failed":
		issue := base
		issue.Action = "confirm_issue"
		ca.Confirmation(ctx, issue)
		base.Action = "confirm_deny"
		if out.Decision == "approved" {
			base.Action = "confirm_approve"
		}
		ca.Confirmation(ctx, base)
	}
}

// truncateUTF8 cuts s to at most n bytes on a rune boundary, marking the cut.
func truncateUTF8(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	const marker = "\n[truncated by claudebridge]"
	cut := n - len(marker)
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + marker, true
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s, _ = truncateUTF8(s, 200)
	}
	return s
}
