package account

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlersSignupSetsSessionCookie(t *testing.T) {
	service := newTestService()
	handlers := NewHandlers(service, HandlerConfig{
		CookieName: "hunch_session",
		CookieTTL:  time.Hour,
		Secure:     false,
	})

	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(`{
		"email":"user@example.com",
		"password":"password123",
		"display_name":"User"
	}`))
	rec := httptest.NewRecorder()

	handlers.Signup(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if got := rec.Result().Cookies(); len(got) != 1 || got[0].Name != "hunch_session" {
		t.Fatalf("cookies = %+v", got)
	}
}

func TestHandlersSigninRejectsBadCredentials(t *testing.T) {
	service := newTestService()
	handlers := NewHandlers(service, HandlerConfig{CookieName: "hunch_session", CookieTTL: time.Hour})

	req := httptest.NewRequest(http.MethodPost, "/signin", strings.NewReader(`{
		"method":"email",
		"email":"missing@example.com",
		"password":"password123"
	}`))
	rec := httptest.NewRecorder()

	handlers.Signin(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandlersSignoutClearsCookie(t *testing.T) {
	service := newTestService()
	handlers := NewHandlers(service, HandlerConfig{CookieName: "hunch_session", CookieTTL: time.Hour})

	signupReq := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(`{
		"email":"user@example.com",
		"password":"password123"
	}`))
	signupRec := httptest.NewRecorder()
	handlers.Signup(signupRec, signupReq)
	cookie := signupRec.Result().Cookies()[0]

	req := httptest.NewRequest(http.MethodPost, "/signout", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handlers.Signout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if clear := rec.Result().Cookies()[0]; clear.MaxAge != -1 {
		t.Fatalf("clear cookie MaxAge = %d, want -1", clear.MaxAge)
	}
}

func newTestService() *Service {
	return NewService(Config{
		Repository: newFakeRepository(),
		Verifier:   fakeVerifier{},
		BcryptCost: 4,
		SessionTTL: time.Hour,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
	})
}
