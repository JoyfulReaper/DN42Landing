package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed static/index.html static/toys.html static/site.css static/toys.js
var staticFS embed.FS

const (
	defaultIPv4Listen = "172.20.220.50:80"
	defaultIPv6Listen = "[fdf0:e12c:5528::50]:80"
	defaultDBPath     = "/var/lib/dn42landing/peering.db"
	defaultNtfyURL    = "http://127.0.0.1:5197/dn42-peering"

	defaultQOTDURL       = "https://qotd.kgivler.dn42/api/quotes/today"
	defaultRandomQOTDURL = "https://qotd.kgivler.dn42/api/quotes/random"
	defaultGitURL        = "https://api.kgivler.dn42/api/github/activity?limit=4"
	defaultStatusURL     = "https://api.kgivler.dn42/api/system/status"
)

var asnPattern = regexp.MustCompile(`(?i)^AS[0-9]{1,10}$`)

type app struct {
	db *sql.DB

	page     *template.Template
	toysPage *template.Template

	ntfyURL   string
	ntfyToken string

	httpClient *http.Client

	peeringRateLimiter *globalRateLimiter
	quoteRateLimiter   *globalRateLimiter
}

type pageData struct {
	Submitted bool
	Error     string
	Form      peeringRequest
}

type toysPageData struct {
	QuoteSubmitted bool
}

type peeringRequest struct {
	ASN                 string
	NetworkName         string
	Contact             string
	Endpoint            string
	WireGuardPublicKey  string
	LinkLocalPreference string
	Notes               string
	RemoteAddress       string
	UserAgent           string
}

type quoteSuggestion struct {
	Text          string
	Author        string
	Source        string
	Notes         string
	RemoteAddress string
	UserAgent     string
}

type globalRateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	lastSeen time.Time
}

func main() {
	dbPath := envOrDefault("DN42LANDING_DB", defaultDBPath)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		log.Fatalf("create database directory: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := initializeDatabase(db); err != nil {
		log.Fatalf("initialize database: %v", err)
	}

	pageBytes, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		log.Fatalf("read embedded page: %v", err)
	}

	toysBytes, err := staticFS.ReadFile("static/toys.html")
	if err != nil {
		log.Fatalf("read embedded toys page: %v", err)
	}

	toysPage, err := template.New("toys").Parse(string(toysBytes))
	if err != nil {
		log.Fatalf("parse toys page template: %v", err)
	}

	page, err := template.New("index").Parse(string(pageBytes))
	if err != nil {
		log.Fatalf("parse page template: %v", err)
	}

	peeringRateLimit, err := time.ParseDuration(
		envOrDefault("DN42LANDING_RATE_LIMIT", "15m"),
	)
	if err != nil {
		log.Fatalf("invalid DN42LANDING_RATE_LIMIT: %v", err)
	}

	quoteRateLimit, err := time.ParseDuration(
		envOrDefault("DN42LANDING_QUOTE_RATE_LIMIT", "5m"),
	)
	if err != nil {
		log.Fatalf("invalid DN42LANDING_QUOTE_RATE_LIMIT: %v", err)
	}

	a := &app{
		db:        db,
		page:      page,
		toysPage:  toysPage,
		ntfyURL:   os.Getenv("DN42LANDING_NTFY_URL"),
		ntfyToken: os.Getenv("DN42LANDING_NTFY_TOKEN"),
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
		peeringRateLimiter: &globalRateLimiter{
			interval: peeringRateLimit,
		},
		quoteRateLimiter: &globalRateLimiter{
			interval: quoteRateLimit,
		},
	}

	if _, present := os.LookupEnv("DN42LANDING_NTFY_URL"); !present {
		a.ntfyURL = defaultNtfyURL
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /whoami", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, requestRemoteHost(r)+"\n")
	})

	mux.HandleFunc("GET /api/whoami", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")

		_ = json.NewEncoder(w).Encode(map[string]string{
			"address": requestRemoteHost(r),
			"network": "dn42",
		})
	})

	mux.HandleFunc("GET /site.css", func(w http.ResponseWriter, r *http.Request) {
		css, err := staticFS.ReadFile("static/site.css")
		if err != nil {
			http.Error(w, "stylesheet unavailable", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(css)
	})

	mux.HandleFunc("GET /", a.handleIndex)
	mux.HandleFunc("GET /toys", a.handleToys)
	mux.HandleFunc("GET /toys.js", func(w http.ResponseWriter, r *http.Request) {
		js, err := staticFS.ReadFile("static/toys.js")
		if err != nil {
			http.Error(w, "script unavailable", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write(js)
	})

	mux.HandleFunc("GET /api/toys/qotd", func(w http.ResponseWriter, r *http.Request) {
		proxyJSON(
			w,
			r,
			a.httpClient,
			envOrDefault("DN42LANDING_QOTD_URL", defaultQOTDURL),
		)
	})

	mux.HandleFunc("GET /api/toys/git", func(w http.ResponseWriter, r *http.Request) {
		proxyJSON(
			w,
			r,
			a.httpClient,
			envOrDefault("DN42LANDING_GIT_URL", defaultGitURL),
		)
	})

	mux.HandleFunc("GET /api/toys/random-quote", func(w http.ResponseWriter, r *http.Request) {
		proxyJSON(
			w,
			r,
			a.httpClient,
			envOrDefault("DN42LANDING_RANDOM_QOTD_URL", defaultRandomQOTDURL),
		)
	})

	mux.HandleFunc("GET /api/toys/workstation", func(w http.ResponseWriter, r *http.Request) {
		proxyJSON(
			w,
			r,
			a.httpClient,
			envOrDefault("DN42LANDING_STATUS_URL", defaultStatusURL),
		)
	})

	mux.HandleFunc("POST /peering/request", a.handlePeeringRequest)
	mux.HandleFunc("POST /toys/quotes/suggest", a.handleQuoteSuggestion)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})

	errCh := make(chan error, 2)

	listeners := []string{
		envOrConfigured("DN42LANDING_IPV4_LISTEN", defaultIPv4Listen),
		envOrConfigured("DN42LANDING_IPV6_LISTEN", defaultIPv6Listen),
	}

	started := 0
	for _, addr := range listeners {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}

		started++
		go func(listenAddr string) {
			log.Printf("listening on http://%s", listenAddr)
			server := &http.Server{
				Addr:              listenAddr,
				Handler:           requestLogging(mux),
				ReadHeaderTimeout: 5 * time.Second,
				ReadTimeout:       10 * time.Second,
				WriteTimeout:      10 * time.Second,
				IdleTimeout:       60 * time.Second,
			}
			errCh <- server.ListenAndServe()
		}(addr)
	}

	if started == 0 {
		log.Fatal("no listen addresses configured")
	}

	log.Fatal(<-errCh)
}

