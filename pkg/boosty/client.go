package boosty

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

const (
	BaseURL   = "https://api.boosty.to"
	UserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

// Client is a Boosty API HTTP client with automatic token refresh.
type Client struct {
	http     *http.Client
	auth     AuthData
	authLock sync.Mutex

	// OnTokenRefresh is called when the access token is refreshed.
	// The bridge should persist the new AuthData.
	OnTokenRefresh func(ctx context.Context, auth AuthData)
}

// NewClient creates a new Boosty API client.
func NewClient(auth AuthData) *Client {
	return &Client{
		http: &http.Client{Timeout: 30 * time.Second},
		auth: auth,
	}
}

// Auth returns the current authentication data.
func (c *Client) Auth() AuthData {
	c.authLock.Lock()
	defer c.authLock.Unlock()
	return c.auth
}

// SetAuth updates the authentication data.
func (c *Client) SetAuth(auth AuthData) {
	c.authLock.Lock()
	defer c.authLock.Unlock()
	c.auth = auth
}

// IsLoggedIn checks if the client has valid credentials.
func (c *Client) IsLoggedIn() bool {
	c.authLock.Lock()
	defer c.authLock.Unlock()
	return c.auth.AccessToken != ""
}

func (c *Client) headers() http.Header {
	c.authLock.Lock()
	defer c.authLock.Unlock()
	h := http.Header{}
	if c.auth.AccessToken != "" {
		h.Set("Authorization", "Bearer "+c.auth.AccessToken)
	}
	h.Set("User-Agent", UserAgent)
	h.Set("X-App", "web")
	if c.auth.DeviceID != "" {
		h.Set("X-From-Id", c.auth.DeviceID)
	}
	h.Set("X-Locale", "en_US")
	h.Set("X-Currency", "USD")
	return h
}

// doRequest executes an HTTP request with auth headers and auto-refreshes on 401.
func (c *Client) doRequest(ctx context.Context, method, path string, body io.Reader, contentType string) (*http.Response, error) {
	log := zerolog.Ctx(ctx)

	// Buffer the body so we can replay it on 401 retry.
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
	}

	reqURL := BaseURL + path

	doOnce := func() (*http.Response, error) {
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header = c.headers()
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		log.Debug().Str("method", method).Str("url", reqURL).Msg("Boosty API request")
		return c.http.Do(req)
	}

	resp, err := doOnce()
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()
		log.Warn().Msg("Got 401, attempting token refresh")
		if refreshErr := c.RefreshToken(ctx); refreshErr != nil {
			return nil, fmt.Errorf("token refresh failed: %w", refreshErr)
		}
		resp, err = doOnce()
		if err != nil {
			return nil, fmt.Errorf("request failed after token refresh: %w", err)
		}
	}

	return resp, nil
}

// doJSON executes a request and decodes the JSON response.
func (c *Client) doJSON(ctx context.Context, method, path string, body io.Reader, contentType string, out any) error {
	resp, err := c.doRequest(ctx, method, path, body, contentType)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// doFormPost sends a POST with application/x-www-form-urlencoded body.
func (c *Client) doFormPost(ctx context.Context, path string, data url.Values, out any) error {
	return c.doJSON(ctx, http.MethodPost, path, strings.NewReader(data.Encode()), "application/x-www-form-urlencoded", out)
}

// RefreshToken refreshes the access token using the refresh token.
func (c *Client) RefreshToken(ctx context.Context) error {
	c.authLock.Lock()
	refreshToken := c.auth.RefreshToken
	deviceID := c.auth.DeviceID
	c.authLock.Unlock()

	if refreshToken == "" {
		return fmt.Errorf("no refresh token available")
	}

	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"device_id":     {deviceID},
		"device_os":     {"web"},
	}

	var authResp AuthResponse
	if err := c.doFormPost(ctx, "/oauth/token", data, &authResp); err != nil {
		return fmt.Errorf("token refresh failed: %w", err)
	}

	c.authLock.Lock()
	c.auth.AccessToken = authResp.AccessToken
	c.auth.RefreshToken = authResp.RefreshToken
	c.auth.ExpiresAt = time.Now().Add(time.Duration(authResp.ExpiresIn) * time.Second)
	newAuth := c.auth
	c.authLock.Unlock()

	if c.OnTokenRefresh != nil {
		c.OnTokenRefresh(ctx, newAuth)
	}

	zerolog.Ctx(ctx).Info().Msg("Boosty token refreshed successfully")
	return nil
}

// GetCurrentUser fetches the current user info.
func (c *Client) GetCurrentUser(ctx context.Context) (*UserInfo, error) {
	var user UserInfo
	if err := c.doJSON(ctx, http.MethodGet, "/v1/user/current", nil, "", &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// DownloadAvatar downloads an avatar image from the given URL.
func (c *Client) DownloadAvatar(ctx context.Context, avatarURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, avatarURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create avatar request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download avatar: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("avatar download failed with status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// DownloadMedia performs an authenticated GET request against any URL and returns the response.
// The caller is responsible for closing the response body.
func (c *Client) DownloadMedia(ctx context.Context, mediaURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create media request: %w", err)
	}
	req.Header = c.headers()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download media: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("media download failed with status %d", resp.StatusCode)
	}
	return resp, nil
}
