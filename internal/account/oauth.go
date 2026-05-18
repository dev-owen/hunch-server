package account

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

type ProviderVerifier interface {
	Verify(ctx context.Context, provider Provider, accessToken string) (ExternalProfile, error)
}

type HTTPProviderVerifier struct {
	Client            *http.Client
	GoogleUserInfoURL string
	KakaoUserInfoURL  string
}

func (v HTTPProviderVerifier) Verify(ctx context.Context, provider Provider, accessToken string) (ExternalProfile, error) {
	if accessToken == "" {
		return ExternalProfile{}, fmt.Errorf("%w: access token is required", ErrInvalidInput)
	}
	switch provider {
	case ProviderGoogle:
		return v.verifyGoogle(ctx, accessToken)
	case ProviderKakao:
		return v.verifyKakao(ctx, accessToken)
	default:
		return ExternalProfile{}, fmt.Errorf("%w: unsupported provider", ErrInvalidInput)
	}
}

func (v HTTPProviderVerifier) httpClient() *http.Client {
	if v.Client != nil {
		return v.Client
	}
	return http.DefaultClient
}

func (v HTTPProviderVerifier) getJSON(ctx context.Context, url string, accessToken string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := v.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%w: status %d", ErrInvalidCredentials, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("%w: decode profile", ErrProviderUnavailable)
	}
	return nil
}

func (v HTTPProviderVerifier) verifyGoogle(ctx context.Context, accessToken string) (ExternalProfile, error) {
	var body struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}
	if err := v.getJSON(ctx, v.GoogleUserInfoURL, accessToken, &body); err != nil {
		return ExternalProfile{}, err
	}
	if body.Sub == "" {
		return ExternalProfile{}, fmt.Errorf("%w: google subject is required", ErrInvalidCredentials)
	}
	return ExternalProfile{
		Provider:      ProviderGoogle,
		Subject:       body.Sub,
		Email:         body.Email,
		EmailVerified: body.EmailVerified,
		DisplayName:   body.Name,
		AvatarURL:     body.Picture,
	}, nil
}

func (v HTTPProviderVerifier) verifyKakao(ctx context.Context, accessToken string) (ExternalProfile, error) {
	var body struct {
		ID           int64 `json:"id"`
		KakaoAccount struct {
			Email           string `json:"email"`
			IsEmailVerified bool   `json:"is_email_verified"`
			Profile         struct {
				Nickname        string `json:"nickname"`
				ProfileImageURL string `json:"profile_image_url"`
			} `json:"profile"`
		} `json:"kakao_account"`
	}
	if err := v.getJSON(ctx, v.KakaoUserInfoURL, accessToken, &body); err != nil {
		return ExternalProfile{}, err
	}
	if body.ID == 0 {
		return ExternalProfile{}, fmt.Errorf("%w: kakao subject is required", ErrInvalidCredentials)
	}
	return ExternalProfile{
		Provider:      ProviderKakao,
		Subject:       strconv.FormatInt(body.ID, 10),
		Email:         body.KakaoAccount.Email,
		EmailVerified: body.KakaoAccount.IsEmailVerified,
		DisplayName:   body.KakaoAccount.Profile.Nickname,
		AvatarURL:     body.KakaoAccount.Profile.ProfileImageURL,
	}, nil
}
