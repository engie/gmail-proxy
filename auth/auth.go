package auth

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-oauth2/oauth2/v4/errors"
	"github.com/go-oauth2/oauth2/v4/generates"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/models"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-oauth2/oauth2/v4/store"
	"github.com/golang-jwt/jwt/v5"
)

type ClientConfig struct {
	ID     string `json:"id"`
	Secret string `json:"secret"`
}

type Auth struct {
	srv *server.Server
}

func New(secret string, clientsJSON string) (*Auth, error) {
	var clients []ClientConfig
	if err := json.Unmarshal([]byte(clientsJSON), &clients); err != nil {
		return nil, fmt.Errorf("parsing OAUTH_CLIENTS: %w", err)
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("OAUTH_CLIENTS must contain at least one client")
	}

	manager := manage.NewDefaultManager()
	manager.MustTokenStorage(store.NewMemoryTokenStore())
	manager.MapAccessGenerate(generates.NewJWTAccessGenerate("", []byte(secret), jwt.SigningMethodHS512))

	clientStore := store.NewClientStore()
	for _, c := range clients {
		if err := clientStore.Set(c.ID, &models.Client{
			ID:     c.ID,
			Secret: c.Secret,
		}); err != nil {
			return nil, fmt.Errorf("registering client %q: %w", c.ID, err)
		}
	}
	manager.MapClientStorage(clientStore)

	srv := server.NewDefaultServer(manager)
	srv.SetAllowGetAccessRequest(false)
	srv.SetClientInfoHandler(server.ClientFormHandler)

	srv.SetInternalErrorHandler(func(err error) *errors.Response {
		log.Printf("oauth internal error: %v", err)
		return nil
	})
	srv.SetResponseErrorHandler(func(re *errors.Response) {
		log.Printf("oauth response error: %v", re.Error)
	})

	return &Auth{srv: srv}, nil
}

func (a *Auth) TokenHandler(w http.ResponseWriter, r *http.Request) {
	if err := a.srv.HandleTokenRequest(w, r); err != nil {
		log.Printf("token request error: %v", err)
	}
}

func (a *Auth) Protect(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := a.srv.ValidationBearerToken(r)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	})
}
