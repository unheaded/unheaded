// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package claudebridge

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"unheaded/pkg/champion"
	"unheaded/pkg/zhencapture"
	"unheaded/pkg/zhentools"
)

// Out-of-band confirmation (S-TWO-BRAINS Phase 2, ADR-104 open question
// 2). When the Champion gate refuses one of its own mutating tools pending
// confirmation (Rule 2), a Confirmer may put the call to the human through
// a channel the model cannot see or answer.
//
// The rule: no confirmation token, code or approval handle ever goes into
// model-visible content (tool_result, system prompt, user turn). The TTY
// code is printed on /dev/tty only; the queue id may appear (it
// authorises nothing without Stevie's own login to the Well).

// Topic for queued confirmations (id, tool and summary only).
const TopicConfirmPending = "zhen.confirm.pending"

// MaxPendingPerService caps pending queue rows per source service
// (migration 031 enforces the same number).
const MaxPendingPerService = 20

// DefaultQueueTTL is how long a queued call waits for a decision.
const DefaultQueueTTL = 24 * time.Hour

// ConfirmRequest is a refused call offered for confirmation.
type ConfirmRequest struct {
	Conversation string
	ToolUseID    string
	Tool         string
	Args         map[string]any
	EmittedBy    string
	Model        string
	Reason       string             // the gate's refusal
	Provider     zhentools.Provider // must be the Champion's own ToolProvider
}

// ConfirmOutcome is what happened, and the text Claude receives.
type ConfirmOutcome struct {
	Decision string // approved, denied, queued, failed, unconfirmable
	Text     string // tool_result text; never a code or token
	IsError  bool
	QueueID  string
	SHA256   string
}

// Confirmer decides refused-pending calls out of band.
type Confirmer interface {
	Confirm(ctx context.Context, req ConfirmRequest) ConfirmOutcome
}

// DenyConfirmer is the default: nothing is confirmable, the refusal goes
// back to Claude unchanged (the bridge's behaviour before Phase 2).
type DenyConfirmer struct{}

// Confirm implements Confirmer.
func (DenyConfirmer) Confirm(context.Context, ConfirmRequest) ConfirmOutcome {
	return ConfirmOutcome{Decision: "denied", Text: "refused: needs out-of-band confirmation, and this session confirms nothing", IsError: true}
}

// ConfirmRecord is the audit trail of one confirmation step.
type ConfirmRecord struct {
	Conversation string
	Model        string
	Tool         string
	Action       string // confirm_issue, confirm_approve, confirm_deny, confirm_expire
	QueueID      string
	CallSHA256   string
	DecidedBy    string // the human, for approve/deny
	Source       string // zhen-claude, zhen-agentd
}

// ConfirmAuditor is implemented by auditors that also record
// confirmation steps (WellAuditor, WotanAuditor, SlogAuditor).
type ConfirmAuditor interface {
	Confirmation(ctx context.Context, r ConfirmRecord)
}

func championOf(p zhentools.Provider) *champion.Champion {
	if cp, ok := p.(*champion.ToolProvider); ok {
		return cp.Champion()
	}
	return nil
}

func unconfirmable(why string) ConfirmOutcome {
	return ConfirmOutcome{Decision: "unconfirmable", Text: "refused: " + why, IsError: true}
}

// TTYConfirmer asks the human at the controlling terminal. It shows the
// tool, the sandbox root, the arguments (secrets redacted) and, for file
// writes, a diff; the human must type a 4-character code printed only on
// the terminal. On approval it runs the call in-process through
// Champion.ConfirmCall; nothing leaves process memory.
type TTYConfirmer struct {
	Champion   *champion.Champion
	TTY        io.ReadWriter // /dev/tty; never stdin
	ApprovedBy string        // default $USER@host
	Timeout    time.Duration // per prompt; default champion.PendingConfirmTTL
	mu         sync.Mutex
}

