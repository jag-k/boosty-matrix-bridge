package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/bridgev2/database"
	"maunium.net/go/mautrix/bridgev2/networkid"

	"github.com/jag-k/boosty-matrix-bridge/pkg/boosty"
)

const (
	FlowIDCookies    = "cookies"
	FlowIDCookieJSON = "cookie_json"

	LoginStepIDCookies    = "fi.mau.boosty.login.cookies"
	LoginStepIDCookieJSON = "fi.mau.boosty.login.cookie_json"
	LoginStepIDComplete   = "fi.mau.boosty.login.complete"

	CookieFieldAuthCookie = "fi.mau.boosty.login.auth_cookie"
	CookieFieldDeviceID   = "fi.mau.boosty.login.device_id"
)

func (bc *BoostyConnector) GetLoginFlows() []bridgev2.LoginFlow {
	return []bridgev2.LoginFlow{
		{
			Name:        "Browser",
			Description: "Log in by extracting the auth cookie from browser (open boosty.to in webview)",
			ID:          FlowIDCookies,
		},
		{
			Name:        "Cookie JSON",
			Description: "Log in by pasting the auth cookie JSON from browser DevTools",
			ID:          FlowIDCookieJSON,
		},
	}
}

func (bc *BoostyConnector) CreateLogin(ctx context.Context, user *bridgev2.User, flowID string) (bridgev2.LoginProcess, error) {
	switch flowID {
	case FlowIDCookies:
		return &CookieLogin{user: user, main: bc}, nil
	case FlowIDCookieJSON:
		return &CookieJSONLogin{user: user, main: bc}, nil
	default:
		return nil, fmt.Errorf("unknown login flow ID: %s", flowID)
	}
}

// CookieJSONLogin implements bridgev2.LoginProcessUserInput for manual JSON cookie entry.
// The user pastes the auth cookie JSON from browser DevTools.
type CookieJSONLogin struct {
	user *bridgev2.User
	main *BoostyConnector
}

var _ bridgev2.LoginProcessUserInput = (*CookieJSONLogin)(nil)

func (c *CookieJSONLogin) Cancel() {}

func (c *CookieJSONLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return &bridgev2.LoginStep{
		Type:   bridgev2.LoginStepTypeUserInput,
		StepID: LoginStepIDCookieJSON,
		Instructions: "Paste the auth cookie JSON from browser DevTools (Application tab → Cookies → boosty.to → auth). " +
			"Optionally paste the _clientId cookie value as device ID.",
		UserInputParams: &bridgev2.LoginUserInputParams{
			Fields: []bridgev2.LoginInputDataField{
				{
					ID:   "auth_json",
					Name: "Auth Cookie JSON",
				},
				{
					ID:   "device_id",
					Name: "Device ID (optional, from _clientId cookie)",
				},
			},
		},
	}, nil
}

func (c *CookieJSONLogin) SubmitUserInput(ctx context.Context, input map[string]string) (*bridgev2.LoginStep, error) {
	authJSON := input["auth_json"]
	if authJSON == "" {
		return nil, fmt.Errorf("auth_json is required")
	}

	type AuthCookie struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
		IsNewUser    bool   `json:"isNewUser"`
	}

	var auth AuthCookie
	if err := json.Unmarshal([]byte(authJSON), &auth); err != nil {
		return nil, fmt.Errorf("failed to parse auth JSON: %w", err)
	}

	if auth.AccessToken == "" {
		return nil, fmt.Errorf("accessToken not found in auth JSON")
	}

	deviceID := input["device_id"]
	if deviceID == "" {
		deviceID = "bridge-" + strconv.Itoa(int(time.Now().UnixMilli()))
	}

	var expiresAt time.Time
	if auth.ExpiresAt > 0 {
		expiresAt = time.UnixMilli(auth.ExpiresAt)
	}

	client := boosty.NewClient(boosty.AuthData{
		AccessToken:  auth.AccessToken,
		RefreshToken: auth.RefreshToken,
		DeviceID:     deviceID,
		ExpiresAt:    expiresAt,
	})

	// Verify the token works by fetching the current user
	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	userIDStr := strconv.Itoa(user.ID)
	remoteName := user.Name
	if remoteName == "" {
		remoteName = user.Email
	}

	ul, err := c.user.NewLogin(ctx, &database.UserLogin{
		ID:         networkid.UserLoginID(userIDStr),
		RemoteName: remoteName,
		Metadata: &UserLoginMetadata{
			AuthData: client.Auth(),
		},
	}, &bridgev2.NewLoginParams{DeleteOnConflict: true})
	if err != nil {
		return nil, fmt.Errorf("failed to save login: %w", err)
	}

	ul.Client.Connect(ctx)

	return &bridgev2.LoginStep{
		Type:           bridgev2.LoginStepTypeComplete,
		StepID:         LoginStepIDComplete,
		Instructions:   fmt.Sprintf("Successfully logged in as %s", remoteName),
		CompleteParams: &bridgev2.LoginCompleteParams{UserLoginID: ul.ID, UserLogin: ul},
	}, nil
}

