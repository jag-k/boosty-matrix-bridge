package connector

import (
	"context"
	"net/url"
	"testing"

	"github.com/jag-k/boosty-matrix-bridge/pkg/boosty"
	"maunium.net/go/mautrix/bridgev2/networkid"
	"maunium.net/go/mautrix/event"
)

func TestMultipartIDsAndFailedAttachments(t *testing.T) {
	b := &BoostyClient{}
	cm, err := b.convertToMatrix(context.Background(), nil, nil, boosty.Message{Data: []boosty.ContentBlock{
		{Type: boosty.ContentTypeText, Content: `["hello","unstyled",[]]`},
		{Type: boosty.ContentTypeVideo, URL: "https://example.com/1"},
		{Type: boosty.ContentTypeFile, Title: "missing.pdf"},
		{Type: boosty.ContentTypeVideo, URL: "https://example.com/2"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(cm.Parts) != 4 {
		t.Fatalf("got %d parts", len(cm.Parts))
	}
	seen := map[networkid.PartID]bool{}
	for _, p := range cm.Parts {
		if seen[p.ID] {
			t.Fatalf("duplicate part ID %q", p.ID)
		}
		seen[p.ID] = true
	}
	if cm.Parts[2].Content.MsgType != event.MsgNotice {
		t.Fatal("attachment failure was hidden")
	}
}

func TestCookieEncoding(t *testing.T) {
	raw := `{"accessToken":"a+b%20","refreshToken":"refresh"}`
	for _, input := range []string{raw, url.PathEscape(raw), "{%22accessToken%22:%22a+b%2520%22,%22refreshToken%22:%22refresh%22}"} {
		var got struct {
			AccessToken string `json:"accessToken"`
		}
		if err := unmarshalAuthCookie(input, &got); err != nil {
			t.Fatal(err)
		}
		if got.AccessToken != "a+b%20" {
			t.Fatalf("token corrupted: %q", got.AccessToken)
		}
	}
}