// Confirm implements Confirmer.
func (t *TTYConfirmer) Confirm(ctx context.Context, req ConfirmRequest) ConfirmOutcome {
	if t == nil || t.Champion == nil || t.TTY == nil {
		return unconfirmable("no terminal to confirm on")
	}
	if championOf(req.Provider) != t.Champion {
		return unconfirmable("only the Champion's own tools can be confirmed")
	}
	timeout := t.Timeout
	if timeout <= 0 {
		timeout = champion.PendingConfirmTTL
	}
	by := t.ApprovedBy
	if by == "" {
		by = operatorName()
	}
	// The model acts for the human at this terminal: the call is bound
	// to tty:<operator>, and only that principal confirms it (RT-03).
	principal := "tty:" + by
	p, err := t.Champion.PendingCallFor(champion.ToolCall{Name: req.Tool, Args: req.Args, EmittedBy: req.EmittedBy, Principal: principal}, timeout)
	if err != nil {
		return unconfirmable(err.Error())
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	code, err := approvalCode()
	if err != nil {
		return ConfirmOutcome{Decision: "failed", Text: "confirmation unavailable", IsError: true}
	}
	if _, err := io.WriteString(t.TTY, renderPrompt(t.Champion, req, p, code)); err != nil {
		return ConfirmOutcome{Decision: "failed", Text: "confirmation unavailable: cannot write to the terminal", IsError: true}
	}
	line, err := readLine(ctx, t.TTY, timeout)
	if err != nil || strings.TrimSpace(line) != code {
		_, _ = io.WriteString(t.TTY, "denied.\n")
		return ConfirmOutcome{Decision: "denied", Text: "denied by the human operator; do not retry this call", IsError: true, SHA256: p.SHA256}
	}
	out, err := t.Champion.ConfirmCall(ctx, p, p.SHA256, principal)
	if err != nil {
		_, _ = io.WriteString(t.TTY, "approved, but the call failed: "+err.Error()+"\n")
		return ConfirmOutcome{Decision: "failed", Text: "approved by the human operator, but the call failed: " + err.Error(), IsError: true, SHA256: p.SHA256}
	}
	_, _ = io.WriteString(t.TTY, "approved; done.\n")
	text := "approved by the human operator; done"
	if s, ok := out.(string); ok && s != "" {
		text += ": " + s
	}
	return ConfirmOutcome{Decision: "approved", Text: text, SHA256: p.SHA256}
}

// codeAlphabet leaves out look-alikes (0/o, 1/l/i).
const codeAlphabet = "abcdefghjkmnpqrstuvwxyz23456789"

func approvalCode() (string, error) {
	var sb strings.Builder
	for i := 0; i < 4; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(codeAlphabet))))
		if err != nil {
			return "", err
		}
		sb.WriteByte(codeAlphabet[n.Int64()])
	}
	return sb.String(), nil
}

// readLine reads one line from r, giving up at the timeout or when ctx
// ends. A goroutine left blocked on a terminal read ends with the process.
func readLine(ctx context.Context, r io.Reader, timeout time.Duration) (string, error) {
	type res struct {
		s   string
		err error
	}
	ch := make(chan res, 1)
	go func() {
		s, err := bufio.NewReaderSize(r, 256).ReadString('\n')
		if err != nil && s == "" {
			ch <- res{"", err}
			return
		}
		ch <- res{s, nil}
	}()
	select {
	case v := <-ch:
		return v.s, v.err
	case <-time.After(timeout):
		return "", errors.New("no answer in time")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func operatorName() string {
	u := os.Getenv("USER")
	if u == "" {
		u = "unknown"
	}
	h, err := os.Hostname()
	if err != nil || h == "" {
		h = "localhost"
	}
	if i := strings.IndexByte(h, '.'); i > 0 {
		h = h[:i]
	}
	return u + "@" + h
}

// renderPrompt is what the human sees. It is written to the terminal
// only. Argument values are redacted; long ones are shown by size.
func renderPrompt(c *champion.Champion, req ConfirmRequest, p champion.PendingCall, code string) string {
	var sb strings.Builder
	sb.WriteString("\n=== zhen: a model asks to run a mutating tool ===\n")
	p.EmittedBy = req.EmittedBy
	sb.WriteString(DescribeCall(c, p))
	fmt.Fprintf(&sb, "type %s to approve, anything else denies: ", code)
	return sb.String()
}

// DescribeCall is the human-readable view of a pending call: tool,
// sandbox, who asked, hash, arguments with secrets redacted and, for file
// writes inside the sandbox, a diff against the file as it is now.
func DescribeCall(c *champion.Champion, p champion.PendingCall) string {
	var sb strings.Builder
	if p.SHA256 == "" {
		_, p.SHA256, _ = champion.CanonicalCallFor(p.Tool, p.Args, p.ProjectRoot, p.Requester)
	}
	fmt.Fprintf(&sb, "tool:     %s (Champion)\n", p.Tool)
	fmt.Fprintf(&sb, "sandbox:  %s\n", p.ProjectRoot)
	fmt.Fprintf(&sb, "asked by: %s\n", safeLine(p.EmittedBy))
	fmt.Fprintf(&sb, "sha256:   %s\n", p.SHA256)
	sb.WriteString("args (secrets redacted):\n")
	keys := make([]string, 0, len(p.Args))
	for k := range p.Args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&sb, "  %s: %s\n", safeLine(k), argPreview(p.Args[k]))
	}
	if d := proposedDiff(c, p); d != "" {
		sb.WriteString(d)
	}
	return sb.String()
}