// CookieLogin implements bridgev2.LoginProcessCookies for browser-based auth.
// The user opens boosty.to in a webview, and the bridge extracts the auth cookie.
type CookieLogin struct {
	user *bridgev2.User
	main *BoostyConnector
}

var _ bridgev2.LoginProcessCookies = (*CookieLogin)(nil)

func (c *CookieLogin) Cancel() {}

func (c *CookieLogin) Start(ctx context.Context) (*bridgev2.LoginStep, error) {
	return &bridgev2.LoginStep{
		Type:         bridgev2.LoginStepTypeCookies,
		StepID:       LoginStepIDCookies,
		Instructions: "Log in to Boosty in the browser. The bridge will extract your auth cookie.",
		CookiesParams: &bridgev2.LoginCookiesParams{
			URL:       "https://boosty.to",
			UserAgent: boosty.UserAgent,
			Fields: []bridgev2.LoginCookieField{
				{
					ID:       CookieFieldAuthCookie,
					Required: true,
					Sources: []bridgev2.LoginCookieFieldSource{
						{
							Type:         bridgev2.LoginCookieTypeCookie,
							Name:         "auth",
							CookieDomain: ".boosty.to",
						},
					},
				},
				{
					ID:       CookieFieldDeviceID,
					Required: false,
					Sources: []bridgev2.LoginCookieFieldSource{
						{
							Type:         bridgev2.LoginCookieTypeCookie,
							Name:         "_clientId",
							CookieDomain: ".boosty.to",
						},
					},
				},
			},
			WaitForURLPattern: "^https://boosty\\.to/(?:\\?.*)?$",
		},
	}, nil
}

func (c *CookieLogin) SubmitCookies(ctx context.Context, cookies map[string]string) (*bridgev2.LoginStep, error) {
	authCookie := cookies[CookieFieldAuthCookie]
	if authCookie == "" {
		return nil, fmt.Errorf("auth cookie is required")
	}

	type AuthCookie struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresAt    int64  `json:"expiresAt"`
		IsNewUser    bool   `json:"isNewUser"`
	}

	var auth AuthCookie
	if err := json.Unmarshal([]byte(authCookie), &auth); err != nil {
		return nil, fmt.Errorf("failed to parse auth cookie: %w", err)
	}

	if auth.AccessToken == "" {
		return nil, fmt.Errorf("accessToken not found in auth cookie")
	}

	deviceID := cookies[CookieFieldDeviceID]
	if deviceID == "" {
		deviceID = "bridge-" + strconv.Itoa(int(time.Now().UnixMilli()))
	}

	var expiresAt time.Time
	if auth.ExpiresAt > 0 {
		expiresAt = time.UnixMilli(auth.ExpiresAt)
	}

	client := boosty.NewClient(boosty.AuthData{
		AccessToken:  auth.AccessToken,
		RefreshToken: auth.RefreshToken,
		DeviceID:     deviceID,
		ExpiresAt:    expiresAt,
	})

	// Verify the token works by fetching the current user
	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	userIDStr := strconv.Itoa(user.ID)
	remoteName := user.Name
	if remoteName == "" {
		remoteName = user.Email
	}

	ul, err := c.user.NewLogin(ctx, &database.UserLogin{
		ID:         networkid.UserLoginID(userIDStr),
		RemoteName: remoteName,
		Metadata: &UserLoginMetadata{
			AuthData: client.Auth(),
		},
	}, &bridgev2.NewLoginParams{DeleteOnConflict: true})
	if err != nil {
		return nil, fmt.Errorf("failed to save login: %w", err)
	}

	ul.Client.Connect(ctx)

	return &bridgev2.LoginStep{
		Type:           bridgev2.LoginStepTypeComplete,
		StepID:         LoginStepIDComplete,
		Instructions:   fmt.Sprintf("Successfully logged in as %s", remoteName),
		CompleteParams: &bridgev2.LoginCompleteParams{UserLoginID: ul.ID, UserLogin: ul},
	}, nil
}