func proxyJSON(
	w http.ResponseWriter,
	r *http.Request,
	client *http.Client,
	url string,
) {
	req, err := http.NewRequestWithContext(
		r.Context(),
		http.MethodGet,
		url,
		nil,
	)
	if err != nil {
		http.Error(w, "upstream request failed", http.StatusInternalServerError)
		return
	}

	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("toy upstream %s: %v", url, err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("toy upstream %s returned %s", url, resp.Status)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")

	if _, err := io.Copy(w, resp.Body); err != nil {
		log.Printf("copy toy upstream response: %v", err)
	}
}

func initializeDatabase(db *sql.DB) error {
	statements := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA synchronous = NORMAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`CREATE TABLE IF NOT EXISTS peering_requests (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        submitted_utc TEXT NOT NULL,
                        asn TEXT NOT NULL,
                        network_name TEXT NOT NULL,
                        contact TEXT NOT NULL,
                        endpoint TEXT NOT NULL,
                        wireguard_public_key TEXT NOT NULL,
                        link_local_preference TEXT NOT NULL DEFAULT '',
                        notes TEXT NOT NULL DEFAULT '',
                        remote_address TEXT NOT NULL,
                        user_agent TEXT NOT NULL DEFAULT '',
                        status TEXT NOT NULL DEFAULT 'Pending'
                );`,

		`CREATE TABLE IF NOT EXISTS quote_suggestions (
                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                        submitted_utc TEXT NOT NULL,
                        quote_text TEXT NOT NULL,
                        author TEXT NOT NULL DEFAULT '',
                        source TEXT NOT NULL DEFAULT '',
                        notes TEXT NOT NULL DEFAULT '',
                        remote_address TEXT NOT NULL,
                        user_agent TEXT NOT NULL DEFAULT '',
                        status TEXT NOT NULL DEFAULT 'Pending'
                );`,

		`CREATE INDEX IF NOT EXISTS idx_quote_suggestions_status_submitted
    ON quote_suggestions(status, submitted_utc DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_peering_requests_status_submitted
                        ON peering_requests(status, submitted_utc DESC);`,
	}

	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}

	return nil
}

