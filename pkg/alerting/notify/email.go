// Package notify provides email notification channel implementation.
package notify

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"html/template"
	"net"
	"net/smtp"
	"strings"
	"time"

	"unheaded/pkg/alerting"
)

// EmailConfig holds email channel configuration.
type EmailConfig struct {
	ChannelConfig
	SMTPHost       string   `json:"smtp_host"`
	SMTPPort       int      `json:"smtp_port"`
	From           string   `json:"from"`
	To             []string `json:"to"`
	Username       string   `json:"username"`
	Password       string   `json:"password"`
	RequireTLS     bool     `json:"require_tls"`
	TLSInsecure    bool     `json:"tls_insecure"`
	HTML           bool     `json:"html"`
	SubjectPrefix  string   `json:"subject_prefix"`
}

// EmailChannel implements the NotificationChannel interface for email.
type EmailChannel struct {
	config   EmailConfig
	htmlTmpl *template.Template
	textTmpl *template.Template
}

// NewEmailChannel creates a new email notification channel.
func NewEmailChannel(config EmailConfig) (*EmailChannel, error) {
	if config.SMTPPort == 0 {
		config.SMTPPort = 587
	}
	if config.SubjectPrefix == "" {
		config.SubjectPrefix = "[Unheaded Alert]"
	}

	htmlTmpl, err := template.New("email_html").Funcs(templateFuncs()).Parse(defaultHTMLTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML template: %w", err)
	}

	textTmpl, err := template.New("email_text").Funcs(templateFuncs()).Parse(defaultTextTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse text template: %w", err)
	}

	return &EmailChannel{
		config:   config,
		htmlTmpl: htmlTmpl,
		textTmpl: textTmpl,
	}, nil
}

// Name returns the channel name.
func (e *EmailChannel) Name() string {
	return e.config.Name
}

// Send sends alert notifications via email.
func (e *EmailChannel) Send(ctx context.Context, alerts []*alerting.Alert) error {
	if len(alerts) == 0 {
		return nil
	}

	subject := e.buildSubject(alerts)
	body, err := e.buildBody(alerts)
	if err != nil {
		return fmt.Errorf("%w: failed to build email body: %v", alerting.ErrNotificationError, err)
	}

	msg := e.buildMessage(subject, body)

	if err := e.sendMail(ctx, msg); err != nil {
		return fmt.Errorf("%w: %v", alerting.ErrNotificationError, err)
	}

	return nil
}

// buildSubject builds the email subject line.
func (e *EmailChannel) buildSubject(alerts []*alerting.Alert) string {
	firingCount := 0
	resolvedCount := 0
	var severity alerting.AlertSeverity

	for _, alert := range alerts {
		if alert.State == alerting.AlertStateFiring {
			firingCount++
			if severity == "" || compareSeverity(alert.Severity, severity) > 0 {
				severity = alert.Severity
			}
		} else {
			resolvedCount++
		}
	}

	var status string
	if firingCount > 0 && resolvedCount > 0 {
		status = fmt.Sprintf("FIRING(%d) RESOLVED(%d)", firingCount, resolvedCount)
	} else if firingCount > 0 {
		status = fmt.Sprintf("FIRING(%d)", firingCount)
	} else {
		status = fmt.Sprintf("RESOLVED(%d)", resolvedCount)
	}

	if len(alerts) == 1 {
		return fmt.Sprintf("%s %s: %s", e.config.SubjectPrefix, status, alerts[0].Name)
	}

	return fmt.Sprintf("%s %s: %d alerts", e.config.SubjectPrefix, status, len(alerts))
}

// buildBody builds the email body.
func (e *EmailChannel) buildBody(alerts []*alerting.Alert) (string, error) {
	data := &TemplateData{
		Alerts: alerts,
		Status: getGroupStatus(alerts),
	}

	var buf bytes.Buffer
	var tmpl *template.Template

	if e.config.HTML {
		tmpl = e.htmlTmpl
	} else {
		tmpl = e.textTmpl
	}

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// buildMessage builds the SMTP message.
func (e *EmailChannel) buildMessage(subject, body string) []byte {
	var msg bytes.Buffer

	msg.WriteString(fmt.Sprintf("From: %s\r\n", e.config.From))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", strings.Join(e.config.To, ", ")))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	msg.WriteString("MIME-Version: 1.0\r\n")

	if e.config.HTML {
		msg.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	} else {
		msg.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	}

	msg.WriteString("\r\n")
	msg.WriteString(body)

	return msg.Bytes()
}

// sendMail sends the email via SMTP.
func (e *EmailChannel) sendMail(ctx context.Context, msg []byte) error {
	addr := fmt.Sprintf("%s:%d", e.config.SMTPHost, e.config.SMTPPort)

	var conn net.Conn
	var err error

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err = dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}

	client, err := smtp.NewClient(conn, e.config.SMTPHost)
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	// Start TLS if required
	if e.config.RequireTLS {
		tlsConfig := &tls.Config{
			ServerName:         e.config.SMTPHost,
			InsecureSkipVerify: e.config.TLSInsecure,
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	// Authenticate if credentials are provided
	if e.config.Username != "" && e.config.Password != "" {
		auth := smtp.PlainAuth("", e.config.Username, e.config.Password, e.config.SMTPHost)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Set sender and recipients
	if err := client.Mail(e.config.From); err != nil {
		return fmt.Errorf("failed to set sender: %w", err)
	}

	for _, to := range e.config.To {
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("failed to add recipient %s: %w", to, err)
		}
	}

	// Send the message
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("failed to open data: %w", err)
	}

	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("failed to close data: %w", err)
	}

	return client.Quit()
}

