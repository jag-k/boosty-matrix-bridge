package boosty

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestRefreshRejectionDoesNotRecurse(t *testing.T) {
	c := NewClient(AuthData{AccessToken: "old", RefreshToken: "bad"})
	var calls int
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls > 2 {
			t.Fatal("recursive refresh")
		}
		return response(401, `{}`), nil
	})
	if _, err := c.GetCurrentUser(context.Background()); err == nil {
		t.Fatal("expected authentication error")
	}
	if calls != 2 {
		t.Fatalf("got %d requests, want API + one refresh", calls)
	}
}

func TestConcurrentRefreshReplaysBodiesAndPersistsOnce(t *testing.T) {
	const count = 8
	c := NewClient(AuthData{AccessToken: "old", RefreshToken: "refresh", DeviceID: "device"})
	var oldRequests, refreshes, saves atomic.Int32
	ready := make(chan struct{})
	c.OnTokenRefresh = func(_ context.Context, auth AuthData) {
		saves.Add(1)
		if auth.AccessToken != "new" || auth.RefreshToken != "rotated" || auth.DeviceID != "device" {
			t.Error("incorrect persisted credentials")
		}
	}
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/oauth/token/" {
			refreshes.Add(1)
			return response(200, `{"access_token":"new","refresh_token":"rotated","expires_in":3600}`), nil
		}
		if string(body) != "payload" {
			t.Error("request body lost on retry")
		}
		if r.Header.Get("Authorization") == "Bearer old" {
			if oldRequests.Add(1) == count {
				close(ready)
			}
			<-ready
			return response(401, `{}`), nil
		}
		return response(200, `{}`), nil
	})
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			if err := c.doJSON(context.Background(), "POST", "/test", strings.NewReader("payload"), "text/plain", nil); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if refreshes.Load() != 1 || saves.Load() != 1 {
		t.Fatalf("refreshes=%d saves=%d", refreshes.Load(), saves.Load())
	}
}

func TestMediaAuthScopeAndRefresh(t *testing.T) {
	c := NewClient(AuthData{AccessToken: "old", RefreshToken: "refresh"})
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/oauth/token/" {
			return response(200, `{"access_token":"new","expires_in":3600}`), nil
		}
		if r.URL.Host == "api.boosty.to" {
			if r.Header.Get("Authorization") == "Bearer old" {
				return response(401, ``), nil
			}
			if r.Header.Get("Authorization") != "Bearer new" {
				return nil, fmt.Errorf("missing refreshed token")
			}
		} else if r.Header.Get("Authorization") != "" || r.Header.Get("X-From-Id") != "" {
			t.Error("credentials sent to CDN")
		}
		return response(200, "media"), nil
	})
	for _, u := range []string{"https://api.boosty.to/file", "https://cdn.example/file?sign=abc"} {
		resp, err := c.DownloadMedia(context.Background(), u)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if c.Auth().RefreshToken != "refresh" {
		t.Fatal("omitted refresh token erased existing token")
	}
}

func TestDialogSignsOnlyUnsignedFilesAndAudio(t *testing.T) {
	c := NewClient(AuthData{})
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return response(200, `{"signedQuery":"?sign=a%2Bb&user_id=1","messages":{"data":[{"data":[{"type":"file","url":"https://cdn.example/a"},{"type":"audio_file","url":"https://cdn.example/b"},{"type":"image","url":"https://cdn.example/c"},{"type":"file","url":"https://cdn.example/d?existing=1"}]}]}}`), nil
	})
	d, err := c.GetDialog(context.Background(), "1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://cdn.example/a?sign=a%2Bb&user_id=1", "https://cdn.example/b?sign=a%2Bb&user_id=1", "https://cdn.example/c", "https://cdn.example/d?existing=1"}
	for i, b := range d.Messages.Data[0].Data {
		if b.URL != want[i] {
			t.Errorf("block %d URL mismatch: %s", i, b.URL)
		}
	}
}

func TestAuthorization400RefreshesButOther400DoesNot(t *testing.T) {
	for _, tc := range []struct {
		name, description string
		wantRefresh       bool
	}{
		{"expired authorization", "Authorization token expired", true},
		{"invalid message", "Invalid message data", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(AuthData{AccessToken: "old", RefreshToken: "refresh"})
			refreshes := 0
			c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if r.URL.Path == "/oauth/token/" {
					refreshes++
					return response(200, `{"access_token":"new"}`), nil
				}
				if r.Header.Get("Authorization") == "Bearer new" {
					return response(200, `{}`), nil
				}
				return response(400, `{"error_description":"`+tc.description+`"}`), nil
			})
			_, err := c.GetCurrentUser(context.Background())
			if tc.wantRefresh {
				if err != nil || refreshes != 1 {
					t.Fatalf("err=%v refreshes=%d", err, refreshes)
				}
			} else if err == nil || refreshes != 0 {
				t.Fatalf("non-auth failure retried: err=%v refreshes=%d", err, refreshes)
			}
		})
	}
}

func TestInvalidGrantIsActionable(t *testing.T) {
	c := NewClient(AuthData{AccessToken: "old", RefreshToken: "bad"})
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return response(400, `{"error":"invalid_grant","error_description":"Invalid grant_type parameters"}`), nil
	})
	if err := c.RefreshToken(context.Background()); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("refresh rejection not identifiable: %v", err)
	}
}