func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	a.renderIndex(w, http.StatusOK, pageData{
		Submitted: r.URL.Query().Get("submitted") == "1",
	})
}

func (a *app) handlePeeringRequest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form submission.", http.StatusBadRequest)
		return
	}

	// Honeypot. Humans never see this field.
	if strings.TrimSpace(r.FormValue("website")) != "" {
		http.Redirect(w, r, "/?submitted=1", http.StatusSeeOther)
		return
	}

	remote := requestRemoteHost(r)

	req := peeringRequest{
		ASN:                 clean(r.FormValue("asn"), 16),
		NetworkName:         clean(r.FormValue("network_name"), 100),
		Contact:             clean(r.FormValue("contact"), 200),
		Endpoint:            clean(r.FormValue("endpoint"), 255),
		WireGuardPublicKey:  clean(r.FormValue("wireguard_public_key"), 100),
		LinkLocalPreference: clean(r.FormValue("link_local_preference"), 100),
		Notes:               clean(r.FormValue("notes"), 1200),
		RemoteAddress:       remote,
		UserAgent:           clean(r.UserAgent(), 300),
	}

	if err := validatePeeringRequest(req); err != nil {
		a.renderIndex(w, http.StatusBadRequest, pageData{
			Error: err.Error(),
			Form:  req,
		})
		return
	}

	if !a.peeringRateLimiter.allow() {
		http.Error(
			w,
			"For my sanity, peering requests are globally limited to one submission every 15 minutes. If someone beat you to it, wait a bit and try again.",
			http.StatusTooManyRequests,
		)
		return
	}

	result, err := a.db.ExecContext(
		r.Context(),
		`INSERT INTO peering_requests (
                        submitted_utc,
                        asn,
                        network_name,
                        contact,
                        endpoint,
                        wireguard_public_key,
                        link_local_preference,
                        notes,
                        remote_address,
                        user_agent,
                        status
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'Pending')`,
		time.Now().UTC().Format(time.RFC3339),
		strings.ToUpper(req.ASN),
		req.NetworkName,
		req.Contact,
		req.Endpoint,
		req.WireGuardPublicKey,
		req.LinkLocalPreference,
		req.Notes,
		req.RemoteAddress,
		req.UserAgent,
	)
	if err != nil {
		log.Printf("save peering request: %v", err)
		http.Error(w, "Unable to save request.", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	if a.ntfyURL != "" {
		notice := fmt.Sprintf(
			"Request ID: %d\nASN: %s\nNetwork: %s\nContact: %s\nEndpoint: %s\nSource: %s",
			id,
			strings.ToUpper(req.ASN),
			req.NetworkName,
			req.Contact,
			req.Endpoint,
			req.RemoteAddress,
		)

		if err := a.publishNtfyMessage(
			r.Context(),
			"DN42 peering request",
			"satellite",
			notice,
		); err != nil {
			log.Printf("ntfy notification failed for request %d: %v", id, err)
		}
	}

	http.Redirect(w, r, "/?submitted=1", http.StatusSeeOther)
}

func (a *app) handleQuoteSuggestion(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form submission.", http.StatusBadRequest)
		return
	}

	// Honeypot. Humans never see this field.
	if strings.TrimSpace(r.FormValue("quote_website")) != "" {
		http.Redirect(w, r, "/toys?quote_submitted=1", http.StatusSeeOther)
		return
	}

	suggestion := quoteSuggestion{
		Text:          clean(r.FormValue("quote_text"), 1000),
		Author:        clean(r.FormValue("quote_author"), 200),
		Source:        clean(r.FormValue("quote_source"), 300),
		Notes:         clean(r.FormValue("quote_notes"), 500),
		RemoteAddress: requestRemoteHost(r),
		UserAgent:     clean(r.UserAgent(), 300),
	}

	if suggestion.Text == "" {
		http.Error(w, "Quote text is required.", http.StatusBadRequest)
		return
	}

	if !a.quoteRateLimiter.allow() {
		http.Error(
			w,
			"Quote suggestions are limited to one submission every 5 minutes. Try again in a bit.",
			http.StatusTooManyRequests,
		)
		return
	}

	result, err := a.db.ExecContext(
		r.Context(),
		`INSERT INTO quote_suggestions (
                        submitted_utc,
                        quote_text,
                        author,
                        source,
                        notes,
                        remote_address,
                        user_agent,
                        status
                ) VALUES (?, ?, ?, ?, ?, ?, ?, 'Pending')`,
		time.Now().UTC().Format(time.RFC3339),
		suggestion.Text,
		suggestion.Author,
		suggestion.Source,
		suggestion.Notes,
		suggestion.RemoteAddress,
		suggestion.UserAgent,
	)
	if err != nil {
		log.Printf("save quote suggestion: %v", err)
		http.Error(w, "Unable to save suggestion.", http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()

	if a.ntfyURL != "" {
		notice := fmt.Sprintf(
			"Quote suggestion #%d\nQuote: %s\nAuthor: %s\nSource: %s\nFrom: %s",
			id,
			suggestion.Text,
			suggestion.Author,
			suggestion.Source,
			suggestion.RemoteAddress,
		)

		if err := a.publishNtfyMessage(
			r.Context(),
			"DN42 quote suggestion",
			"memo",
			notice,
		); err != nil {
			log.Printf(
				"quote suggestion ntfy failed for %d: %v",
				id,
				err,
			)
		}
	}

	http.Redirect(w, r, "/toys?quote_submitted=1", http.StatusSeeOther)
}

func (a *app) handleToys(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")

	data := toysPageData{
		QuoteSubmitted: r.URL.Query().Get("quote_submitted") == "1",
	}

	if err := a.toysPage.Execute(w, data); err != nil {
		log.Printf("render toys page: %v", err)
	}
}

func (a *app) renderIndex(w http.ResponseWriter, status int, data pageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(status)

	if err := a.page.Execute(w, data); err != nil {
		log.Printf("render page: %v", err)
	}
}

func validatePeeringRequest(req peeringRequest) error {
	if !asnPattern.MatchString(req.ASN) {
		return errors.New("ASN must look like AS4242421234.")
	}
	if req.NetworkName == "" {
		return errors.New("Network name is required.")
	}
	if req.Contact == "" {
		return errors.New("Contact information is required.")
	}
	if req.Endpoint == "" {
		return errors.New("Endpoint is required. Use a hostname/IP and UDP port, or describe an outbound-only arrangement.")
	}
	if err := validateWireGuardPublicKey(req.WireGuardPublicKey); err != nil {
		return err
	}
	return nil
}

func validateWireGuardPublicKey(value string) error {
	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return errors.New("WireGuard public key must be a valid 32-byte base64 public key.")
	}
	return nil
}

func (a *app) publishNtfyMessage(
	parent context.Context,
	title string,
	tags string,
	body string,
) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		a.ntfyURL,
		strings.NewReader(body),
	)
	if err != nil {
		return err
	}

	req.Header.Set("Title", title)
	req.Header.Set("Priority", "default")
	req.Header.Set("Tags", tags)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	if a.ntfyToken != "" {
		req.Header.Set("Authorization", "Bearer "+a.ntfyToken)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ntfy returned HTTP %d", resp.StatusCode)
	}

	return nil
}

