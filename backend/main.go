package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql
var schema string

type project struct {
	ID          int64    `json:"id"`
	Title       string   `json:"title" binding:"required,max=160"`
	Description string   `json:"description" binding:"required,max=4000"`
	URL         string   `json:"url" binding:"max=2048"`
	Tags        []string `json:"tags"`
	SortOrder   int32    `json:"sort_order" binding:"gte=0"`
}
type app struct {
	db            *pgxpool.Pool
	cache         *redis.Client
	started       time.Time
	requests      atomic.Uint64
	hits          atomic.Uint64
	misses        atomic.Uint64
	password      []byte
	email, origin string
	secure        bool
	mailer        *smtpMailer
}

func env(key, fallback string) string {
	if s := os.Getenv(key); s != "" {
		return s
	}
	return fallback
}
func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	password := os.Getenv("ADMIN_PASSWORD")
	if len(password) < 16 || len(password) > 72 || strings.HasPrefix(password, "replace-") {
		log.Fatal("Set a unique ADMIN_PASSWORD of 16-72 bytes")
	}
	email := os.Getenv("ADMIN_EMAIL")
	if _, err := mail.ParseAddress(email); err != nil {
		log.Fatal("Set a valid ADMIN_EMAIL")
	}
	db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err = db.Ping(ctx); err != nil {
		log.Fatal(err)
	}
	if _, err = db.Exec(ctx, schema); err != nil {
		log.Fatal(err)
	}
	opts, err := redis.ParseURL(env("REDIS_URL", "redis://localhost:6379/0"))
	if err != nil {
		log.Fatal(err)
	}
	cache := redis.NewClient(opts)
	defer cache.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}
	a := &app{db: db, cache: cache, started: time.Now(), password: hash, email: email, origin: env("PUBLIC_ORIGIN", "http://localhost:3000"), secure: env("SESSION_SECURE", "false") == "true"}
	a.mailer, err = smtpMailerFromEnv(email)
	if err != nil {
		log.Fatal(err)
	}
	mailContext, stopMail := context.WithCancel(context.Background())
	defer stopMail()
	mailDone := make(chan struct{})
	go func() {
		defer close(mailDone)
		a.runContactMailWorker(mailContext)
	}()
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), func(c *gin.Context) {
		a.requests.Add(1)
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cache-Control", "no-store")
		c.Next()
	})
	if err := r.SetTrustedProxies(nil); err != nil {
		log.Fatal(err)
	}
	r.GET("/api/health", a.health)
	r.GET("/api/projects", a.projects)
	r.POST("/api/contact", a.sameOrigin, a.limit("contact", 5, time.Hour), a.contact)
	r.POST("/api/auth/login", a.sameOrigin, a.limit("login", 10, 15*time.Minute), a.login)
	r.POST("/api/auth/logout", a.sameOrigin, a.logout)
	r.GET("/api/auth/me", a.auth, func(c *gin.Context) { c.JSON(200, gin.H{"email": a.email}) })
	admin := r.Group("/api/admin", a.sameOrigin, a.auth)
	admin.POST("/projects", a.saveProject)
	admin.PUT("/projects/:id", a.saveProject)
	admin.DELETE("/projects/:id", a.deleteProject)
	srv := &http.Server{Addr: ":8080", Handler: r, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	shutdown, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	_ = srv.Shutdown(shutdown)
	stopMail()
	<-mailDone
}
func (a *app) sameOrigin(c *gin.Context) {
	if c.Request.Method != "GET" && c.GetHeader("Origin") != a.origin {
		c.AbortWithStatusJSON(403, gin.H{"error": "Origin denied"})
		return
	}
	c.Next()
}

const limiterScript = "local n = redis.call('INCR', KEYS[1]); if n == 1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end; return n"

