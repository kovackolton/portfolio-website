package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type contactNotification struct {
	ID                   int64
	Name, Email, Message string
}

type smtpMailer struct {
	host, port, username, password, mode string
	from, to                             *mail.Address
	// Optional custom trust roots for the local TLS test server; never configured by environment.
	tlsConfig *tls.Config
}

func smtpMailerFromEnv(adminEmail string) (*smtpMailer, error) {
	host := strings.TrimSpace(os.Getenv("SMTP_HOST"))
	if host == "" {
		if os.Getenv("SMTP_USERNAME") != "" || os.Getenv("SMTP_PASSWORD") != "" || os.Getenv("SMTP_FROM") != "" {
			return nil, errors.New("SMTP_HOST is required when SMTP credentials or sender are configured")
		}
		return nil, nil
	}
	mode := env("SMTP_TLS_MODE", "starttls")
	if mode != "starttls" && mode != "implicit" {
		return nil, errors.New("SMTP_TLS_MODE must be starttls or implicit; plaintext delivery is disabled")
	}
	defaultPort := "587"
	if mode == "implicit" {
		defaultPort = "465"
	}
	port := env("SMTP_PORT", defaultPort)
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return nil, errors.New("SMTP_PORT must be a valid TCP port")
	}
	username, password := os.Getenv("SMTP_USERNAME"), os.Getenv("SMTP_PASSWORD")
	if (username == "") != (password == "") {
		return nil, errors.New("set both SMTP_USERNAME and SMTP_PASSWORD, or neither for a trusted relay")
	}
	from, err := notificationAddress(env("SMTP_FROM", adminEmail))
	if err != nil {
		return nil, errors.New("SMTP_FROM must be a valid email address")
	}
	to, err := notificationAddress(adminEmail)
	if err != nil {
		return nil, errors.New("ADMIN_EMAIL must be a valid notification recipient")
	}
	return &smtpMailer{host: host, port: port, username: username, password: password, mode: mode, from: from, to: to}, nil
}

func notificationAddress(value string) (*mail.Address, error) {
	if strings.ContainsAny(value, "\r\n") {
		return nil, errors.New("email headers cannot contain line breaks")
	}
	return mail.ParseAddress(value)
}

func (m *smtpMailer) message(n contactNotification) ([]byte, error) {
	reply, err := notificationAddress(n.Email)
	if err != nil {
		return nil, err
	}
	var message bytes.Buffer
	fmt.Fprintf(&message, "From: %s\r\nTo: %s\r\nReply-To: %s\r\nSubject: Portfolio contact message #%d\r\nDate: %s\r\nMessage-ID: <portfolio-contact-%d@%s>\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", m.from.String(), m.to.String(), (&mail.Address{Address: reply.Address}).String(), n.ID, time.Now().Format(time.RFC1123Z), n.ID, strings.SplitN(m.from.Address, "@", 2)[1])
	body := quotedprintable.NewWriter(&message)
	if _, err := fmt.Fprintf(body, "New portfolio contact message\n\nName: %s\nEmail: %s\n\nMessage:\n%s\n", n.Name, n.Email, n.Message); err != nil {
		return nil, err
	}
	if err := body.Close(); err != nil {
		return nil, err
	}
	return message.Bytes(), nil
}

func (m *smtpMailer) send(ctx context.Context, n contactNotification) error {
	message, err := m.message(n)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(m.host, m.port))
	if err != nil {
		return err
	}
	defer conn.Close()
	// SMTP methods lack context arguments: closing the connection interrupts cancellation.
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	tlsConfig := &tls.Config{ServerName: m.host, MinVersion: tls.VersionTLS12}
	if m.tlsConfig != nil {
		tlsConfig = m.tlsConfig.Clone()
	}
	if m.mode == "implicit" {
		secured := tls.Client(conn, tlsConfig)
		if err = secured.HandshakeContext(ctx); err != nil {
			return err
		}
		conn = secured
	}
	client, err := smtp.NewClient(conn, m.host)
	if err != nil {
		return err
	}
	defer client.Close()
	if m.mode == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("SMTP server does not support required STARTTLS")
		}
		if err = client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if m.username != "" {
		if err = client.Auth(smtp.PlainAuth("", m.username, m.password, m.host)); err != nil {
			return err
		}
	}
	if err = client.Mail(m.from.Address); err != nil {
		return err
	}
	if err = client.Rcpt(m.to.Address); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = writer.Write(message); err != nil {
		_ = writer.Close()
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	// DATA acceptance is success; a failed QUIT should not cause a duplicate retry.
	_ = client.Quit()
	return nil
}

const contactEmailMaxAttempts = 8

func contactEmailRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > contactEmailMaxAttempts {
		attempt = contactEmailMaxAttempts
	}
	return time.Minute * time.Duration(1<<(attempt-1))
}

func (a *app) runContactMailWorker(ctx context.Context) {
	if a.mailer == nil {
		log.Print("Contact email delivery disabled: set SMTP_HOST and provider credentials; new messages remain queued in PostgreSQL")
		return
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		for i := 0; i < 10 && ctx.Err() == nil; i++ {
			processed, err := a.deliverNextContactEmail(ctx)
			if err != nil {
				log.Print("Contact email queue processing failed; will retry on next poll")
				break
			}
			if !processed {
				break
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *app) deliverNextContactEmail(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 2*time.Second)
		defer done()
		_ = tx.Rollback(cleanup)
	}()
	var n contactNotification
	var attempts int
	err = tx.QueryRow(ctx, "SELECT id,name,email,message,email_attempts FROM contact_messages WHERE email_pending AND email_attempts < $1 AND email_next_attempt_at <= now() ORDER BY email_next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT 1", contactEmailMaxAttempts).Scan(&n.ID, &n.Name, &n.Email, &n.Message, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = a.mailer.send(ctx, n); err != nil {
		attempts++
		log.Printf("Contact notification %d delivery failed (attempt %d/%d); message retained in database", n.ID, attempts, contactEmailMaxAttempts)
		_, err = tx.Exec(ctx, "UPDATE contact_messages SET email_attempts=$2,email_next_attempt_at=now()+($3 * interval '1 second') WHERE id=$1", n.ID, attempts, int(contactEmailRetryDelay(attempts).Seconds()))
	} else {
		_, err = tx.Exec(ctx, "UPDATE contact_messages SET email_pending=false,email_sent_at=now(),email_attempts=email_attempts+1 WHERE id=$1", n.ID)
	}
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}