func (l *globalRateLimiter) allow() bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if !l.lastSeen.IsZero() && now.Sub(l.lastSeen) < l.interval {
		return false
	}

	l.lastSeen = now
	return true
}

func requestLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)

		if r.Method == http.MethodGet && r.URL.Path == "/" {
			payload := makeMissionControlVisitPayload(r)
			go publishMissionControlVisit(payload)
		}

		if r.URL.Path != "/whoami" && r.URL.Path != "/api/whoami" {
			log.Printf(
				"%s %s remote=%s ua=%q duration=%s",
				r.Method,
				r.URL.Path,
				requestRemoteHost(r),
				r.UserAgent(),
				time.Since(start).Round(time.Millisecond),
			)
		}
	})
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

// requestRemoteHost returns the original client address when a request came
// through a trusted local reverse proxy.
//
// X-Real-IP is ignored unless the immediate TCP peer is loopback, so direct
// clients cannot spoof their source address by supplying the header themselves.
func requestRemoteHost(r *http.Request) string {
	immediate := remoteHost(r.RemoteAddr)

	peerIP := net.ParseIP(immediate)
	if peerIP == nil || !peerIP.IsLoopback() {
		return immediate
	}

	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	parsed := net.ParseIP(realIP)
	if parsed == nil {
		return immediate
	}

	return parsed.String()
}

func clean(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		value = value[:max]
	}
	return value
}

func envOrDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func envOrConfigured(name, fallback string) string {
	value, present := os.LookupEnv(name)
	if !present {
		return fallback
	}
	return strings.TrimSpace(value)
}
