package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var scopes = []string{
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/gmail.compose",
}

type clientSecretFile struct {
	Installed struct {
		ClientID     string   `json:"client_id"`
		ClientSecret string   `json:"client_secret"`
		AuthURI      string   `json:"auth_uri"`
		TokenURI     string   `json:"token_uri"`
		RedirectURIs []string `json:"redirect_uris"`
	} `json:"installed"`
}

type tokenFile struct {
	Token        string   `json:"token"`
	RefreshToken string   `json:"refresh_token"`
	TokenURI     string   `json:"token_uri"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
	Expiry       string   `json:"expiry"`
}

func main() {
	csPath := "client_secret.json"
	outPath := "token.json"

	if len(os.Args) > 1 {
		csPath = os.Args[1]
	}
	if len(os.Args) > 2 {
		outPath = os.Args[2]
	}

	data, err := os.ReadFile(csPath)
	if err != nil {
		log.Fatalf("reading %s: %v", csPath, err)
	}

	var cs clientSecretFile
	if err := json.Unmarshal(data, &cs); err != nil {
		log.Fatalf("parsing %s: %v", csPath, err)
	}

	conf := &oauth2.Config{
		ClientID:     cs.Installed.ClientID,
		ClientSecret: cs.Installed.ClientSecret,
		Endpoint:     google.Endpoint,
		Scopes:       scopes,
		RedirectURL:  "http://localhost:8085/callback",
	}

	codeCh := make(chan string, 1)
	srv := &http.Server{Addr: ":8085"}

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "no code in callback", http.StatusBadRequest)
			return
		}
		codeCh <- code
		fmt.Fprintln(w, "Authorization complete. You can close this tab.")
	})

	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("callback server: %v", err)
		}
	}()

	url := conf.AuthCodeURL("state", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Printf("\nOpen this URL in your browser:\n\n%s\n\nWaiting for callback...\n", url)

	code := <-codeCh

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tok, err := conf.Exchange(ctx, code)
	if err != nil {
		log.Fatalf("token exchange failed: %v", err)
	}

	srv.Shutdown(ctx)

	tf := tokenFile{
		Token:        tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		TokenURI:     conf.Endpoint.TokenURL,
		ClientID:     conf.ClientID,
		ClientSecret: conf.ClientSecret,
		Scopes:       scopes,
		Expiry:       tok.Expiry.Format(time.RFC3339),
	}

	out, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		log.Fatalf("marshaling token: %v", err)
	}

	if err := os.WriteFile(outPath, out, 0600); err != nil {
		log.Fatalf("writing %s: %v", outPath, err)
	}

	fmt.Printf("\nToken written to %s with scopes: %v\n", outPath, scopes)
}
