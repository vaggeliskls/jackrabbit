package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func Stack() func(http.Handler) http.Handler {
	return chi.Chain(
		middleware.RequestID,
		RequestLogger,
		middleware.Recoverer,
		middleware.Timeout(30*time.Second),
	).Handler
}

// StackNoTimeout returns middleware chain without timeout (for SSE and long-lived connections)
func StackNoTimeout() func(http.Handler) http.Handler {
	return chi.Chain(
		middleware.RequestID,
		RequestLogger,
		middleware.Recoverer,
	).Handler
}

func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := middleware.GetReqID(r.Context())

		logger := log.With().
			Str("request_id", reqID).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_addr", r.RemoteAddr).
			Logger()

		ctx := logger.WithContext(r.Context())
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r.WithContext(ctx))

		logger.Info().
			Int("status", ww.Status()).
			Int("bytes", ww.BytesWritten()).
			Dur("duration_ms", time.Since(start)).
			Msg("request completed")
	})
}

func GetLogger(ctx context.Context) *zerolog.Logger {
	logger := zerolog.Ctx(ctx)
	return logger
}
