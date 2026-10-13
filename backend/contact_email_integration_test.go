package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Uses an isolated schema; never reads or sends the user's contact messages.
func TestContactEmailQueuePersistence(t *testing.T) {
	databaseURL := os.Getenv("CONTACT_EMAIL_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set CONTACT_EMAIL_TEST_DATABASE_URL for isolated PostgreSQL queue checks")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	namespace := fmt.Sprintf("contact_mail_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{namespace}.Sanitize()
	if _, err = db.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := db.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = namespace
	isolated, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer isolated.Close()
	// Re-running schema setup must be safe, as it runs on every API restart.
	for i := 0; i < 2; i++ {
		if _, err = isolated.Exec(ctx, schema); err != nil {
			t.Fatal(err)
		}
	}
	// Exercise the real form handler: one INSERT saves both content and queue intent.
	recorder := httptest.NewRecorder()
	requestContext, _ := gin.CreateTestContext(recorder)
	requestContext.Request = httptest.NewRequest("POST", "/api/contact", strings.NewReader(`{"name":"Form test","email":"visitor@example.com","message":"Save and queue together"}`))
	requestContext.Request.Header.Set("Content-Type", "application/json")
	(&app{db: isolated}).contact(requestContext)
	if recorder.Code != 201 {
		t.Fatalf("contact handler failed: %d", recorder.Code)
	}
	var queued bool
	var savedMessage string
	if err = isolated.QueryRow(ctx, "SELECT email_pending,message FROM contact_messages WHERE name='Form test'").Scan(&queued, &savedMessage); err != nil || !queued || savedMessage != "Save and queue together" {
		t.Fatal("form did not save content and notification intent together", err)
	}
	if _, err = isolated.Exec(ctx, "DELETE FROM contact_messages WHERE name='Form test'"); err != nil {
		t.Fatal(err)
	}
	if _, err = isolated.Exec(ctx, "INSERT INTO contact_messages(name,email,message) VALUES('Historical','old@example.com','Keep database behavior')"); err != nil {
		t.Fatal(err)
	}
	var historicalPending bool
	if err = isolated.QueryRow(ctx, "SELECT email_pending FROM contact_messages WHERE name='Historical'").Scan(&historicalPending); err != nil || historicalPending {
		t.Fatal("historical/default messages must not be auto-mailed", err)
	}
	var id int64
	// Deliberately invalid address causes a local formatting failure, not a network send.
	if err = isolated.QueryRow(ctx, "INSERT INTO contact_messages(name,email,message,email_pending) VALUES('Queued','invalid','Preserved on failure',true) RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	a := &app{db: isolated, mailer: &smtpMailer{}}
	processed, err := a.deliverNextContactEmail(ctx)
	if err != nil || !processed {
		t.Fatal("failed delivery must record retry", err)
	}
	var pending bool
	var attempts int
	var next time.Time
	var message string
	if err = isolated.QueryRow(ctx, "SELECT email_pending,email_attempts,email_next_attempt_at,message FROM contact_messages WHERE id=$1", id).Scan(&pending, &attempts, &next, &message); err != nil {
		t.Fatal(err)
	}
	if !pending || attempts != 1 || !next.After(time.Now()) || message != "Preserved on failure" {
		t.Fatal("delivery failure lost message or did not schedule retry")
	}
	processed, err = a.deliverNextContactEmail(ctx)
	if err != nil || processed {
		t.Fatal("future retries must wait", err)
	}
	if _, err = isolated.Exec(ctx, "UPDATE contact_messages SET email_attempts=$2,email_next_attempt_at=now() WHERE id=$1", id, contactEmailMaxAttempts); err != nil {
		t.Fatal(err)
	}
	processed, err = a.deliverNextContactEmail(ctx)
	if err != nil || processed {
		t.Fatal("exhausted deliveries must stop automatic retries", err)
	}
}
