package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPProviderVerifierVerifiesGoogleProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer google-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{
			"sub":"google-subject",
			"email":"user@example.com",
			"email_verified":true,
			"name":"User Name",
			"picture":"https://example.com/avatar.png"
		}`))
	}))
	defer server.Close()

	verifier := HTTPProviderVerifier{
		Client:            server.Client(),
		GoogleUserInfoURL: server.URL,
		KakaoUserInfoURL:  server.URL,
	}

	profile, err := verifier.Verify(context.Background(), ProviderGoogle, "google-token")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if profile.Provider != ProviderGoogle || profile.Subject != "google-subject" {
		t.Fatalf("profile = %+v", profile)
	}
	if profile.Email != "user@example.com" || !profile.EmailVerified {
		t.Fatalf("email = %q verified=%v", profile.Email, profile.EmailVerified)
	}
}

func TestHTTPProviderVerifierVerifiesKakaoProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer kakao-token" {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{
			"id":12345,
			"kakao_account":{
				"email":"kakao@example.com",
				"is_email_verified":true,
				"profile":{
					"nickname":"Kakao User",
					"profile_image_url":"https://example.com/kakao.png"
				}
			}
		}`))
	}))
	defer server.Close()

	verifier := HTTPProviderVerifier{
		Client:            server.Client(),
		GoogleUserInfoURL: server.URL,
		KakaoUserInfoURL:  server.URL,
	}

	profile, err := verifier.Verify(context.Background(), ProviderKakao, "kakao-token")
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if profile.Provider != ProviderKakao || profile.Subject != "12345" {
		t.Fatalf("profile = %+v", profile)
	}
	if profile.DisplayName != "Kakao User" {
		t.Fatalf("DisplayName = %q", profile.DisplayName)
	}
}

func TestHTTPProviderVerifierRejectsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad token", http.StatusUnauthorized)
	}))
	defer server.Close()

	verifier := HTTPProviderVerifier{
		Client:            server.Client(),
		GoogleUserInfoURL: server.URL,
		KakaoUserInfoURL:  server.URL,
	}

	_, err := verifier.Verify(context.Background(), ProviderGoogle, "bad-token")
	if err == nil {
		t.Fatal("Verify() error = nil, want provider error")
	}
}