func argPreview(v any) string {
	var s string
	switch x := v.(type) {
	case string:
		s = x
	default:
		b, _ := json.Marshal(x)
		s = string(b)
	}
	s, _ = zhencapture.Redact(s)
	if len(s) > 200 || strings.ContainsAny(s, "\n\r") {
		return fmt.Sprintf("(%d bytes; see the diff)", len(s))
	}
	return safeLine(s)
}

// safeLine drops control characters so nothing can rewrite the terminal.
func safeLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r < 0xa0) {
			return '?'
		}
		return r
	}, s)
}

const maxDiffLines = 400

// proposedDiff shows what write_file / patch_file would change, for a
// path inside the sandbox only; secrets in it are redacted.
func proposedDiff(c *champion.Champion, p champion.PendingCall) string {
	path, _ := p.Args["path"].(string)
	if path == "" || (p.Tool != "write_file" && p.Tool != "patch_file") {
		return ""
	}
	if err := c.CheckPath(path); err != nil {
		return "diff: not shown (" + safeLine(err.Error()) + ")\n"
	}
	cur, err := os.ReadFile(path) // #nosec G304 -- path passed the Champion's sandbox check just above
	if err != nil && !os.IsNotExist(err) {
		return "diff: current file unreadable\n"
	}
	var next string
	switch p.Tool {
	case "write_file":
		next, _ = p.Args["content"].(string)
	case "patch_file":
		oldText, _ := p.Args["old_text"].(string)
		newText, _ := p.Args["new_text"].(string)
		if oldText == "" || !strings.Contains(string(cur), oldText) {
			return "diff: old_text not found in the current file (the patch will fail)\n"
		}
		next = strings.Replace(string(cur), oldText, newText, 1)
	}
	a, _ := zhencapture.Redact(string(cur))
	b, _ := zhencapture.Redact(next)
	return "diff:\n--- " + safeLine(path) + " (current)\n+++ " + safeLine(path) + " (proposed)\n" + lineDiff(splitLines(a), splitLines(b))
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// lineDiff is a plain LCS line diff ("-", "+", " " prefixes), bounded.
func lineDiff(a, b []string) string {
	if len(a) > maxDiffLines || len(b) > maxDiffLines {
		return fmt.Sprintf("(too long to diff here: %d -> %d lines)\n", len(a), len(b))
	}
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var sb strings.Builder
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && a[i] == b[j]:
			sb.WriteString(" " + safeLine(a[i]) + "\n")
			i++
			j++
		case j < m && (i == n || lcs[i][j+1] >= lcs[i+1][j]):
			sb.WriteString("+" + safeLine(b[j]) + "\n")
			j++
		default:
			sb.WriteString("-" + safeLine(a[i]) + "\n")
			i++
		}
	}
	return sb.String()
}

// QueuedCall is one zhen_pending_confirmations row (migration 031).
type QueuedCall struct {
	ID            string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	SourceService string
	EmittedBy     string
	ProjectRoot   string
	Tool          string
	Call          json.RawMessage // the args, as hashed
	CallSHA256    string
	Summary       string
	CardID        string
	Status        string
	DecidedAt     *time.Time
	DecidedBy     string
}

// ConfirmQueue stores queued calls (the Well in production).
type ConfirmQueue interface {
	Enqueue(ctx context.Context, c QueuedCall) error
	CountPending(ctx context.Context, source string) (int, error)
}

// JSONPublisher publishes small JSON notices (WotanAuditor implements it).
type JSONPublisher interface {
	PublishJSON(topic string, v any)
}

