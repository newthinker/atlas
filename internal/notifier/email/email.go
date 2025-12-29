// Package email implements an SMTP-based email notifier
package email

import (
	"fmt"
	"net/smtp"
	"strings"
	"time"

	"github.com/newthinker/atlas/internal/core"
	"github.com/newthinker/atlas/internal/notifier"
)

// Email implements the Notifier interface for SMTP email
type Email struct {
	host     string
	port     int
	username string
	password string
	from     string
	to       []string
}

// New creates a new Email notifier
func New(host string, port int, username, password, from string, to []string) *Email {
	return &Email{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		to:       to,
	}
}

func (e *Email) Name() string { return "email" }

func (e *Email) Init(cfg notifier.Config) error {
	if host, ok := cfg.Params["host"].(string); ok {
		e.host = host
	}
	if port, ok := cfg.Params["port"].(int); ok {
		e.port = port
	}
	if username, ok := cfg.Params["username"].(string); ok {
		e.username = username
	}
	if password, ok := cfg.Params["password"].(string); ok {
		e.password = password
	}
	if from, ok := cfg.Params["from"].(string); ok {
		e.from = from
	}
	if to, ok := cfg.Params["to"].([]string); ok {
		e.to = to
	}

	if e.host == "" || e.from == "" || len(e.to) == 0 {
		return fmt.Errorf("email: host, from, and to are required")
	}
	return nil
}

func (e *Email) Send(signal core.Signal) error {
	subject := fmt.Sprintf("ATLAS Signal: %s %s", signal.Symbol, signal.Action)
	body := e.formatSignal(signal)
	return e.sendEmail(subject, body)
}

func (e *Email) SendBatch(signals []core.Signal) error {
	if len(signals) == 0 {
		return nil
	}

	subject := fmt.Sprintf("ATLAS Digest: %d Trading Signals", len(signals))

	var sb strings.Builder
	sb.WriteString("<html><body>")
	sb.WriteString("<h2>ATLAS Trading Signals</h2>")
	sb.WriteString(fmt.Sprintf("<p>Generated at: %s</p>", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("<hr>")

	for _, signal := range signals {
		sb.WriteString(e.formatSignalHTML(signal))
		sb.WriteString("<hr>")
	}

	sb.WriteString("</body></html>")

	return e.sendEmail(subject, sb.String())
}

func (e *Email) formatSignal(signal core.Signal) string {
	return fmt.Sprintf(`
ATLAS Trading Signal

Symbol: %s
Action: %s
Confidence: %.1f%%
Strategy: %s
Reason: %s
Time: %s
`,
		signal.Symbol,
		signal.Action,
		signal.Confidence*100,
		signal.Strategy,
		signal.Reason,
		signal.GeneratedAt.Format("2006-01-02 15:04:05"),
	)
}

func (e *Email) formatSignalHTML(signal core.Signal) string {
	actionColor := "#28a745" // green for buy
	if signal.Action == core.ActionSell || signal.Action == core.ActionStrongSell {
		actionColor = "#dc3545" // red for sell
	}

	return fmt.Sprintf(`
<div style="margin: 10px 0;">
  <h3 style="color: %s;">%s - %s</h3>
  <p><strong>Confidence:</strong> %.1f%%</p>
  <p><strong>Strategy:</strong> %s</p>
  <p><strong>Reason:</strong> %s</p>
  <p><small>%s</small></p>
</div>
`,
		actionColor,
		signal.Symbol,
		signal.Action,
		signal.Confidence*100,
		signal.Strategy,
		signal.Reason,
		signal.GeneratedAt.Format("2006-01-02 15:04:05"),
	)
}

func (e *Email) sendEmail(subject, body string) error {
	addr := fmt.Sprintf("%s:%d", e.host, e.port)

	var auth smtp.Auth
	if e.username != "" {
		auth = smtp.PlainAuth("", e.username, e.password, e.host)
	}

	contentType := "text/plain"
	if strings.Contains(body, "<html>") {
		contentType = "text/html"
	}

	msg := fmt.Sprintf("From: %s\r\n"+
		"To: %s\r\n"+
		"Subject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: %s; charset=UTF-8\r\n"+
		"\r\n"+
		"%s",
		e.from,
		strings.Join(e.to, ","),
		subject,
		contentType,
		body,
	)

	return smtp.SendMail(addr, auth, e.from, e.to, []byte(msg))
}
