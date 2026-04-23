package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"
)

// AuthHandler handles authentication verification for Traefik ForwardAuth
type AuthHandler struct {
	logger zerolog.Logger
	token  string
}

func NewAuthHandler(logger zerolog.Logger, token string) *AuthHandler {
	return &AuthHandler{
		logger: logger.With().Str("component", "auth").Logger(),
		token:  token,
	}
}

func (h *AuthHandler) RegisterRoutes(r chi.Router) {
	r.Get("/auth/verify", h.Verify)
}

// Verify checks the Authorization header and returns 200 if valid, 401 if not
func (h *AuthHandler) Verify(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	
	// Check for Bearer token
	expectedAuth := "Bearer " + h.token
	
	if authHeader != expectedAuth {
		h.logger.Warn().
			Str("remote_addr", r.RemoteAddr).
			Str("path", r.URL.Path).
			Msg("unauthorized request")
		
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Extract runner slug from query or path if present (for future use)
	runnerSlug := r.URL.Query().Get("runner_slug")
	if runnerSlug != "" {
		w.Header().Set("X-Runner-Slug", runnerSlug)
	}

	h.logger.Debug().
		Str("remote_addr", r.RemoteAddr).
		Msg("request authorized")

	w.WriteHeader(http.StatusOK)
}