// Test tests the email connection.
func (e *EmailChannel) Test(ctx context.Context) error {
	testAlert := &alerting.Alert{
		ID:          "test-alert",
		Name:        "Test Alert",
		Description: "This is a test notification from the Unheaded Kingdom alerting system.",
		State:       alerting.AlertStateFiring,
		Severity:    alerting.SeverityInfo,
		Labels:      map[string]string{"alertname": "TestAlert", "test": "true"},
		Annotations: map[string]string{"summary": "Test notification"},
		StartsAt:    time.Now(),
		Fingerprint: "test-fingerprint",
	}

	return e.Send(ctx, []*alerting.Alert{testAlert})
}

// compareSeverity compares two severities, returns positive if a > b.
func compareSeverity(a, b alerting.AlertSeverity) int {
	order := map[alerting.AlertSeverity]int{
		alerting.SeverityCritical: 3,
		alerting.SeverityWarning:  2,
		alerting.SeverityInfo:     1,
	}
	return order[a] - order[b]
}

// getGroupStatus determines the overall status of alert group.
func getGroupStatus(alerts []*alerting.Alert) string {
	for _, alert := range alerts {
		if alert.State == alerting.AlertStateFiring {
			return "firing"
		}
	}
	return "resolved"
}

const defaultHTMLTemplate = `<!DOCTYPE html>
<html>
<head>
<style>
body { font-family: Arial, sans-serif; margin: 0; padding: 20px; }
.alert { border: 1px solid #ddd; margin: 10px 0; padding: 15px; border-radius: 4px; }
.alert-critical { border-left: 4px solid #dc3545; }
.alert-warning { border-left: 4px solid #ffc107; }
.alert-info { border-left: 4px solid #17a2b8; }
.alert-resolved { opacity: 0.7; }
.alert-name { font-weight: bold; font-size: 16px; }
.alert-meta { color: #666; font-size: 12px; margin-top: 5px; }
.labels { margin-top: 10px; }
.label { display: inline-block; background: #f0f0f0; padding: 2px 6px; border-radius: 3px; margin: 2px; font-size: 12px; }
</style>
</head>
<body>
<h2>{{ if eq .Status "firing" }}FIRING{{ else }}RESOLVED{{ end }}: {{ len .Alerts }} Alert(s)</h2>
{{ range .Alerts }}
<div class="alert alert-{{ .Severity | toLower }}{{ if eq .State "resolved" }} alert-resolved{{ end }}">
  <div class="alert-name">{{ severityIcon .Severity }} {{ .Name }}</div>
  {{ if .Description }}<p>{{ .Description }}</p>{{ end }}
  <div class="alert-meta">
    <strong>Status:</strong> {{ .State | toUpper }}<br>
    <strong>Value:</strong> {{ .Value }} (threshold: {{ .Threshold }})<br>
    <strong>Started:</strong> {{ formatTime .StartsAt }}<br>
    {{ if eq .State "resolved" }}<strong>Resolved:</strong> {{ formatTime .ResolvedAt }}<br>{{ end }}
  </div>
  <div class="labels">
    {{ range $k, $v := .Labels }}<span class="label">{{ $k }}={{ $v }}</span>{{ end }}
  </div>
</div>
{{ end }}
</body>
</html>`

const defaultTextTemplate = `{{ if eq .Status "firing" }}FIRING{{ else }}RESOLVED{{ end }}: {{ len .Alerts }} Alert(s)
================================================================================
{{ range .Alerts }}
{{ severityIcon .Severity }} {{ .Name }}
{{ if .Description }}Description: {{ .Description }}{{ end }}
Status: {{ .State | toUpper }}
Value: {{ .Value }} (threshold: {{ .Threshold }})
Started: {{ formatTime .StartsAt }}
{{ if eq .State "resolved" }}Resolved: {{ formatTime .ResolvedAt }}{{ end }}
Labels: {{ range $k, $v := .Labels }}{{ $k }}={{ $v }} {{ end }}
--------------------------------------------------------------------------------
{{ end }}`
