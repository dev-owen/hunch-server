package httpserver

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var errNotReady = errors.New("not ready")

type ReadinessCheck func(*http.Request) error

type Dependencies struct {
	ReadinessCheck ReadinessCheck

	AccountSignup  http.Handler
	AccountSignin  http.Handler
	AccountSignout http.Handler
	AccountDelete  http.Handler
}

func NewRouter(deps Dependencies) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Recoverer)

	router.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	router.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if deps.ReadinessCheck != nil {
			if err := deps.ReadinessCheck(r); err != nil {
				http.Error(w, "not ready", http.StatusServiceUnavailable)
				return
			}
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})

	if deps.AccountSignup != nil {
		router.Method(http.MethodPost, "/signup", deps.AccountSignup)
	}
	if deps.AccountSignin != nil {
		router.Method(http.MethodPost, "/signin", deps.AccountSignin)
	}
	if deps.AccountSignout != nil {
		router.Method(http.MethodPost, "/signout", deps.AccountSignout)
	}
	if deps.AccountDelete != nil {
		router.Method(http.MethodDelete, "/delete", deps.AccountDelete)
	}

	router.Handle("/metrics", promhttp.Handler())

	return router
}
