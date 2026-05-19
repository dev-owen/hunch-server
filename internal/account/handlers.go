package account

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

const maxRequestBodyBytes = 1 << 20

type HandlerConfig struct {
	CookieName string
	CookieTTL  time.Duration
	Secure     bool
}

type Handlers struct {
	service *Service
	config  HandlerConfig
}

func NewHandlers(service *Service, config HandlerConfig) *Handlers {
	return &Handlers{service: service, config: config}
}

type signupRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type signinRequest struct {
	Method      Provider `json:"method"`
	Email       string   `json:"email"`
	Password    string   `json:"password"`
	AccessToken string   `json:"access_token"`
}

type accountResponse struct {
	Account Account `json:"account"`
}

func (h *Handlers) Signup(w http.ResponseWriter, r *http.Request) {
	var body signupRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	result, err := h.service.SignupEmail(r.Context(), SignupEmailInput{
		Email:       body.Email,
		Password:    body.Password,
		DisplayName: body.DisplayName,
		UserAgent:   r.UserAgent(),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	h.setSessionCookie(w, result.Session.RawToken)
	writeJSON(w, http.StatusCreated, accountResponse{Account: result.Account})
}

func (h *Handlers) Signin(w http.ResponseWriter, r *http.Request) {
	var body signinRequest
	if !decodeJSON(w, r, &body) {
		return
	}
	result, err := h.service.Signin(r.Context(), SigninInput{
		Method:      body.Method,
		Email:       body.Email,
		Password:    body.Password,
		AccessToken: body.AccessToken,
		UserAgent:   r.UserAgent(),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	h.setSessionCookie(w, result.Session.RawToken)
	writeJSON(w, http.StatusOK, accountResponse{Account: result.Account})
}

func (h *Handlers) Signout(w http.ResponseWriter, r *http.Request) {
	rawToken, ok := h.sessionTokenFromRequest(r)
	if !ok {
		writeError(w, ErrUnauthenticated)
		return
	}
	if err := h.service.Signout(r.Context(), rawToken); err != nil {
		writeError(w, err)
		return
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	rawToken, ok := h.sessionTokenFromRequest(r)
	if !ok {
		writeError(w, ErrUnauthenticated)
		return
	}
	account, err := h.service.Authenticate(r.Context(), rawToken)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := h.service.DeleteAccount(r.Context(), account.ID); err != nil {
		writeError(w, err)
		return
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) sessionTokenFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(h.config.CookieName)
	if err != nil || cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func (h *Handlers) setSessionCookie(w http.ResponseWriter, rawToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.config.CookieName,
		Value:    rawToken,
		Path:     "/",
		MaxAge:   int(h.config.CookieTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.config.Secure,
	})
}

func (h *Handlers) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     h.config.CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   h.config.Secure,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBodyBytes))
	if err := decoder.Decode(target); err != nil {
		writeError(w, ErrInvalidInput)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, ErrInvalidInput)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, ErrEmailAlreadyExists):
		status = http.StatusConflict
	case errors.Is(err, ErrInvalidCredentials), errors.Is(err, ErrUnauthenticated):
		status = http.StatusUnauthorized
	case errors.Is(err, ErrProviderUnavailable):
		status = http.StatusBadGateway
	}
	http.Error(w, http.StatusText(status), status)
}
