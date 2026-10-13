package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/textproto"
	"strings"
	"testing"
	"time"
)

func TestSMTPConfiguration(t *testing.T) {
	for _, key := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_TLS_MODE"} {
		t.Setenv(key, "")
	}
	m, err := smtpMailerFromEnv("admin@example.com")
	if err != nil || m != nil {
		t.Fatal("unset SMTP should preserve database-only behavior")
	}
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_TLS_MODE", "implicit")
	m, err = smtpMailerFromEnv("admin@example.com")
	if err != nil || m.port != "465" || m.to.Address != "admin@example.com" {
		t.Fatal("implicit TLS defaults or recipient incorrect")
	}
	t.Setenv("SMTP_TLS_MODE", "none")
	if _, err = smtpMailerFromEnv("admin@example.com"); err == nil {
		t.Fatal("plaintext mode must be rejected")
	}
	t.Setenv("SMTP_TLS_MODE", "starttls")
	t.Setenv("SMTP_USERNAME", "sender")
	if _, err = smtpMailerFromEnv("admin@example.com"); err == nil {
		t.Fatal("partial credentials must be rejected")
	}
	t.Setenv("SMTP_PASSWORD", "test-only-password")
	t.Setenv("SMTP_FROM", "sender@example.com\r\nBcc: attacker@example.com")
	if _, err = smtpMailerFromEnv("admin@example.com"); err == nil {
		t.Fatal("sender header injection must be rejected")
	}
}

func TestContactNotificationMessage(t *testing.T) {
	m := &smtpMailer{from: &mail.Address{Address: "sender@example.com"}, to: &mail.Address{Address: "admin@example.com"}}
	n := contactNotification{ID: 42, Name: "Visitor\r\nBcc: attacker@example.com", Email: "visitor@example.com", Message: "Unicode: café 世界\n.dot-leading line"}
	raw, err := m.message(n)
	if err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if message.Header.Get("Bcc") != "" || message.Header.Get("To") != "<admin@example.com>" || message.Header.Get("Reply-To") != "<visitor@example.com>" {
		t.Fatal("recipient/reply headers incorrect or injected")
	}
	body, err := io.ReadAll(quotedprintable.NewReader(message.Body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "café 世界") || !strings.Contains(string(body), n.Name) {
		t.Fatalf("message body lost content: %q", body)
	}
	n.Email = "visitor@example.com\r\nBcc: attacker@example.com"
	if _, err = m.message(n); err == nil {
		t.Fatal("reply header injection must be rejected")
	}
}

// Local-only SMTP fixture: never sends email to an external provider.
func TestSMTPDelivery(t *testing.T) {
	certServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificate := certServer.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(certServer.Certificate())
	certServer.Close()
	for _, mode := range []string{"starttls", "implicit"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			host, port, _ := net.SplitHostPort(listener.Addr().String())
			done := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					done <- err
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				serverTLS := &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
				if mode == "implicit" {
					conn = tls.Server(conn, serverTLS)
				}
				text := textproto.NewConn(conn)
				_ = text.PrintfLine("220 localhost test SMTP")
				for {
					line, err := text.ReadLine()
					if err != nil {
						done <- err
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"):
						_ = text.PrintfLine("250-localhost")
						_ = text.PrintfLine("250-STARTTLS")
						_ = text.PrintfLine("250 AUTH PLAIN")
					case line == "STARTTLS":
						_ = text.PrintfLine("220 start TLS")
						conn = tls.Server(conn, serverTLS)
						text = textproto.NewConn(conn)
					case strings.HasPrefix(line, "AUTH PLAIN "):
						decoded, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "AUTH PLAIN "))
						if string(decoded) != "\x00sender\x00test-only-password" {
							done <- fmt.Errorf("unexpected AUTH payload")
							return
						}
						_ = text.PrintfLine("235 authenticated")
					case line == "MAIL FROM:<sender@example.com>" || line == "RCPT TO:<admin@example.com>":
						_ = text.PrintfLine("250 accepted")
					case line == "DATA":
						_ = text.PrintfLine("354 send data")
						body, err := text.ReadDotBytes()
						if err != nil {
							done <- err
							return
						}
						if !strings.Contains(string(body), "Reply-To: <visitor@example.com>") {
							done <- fmt.Errorf("missing reply address")
							return
						}
						_ = text.PrintfLine("250 queued")
					case line == "QUIT":
						_ = text.PrintfLine("221 bye")
						done <- nil
						return
					default:
						done <- fmt.Errorf("unexpected SMTP command %q", line)
						return
					}
				}
			}()
			m := &smtpMailer{host: host, port: port, mode: mode, username: "sender", password: "test-only-password", from: &mail.Address{Address: "sender@example.com"}, to: &mail.Address{Address: "admin@example.com"}, tlsConfig: &tls.Config{ServerName: host, RootCAs: roots, MinVersion: tls.VersionTLS12}}
			if err = m.send(context.Background(), contactNotification{ID: 1, Name: "Visitor", Email: "visitor@example.com", Message: "Hello"}); err != nil {
				t.Fatal(err)
			}
			if err = <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestContactEmailRetryDelay(t *testing.T) {
	if contactEmailRetryDelay(1) != time.Minute || contactEmailRetryDelay(3) != 4*time.Minute || contactEmailRetryDelay(100) != 128*time.Minute {
		t.Fatal("retry delays are not bounded exponential backoff")
	}
}
