package gmailproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type GmailToken struct {
	Token        string   `json:"token"`
	RefreshToken string   `json:"refresh_token"`
	TokenURI     string   `json:"token_uri"`
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"`
	Scopes       []string `json:"scopes"`
	Expiry       string   `json:"expiry"`
}

func NewGmailService(ctx context.Context, tokenJSON string) (*gmail.Service, error) {
	var gt GmailToken
	if err := json.Unmarshal([]byte(tokenJSON), &gt); err != nil {
		return nil, fmt.Errorf("parsing gmail token json: %w", err)
	}

	if gt.RefreshToken == "" {
		return nil, fmt.Errorf("gmail token json missing refresh_token")
	}
	if gt.ClientID == "" || gt.ClientSecret == "" {
		return nil, fmt.Errorf("gmail token json missing client_id or client_secret")
	}

	conf := &oauth2.Config{
		ClientID:     gt.ClientID,
		ClientSecret: gt.ClientSecret,
		Endpoint: oauth2.Endpoint{
			TokenURL: gt.TokenURI,
		},
		Scopes: gt.Scopes,
	}

	var expiry time.Time
	if gt.Expiry != "" {
		if t, err := time.Parse(time.RFC3339, gt.Expiry); err == nil {
			expiry = t
		}
	}

	tok := &oauth2.Token{
		AccessToken:  gt.Token,
		RefreshToken: gt.RefreshToken,
		Expiry:       expiry,
		TokenType:    "Bearer",
	}

	client := conf.Client(ctx, tok)
	return gmail.NewService(ctx, option.WithHTTPClient(client))
}
