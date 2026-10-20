// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"os"
	"regexp"
	"strings"
	"time"
)

var (
	emailRE    = regexp.MustCompile(`^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$`)
	hostPortRE = regexp.MustCompile(`^[A-Za-z0-9.-]+:[0-9]{1,5}$`)
)

// smtpSender submits mail to one relay (smtp.gmail.com:587 for a Google
// account) as one authenticated user. The password is read from a file at
// each send, never from the environment or the repo.
type smtpSender struct {
	addr     string // host:port
	host     string
	user     string
	passFile string
	from     string
	to       []string
	startTLS bool
	timeout  time.Duration
}

// smtpFromEnv reads the relay settings. They name the relay and the people;
// none is a secret (SMTP_PASSWORD_FILE is a path).
func smtpFromEnv(getenv func(string) string) (*smtpSender, error) {
	s := &smtpSender{
		addr:     getenv("SMTP_SMARTHOST"),
		user:     getenv("SMTP_USERNAME"),
		passFile: getenv("SMTP_PASSWORD_FILE"),
		from:     getenv("ALERT_MAIL_FROM"),
		startTLS: getenv("SMTP_STARTTLS") != "false",
		timeout:  30 * time.Second,
	}
	var errs []error
	if !hostPortRE.MatchString(s.addr) {
		errs = append(errs, fmt.Errorf("SMTP_SMARTHOST %q: want host:port", s.addr))
	} else {
		s.host, _, _ = net.SplitHostPort(s.addr)
	}
	for _, t := range strings.Split(getenv("ALERT_MAIL_TO"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			s.to = append(s.to, t)
		}
	}
	if len(s.to) == 0 {
		errs = append(errs, errors.New("ALERT_MAIL_TO: at least one address"))
	}
	for _, a := range append([]string{s.from}, s.to...) {
		if !emailRE.MatchString(a) {
			errs = append(errs, fmt.Errorf("not an email address: %q", a))
		}
	}
	if s.user != "" && s.passFile == "" {
		errs = append(errs, errors.New("SMTP_USERNAME set without SMTP_PASSWORD_FILE"))
	}
	// A login over plaintext would hand the password to the network.
	if s.user != "" && !s.startTLS {
		errs = append(errs, errors.New("SMTP_STARTTLS=false with a login: refusing to send the password unencrypted"))
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return s, nil
}

func (s *smtpSender) Send(msg Message) error {
	conn, err := net.DialTimeout("tcp", s.addr, s.timeout)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Now().Add(s.timeout))
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }()
	if s.startTLS {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("%s does not offer STARTTLS", s.addr)
		}
		if err := c.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if s.user != "" {
		pass, err := os.ReadFile(s.passFile)
		if err != nil {
			return fmt.Errorf("SMTP password: %w", err)
		}
		if err := c.Auth(smtp.PlainAuth("", s.user, strings.TrimSpace(string(pass)), s.host)); err != nil {
			return err
		}
	}
	if err := c.Mail(s.from); err != nil {
		return err
	}
	for _, t := range s.to {
		if err := c.Rcpt(t); err != nil {
			return err
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(format(s.from, s.to, msg, time.Now())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// format builds the RFC 5322 message. Subjects are built from alert labels,
// so CR/LF are stripped (no header injection) and non-ASCII is Q-encoded.
func format(from string, to []string, msg Message, now time.Time) []byte {
	subject := strings.Join(strings.Fields(msg.Subject), " ")
	if len(subject) > 200 {
		subject = subject[:200]
	}
	var id [12]byte
	_, _ = rand.Read(id[:])
	domain := from[strings.LastIndexByte(from, '@')+1:]
	var b strings.Builder
	fmt.Fprintf(&b, "From: Unheaded alerts <%s>\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", strings.Join(to, ", "))
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", now.Format(time.RFC1123Z))
	fmt.Fprintf(&b, "Message-ID: <%s@%s>\r\n", hex.EncodeToString(id[:]), domain)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	// RFC 3834: machine-generated; mail systems must not auto-reply to it.
	b.WriteString("Auto-Submitted: auto-generated\r\n")
	b.WriteString("X-Auto-Response-Suppress: All\r\n")
	fmt.Fprintf(&b, "X-Unheaded-Urgency: %s\r\n\r\n", msg.Urgency)
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(msg.Body, "\r\n", "\n"), "\n", "\r\n"))
	return []byte(b.String())
}