// QueueConfirmer queues the call in the Well and returns at once; Claude
// is told it is queued. Stevie approves it later with
// `zhen-claude confirm approve <id>`, a separate process under his login.
type QueueConfirmer struct {
	Champion *champion.Champion
	Queue    ConfirmQueue
	Pub      JSONPublisher // optional: zhen.confirm.pending
	Source   string        // zhen-claude or zhen-agentd
	TTL      time.Duration // default DefaultQueueTTL
	CardID   string
}

// Confirm implements Confirmer.
func (q *QueueConfirmer) Confirm(ctx context.Context, req ConfirmRequest) ConfirmOutcome {
	if q == nil || q.Champion == nil || q.Queue == nil {
		return unconfirmable("no confirmation queue")
	}
	if q.Source != "zhen-claude" && q.Source != "zhen-agentd" {
		return unconfirmable("queue source must be zhen-claude or zhen-agentd")
	}
	if championOf(req.Provider) != q.Champion {
		return unconfirmable("only the Champion's own tools can be confirmed")
	}
	ttl := q.TTL
	if ttl <= 0 {
		ttl = DefaultQueueTTL
	}
	// No requester is bound: the queue row has no column for one, and a
	// requester-less call is confirmable only by champion.QueuePrincipal
	// (`zhen-claude confirm approve`), never by a ui:/tty: principal.
	p, err := q.Champion.PendingCallFor(champion.ToolCall{Name: req.Tool, Args: req.Args, EmittedBy: req.EmittedBy}, ttl)
	if err != nil {
		return unconfirmable(err.Error())
	}
	n, err := q.Queue.CountPending(ctx, q.Source)
	if err != nil {
		return ConfirmOutcome{Decision: "failed", Text: "refused: the confirmation queue is unavailable", IsError: true}
	}
	if n >= MaxPendingPerService {
		return ConfirmOutcome{Decision: "failed", Text: fmt.Sprintf("refused: the confirmation queue is full (%d pending); stop and report what you wanted to do", n), IsError: true}
	}
	args, err := json.Marshal(p.Args)
	if err != nil {
		return ConfirmOutcome{Decision: "failed", Text: "refused: cannot encode the call", IsError: true}
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return ConfirmOutcome{Decision: "failed", Text: "confirmation unavailable", IsError: true}
	}
	id := hex.EncodeToString(raw[:])
	now := time.Now().UTC()
	qc := QueuedCall{
		ID: id, CreatedAt: now, ExpiresAt: now.Add(ttl), SourceService: q.Source,
		EmittedBy: truncateUTF8Plain(req.EmittedBy, 256), ProjectRoot: p.ProjectRoot, Tool: p.Tool,
		Call: args, CallSHA256: p.SHA256, Summary: callSummary(p), CardID: q.CardID, Status: "pending",
	}
	if err := q.Queue.Enqueue(ctx, qc); err != nil {
		return ConfirmOutcome{Decision: "failed", Text: "refused: the call could not be queued for approval", IsError: true}
	}
	if q.Pub != nil {
		q.Pub.PublishJSON(TopicConfirmPending, map[string]any{
			"id": id, "tool": p.Tool, "summary": qc.Summary, "source": q.Source,
			"expires_at": qc.ExpiresAt.Format(time.RFC3339),
		})
	}
	return ConfirmOutcome{
		Decision: "queued", QueueID: id, SHA256: p.SHA256, IsError: true,
		Text: "queued for human approval as " + id + "; it runs only if the operator approves it out of band. Do not retry it; continue without it or stop.",
	}
}

// callSummary is a one-line description with no file body.
func callSummary(p champion.PendingCall) string {
	var s string
	switch p.Tool {
	case "write_file":
		path, _ := p.Args["path"].(string)
		content, _ := p.Args["content"].(string)
		s = fmt.Sprintf("write_file %s (%d bytes)", path, len(content))
	case "patch_file":
		path, _ := p.Args["path"].(string)
		s = "patch_file " + path
	case "kanban_update":
		id, _ := p.Args["id"].(string)
		s = "kanban_update " + id
	case "kanban_create":
		title := ""
		if t, ok := p.Args["task"].(map[string]any); ok {
			title, _ = t["title"].(string)
		}
		s = "kanban_create " + title
	default:
		s = p.Tool
	}
	s, _ = zhencapture.Redact(safeLine(s))
	return truncateUTF8Plain(s, 512)
}
