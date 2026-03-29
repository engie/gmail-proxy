package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/stephen/gmail-proxy/auth"
	"github.com/stephen/gmail-proxy/gmailproxy"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx := context.Background()

	// Initialize Gmail service.
	gmailSvc, err := gmailproxy.NewGmailService(ctx, cfg.GmailTokenJSON)
	if err != nil {
		log.Fatalf("gmail init error: %v", err)
	}
	log.Println("gmail service initialized")

	// Resolve label name to ID.
	labelID, err := gmailproxy.ResolveLabelID(gmailSvc, cfg.AllowedLabel)
	if err != nil {
		log.Fatalf("label resolution error: %v", err)
	}
	log.Printf("resolved label %q to ID %q", cfg.AllowedLabel, labelID)

	// Initialize OAuth2 auth server.
	authServer, err := auth.New(cfg.OAuthSecret, cfg.OAuthClients)
	if err != nil {
		log.Fatalf("oauth init error: %v", err)
	}
	log.Println("oauth server initialized")

	proxy := gmailproxy.NewProxy(gmailSvc, labelID)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /oauth/token", authServer.TokenHandler)
	mux.Handle("GET /api/messages", authServer.Protect(proxy.ListMessages))
	mux.Handle("GET /api/messages/{id}", authServer.Protect(proxy.GetMessage))
	mux.Handle("GET /api/messages/{id}/attachments/{attachmentId}", authServer.Protect(proxy.GetAttachment))
	mux.Handle("POST /api/drafts", authServer.Protect(proxy.CreateDraft))
	mux.Handle("GET /api/labels", authServer.Protect(proxy.ListLabels))

	addr := ":" + cfg.Port
	log.Printf("starting gmail proxy on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

type config struct {
	GmailTokenJSON string
	OAuthSecret    string
	OAuthClients   string
	AllowedLabel   string
	Port           string
}

func loadConfig() (*config, error) {
	cfg := &config{
		GmailTokenJSON: os.Getenv("GMAIL_TOKEN_JSON"),
		OAuthSecret:    os.Getenv("OAUTH_SECRET"),
		OAuthClients:   os.Getenv("OAUTH_CLIENTS"),
		AllowedLabel:   os.Getenv("ALLOWED_LABEL"),
		Port:           os.Getenv("PORT"),
	}

	if cfg.GmailTokenJSON == "" {
		return nil, fmt.Errorf("GMAIL_TOKEN_JSON is required")
	}
	if cfg.OAuthSecret == "" {
		return nil, fmt.Errorf("OAUTH_SECRET is required")
	}
	if cfg.OAuthClients == "" {
		return nil, fmt.Errorf("OAUTH_CLIENTS is required")
	}
	if cfg.AllowedLabel == "" {
		return nil, fmt.Errorf("ALLOWED_LABEL is required")
	}
	if cfg.Port == "" {
		cfg.Port = "8080"
	}

	return cfg, nil
}