func (a *app) limit(prefix string, maximum int64, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Shared proxy bucket cannot be bypassed using spoofed forwarding headers.
		n, err := a.cache.Eval(c.Request.Context(), limiterScript, []string{"limit:" + prefix + ":" + c.ClientIP()}, int(window.Seconds())).Int64()
		if err != nil {
			c.AbortWithStatusJSON(503, gin.H{"error": "Please try again later"})
			return
		}
		if n > maximum {
			c.Header("Retry-After", strconv.Itoa(int(window.Seconds())))
			c.AbortWithStatusJSON(429, gin.H{"error": "Too many requests. Try again later."})
			return
		}
		c.Next()
	}
}
func digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func (a *app) auth(c *gin.Context) {
	token, err := c.Cookie("portfolio_session")
	if err != nil || len(token) != 64 {
		c.AbortWithStatus(401)
		return
	}
	var found bool
	err = a.db.QueryRow(c.Request.Context(), "SELECT EXISTS(SELECT 1 FROM sessions WHERE token_hash=$1 AND expires_at>now())", digest(token)).Scan(&found)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	if !found {
		c.AbortWithStatus(401)
		return
	}
	c.Next()
}
func (a *app) cookie(c *gin.Context, token string, age int) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie("portfolio_session", token, age, "/", "", a.secure, true)
}
func (a *app) login(c *gin.Context) {
	var input struct {
		Email    string `json:"email" binding:"required,max=254"`
		Password string `json:"password" binding:"required,max=72"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "Invalid login"})
		return
	}
	validPassword := bcrypt.CompareHashAndPassword(a.password, []byte(input.Password)) == nil
	validEmail := subtle.ConstantTimeCompare([]byte(strings.ToLower(input.Email)), []byte(strings.ToLower(a.email))) == 1
	if !validPassword || !validEmail {
		c.JSON(401, gin.H{"error": "Invalid credentials"})
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		c.Status(500)
		return
	}
	token := hex.EncodeToString(b)
	tx, err := a.db.Begin(c.Request.Context())
	if err != nil {
		c.Status(503)
		return
	}
	defer tx.Rollback(c.Request.Context())
	_, err = tx.Exec(c.Request.Context(), "DELETE FROM sessions WHERE expires_at<=now()")
	if err == nil {
		_, err = tx.Exec(c.Request.Context(), "INSERT INTO sessions(token_hash,expires_at) VALUES($1,now()+interval '8 hours')", digest(token))
	}
	if err == nil {
		err = tx.Commit(c.Request.Context())
	}
	if err != nil {
		c.Status(503)
		return
	}
	a.cookie(c, token, 8*3600)
	c.JSON(200, gin.H{"email": a.email})
}
func (a *app) logout(c *gin.Context) {
	if token, err := c.Cookie("portfolio_session"); err == nil {
		if _, err = a.db.Exec(c.Request.Context(), "DELETE FROM sessions WHERE token_hash=$1", digest(token)); err != nil {
			c.Status(503)
			return
		}
	}
	a.cookie(c, "", -1)
	c.Status(204)
}
func (a *app) projects(c *gin.Context) {
	ctx := c.Request.Context()
	// Generation keys prevent a concurrent reader from filling stale cache after an edit.
	generation, _ := a.cache.Get(ctx, "projects:generation").Result()
	key := "projects:ordered:" + generation
	if data, err := a.cache.Get(ctx, key).Bytes(); err == nil {
		a.hits.Add(1)
		c.Header("X-Cache", "HIT")
		c.Data(200, "application/json", data)
		return
	}
	a.misses.Add(1)
	rows, err := a.db.Query(ctx, "SELECT id,title,description,url,tags,sort_order FROM projects ORDER BY sort_order ASC,created_at DESC,id DESC")
	if err != nil {
		c.JSON(503, gin.H{"error": "Projects unavailable"})
		return
	}
	defer rows.Close()
	items := []project{}
	for rows.Next() {
		var p project
		if err = rows.Scan(&p.ID, &p.Title, &p.Description, &p.URL, &p.Tags, &p.SortOrder); err != nil {
			c.Status(500)
			return
		}
		items = append(items, p)
	}
	if rows.Err() != nil {
		c.Status(503)
		return
	}
	data, _ := json.Marshal(items)
	_ = a.cache.Set(ctx, key, data, 60*time.Second).Err()
	c.Header("X-Cache", "MISS")
	c.Data(200, "application/json", data)
}
func validProject(p project) bool {
	if strings.TrimSpace(p.Title) == "" || strings.TrimSpace(p.Description) == "" || len(p.Tags) > 12 {
		return false
	}
	for _, t := range p.Tags {
		if len(t) > 40 {
			return false
		}
	}
	if p.URL != "" {
		u, err := url.Parse(p.URL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
			return false
		}
	}
	return true
}
func (a *app) invalidate(ctx context.Context) {
	if err := a.cache.Incr(ctx, "projects:generation").Err(); err != nil {
		log.Print("Cache invalidation failed; entries expire within 60 seconds")
	}
}
func (a *app) saveProject(c *gin.Context) {
	var p project
	if c.ShouldBindJSON(&p) != nil || !validProject(p) {
		c.JSON(400, gin.H{"error": "Invalid title, description, tags, URL, or sort order (use a whole number from 0 to 2147483647)"})
		return
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	var err error
	if c.Request.Method == "POST" {
		err = a.db.QueryRow(c.Request.Context(), "INSERT INTO projects(title,description,url,tags,sort_order) VALUES($1,$2,$3,$4,$5) RETURNING id", p.Title, p.Description, p.URL, p.Tags, p.SortOrder).Scan(&p.ID)
	} else {
		id, e := strconv.ParseInt(c.Param("id"), 10, 64)
		if e != nil || id <= 0 {
			c.Status(400)
			return
		}
		err = a.db.QueryRow(c.Request.Context(), "UPDATE projects SET title=$1,description=$2,url=$3,tags=$4,sort_order=$5 WHERE id=$6 RETURNING id", p.Title, p.Description, p.URL, p.Tags, p.SortOrder, id).Scan(&p.ID)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		c.Status(404)
		return
	}
	if err != nil {
		c.Status(503)
		return
	}
	a.invalidate(c.Request.Context())
	c.JSON(200, p)
}
func (a *app) deleteProject(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.Status(400)
		return
	}
	result, err := a.db.Exec(c.Request.Context(), "DELETE FROM projects WHERE id=$1", id)
	if err != nil {
		c.Status(503)
		return
	}
	if result.RowsAffected() == 0 {
		c.Status(404)
		return
	}
	a.invalidate(c.Request.Context())
	c.Status(204)
}
func (a *app) contact(c *gin.Context) {
	var input struct {
		Name    string `json:"name" binding:"required,max=120"`
		Email   string `json:"email" binding:"required,max=254"`
		Message string `json:"message" binding:"required,max=4000"`
	}
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, gin.H{"error": "Invalid contact message"})
		return
	}
	if _, err := mail.ParseAddress(input.Email); err != nil || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Message) == "" {
		c.Status(400)
		return
	}
	_, err := a.db.Exec(c.Request.Context(), "INSERT INTO contact_messages(name,email,message,email_pending) VALUES($1,$2,$3,true)", input.Name, input.Email, input.Message)
	if err != nil {
		c.JSON(503, gin.H{"error": "Unable to save message"})
		return
	}
	c.JSON(201, gin.H{"message": "Message received. Thank you!"})
}
func (a *app) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	started := time.Now()
	dbErr := a.db.Ping(ctx)
	latency := float64(time.Since(started).Microseconds()) / 1000
	redisErr := a.cache.Ping(ctx).Err()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	status := 200
	state := "healthy"
	if dbErr != nil || redisErr != nil {
		status = 503
		state = "degraded"
	}
	c.JSON(status, gin.H{"status": state, "uptime_seconds": int(time.Since(a.started).Seconds()), "database_ms": latency, "database_connected": dbErr == nil, "redis_connected": redisErr == nil, "memory_mb": float64(mem.Alloc) / 1024 / 1024, "requests": a.requests.Load(), "cache_hits": a.hits.Load(), "cache_misses": a.misses.Load()})
}
