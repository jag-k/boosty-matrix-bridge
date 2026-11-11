package connector

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestBase64AuthCookie(t *testing.T) {
	// The Unicode suffix exercises both standard (+/) and URL-safe (-_) alphabets.
	raw := `{"accessToken":"a+b%20","refreshToken":"r/+==","extra":"` + "\uffff\uffff" + `"}`
	for name, encoding := range map[string]*base64.Encoding{
		"standard":          base64.StdEncoding,
		"standard unpadded": base64.RawStdEncoding,
		"URL-safe":          base64.URLEncoding,
		"URL-safe unpadded": base64.RawURLEncoding,
	} {
		t.Run(name, func(t *testing.T) {
			encoded := encoding.EncodeToString([]byte(raw))
			for _, input := range []string{encoded, url.PathEscape(encoded), " \n" + encoded + "\t"} {
				var got struct {
					AccessToken  string `json:"accessToken"`
					RefreshToken string `json:"refreshToken"`
				}
				if err := unmarshalAuthCookie(input, &got); err != nil {
					t.Fatal(err)
				}
				if got.AccessToken != "a+b%20" || got.RefreshToken != "r/+==" {
					t.Fatal("cookie tokens were corrupted")
				}
			}
		})
	}
}

func TestInvalidAuthCookieDoesNotExposeInput(t *testing.T) {
	for _, input := range []string{
		"private-token%invalid",
		"private-token!",
		base64.StdEncoding.EncodeToString([]byte("private-token")),
		base64.StdEncoding.EncodeToString([]byte(`{"accessToken":`)),
	} {
		var got map[string]any
		if err := unmarshalAuthCookie(input, &got); err == nil {
			t.Fatal("invalid cookie was accepted")
		} else if strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), input) {
			t.Fatal("parse error exposed credentials")
		}
	}
}
