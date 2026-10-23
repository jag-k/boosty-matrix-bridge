package boosty

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GetDialogs fetches the list of dialogs (paginated).
func (c *Client) GetDialogs(ctx context.Context, limit int) (*DialogListResponse, error) {
	path := fmt.Sprintf("/v1/dialog/?sort_by=date&sort_order=desc&limit=%d", limit)
	var resp DialogListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, "", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// GetDialog fetches a single dialog with messages.
func (c *Client) GetDialog(ctx context.Context, dialogID string) (*DialogDetailResponse, error) {
	path := fmt.Sprintf("/v1/dialog/%s", url.PathEscape(dialogID))
	var resp DialogDetailResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, "", &resp); err != nil {
		return nil, err
	}

	signMessageMedia(resp.Messages.Data, resp.SignedQuery)
	return &resp, nil
}

func signMessageMedia(messages []Message, signedQuery string) {
	// File and audio URLs need the dialog-level signature; images are already signed.
	for i := range messages {
		for j := range messages[i].Data {
			block := &messages[i].Data[j]
			if block.Type != ContentTypeFile && block.Type != ContentTypeAudio {
				continue
			}
			u, err := url.Parse(block.URL)
			if err == nil && u.Host != "" && u.RawQuery == "" && signedQuery != "" {
				u.RawQuery = strings.TrimPrefix(signedQuery, "?")
				block.URL = u.String()
			}
		}
	}
}

// SendMessage sends a text message to a dialog.
// The data format is: [{"type":"text","content":"[\"text\",\"unstyled\",[]]","modificator":""},{"type":"text","content":"","modificator":"BLOCK_END"}]
func (c *Client) SendMessage(ctx context.Context, dialogID string, text string) (*Message, error) {
	contentInner, _ := json.Marshal([]any{text, "unstyled", []any{}})

	type block struct {
		Type        string `json:"type"`
		Content     string `json:"content"`
		Modificator string `json:"modificator"`
	}
	dataBytes, _ := json.Marshal([]block{
		{Type: "text", Content: string(contentInner), Modificator: ""},
		{Type: "text", Content: "", Modificator: "BLOCK_END"},
	})

	formData := url.Values{
		"data":        {string(dataBytes)},
		"teaser_data": {"[]"},
	}

	path := fmt.Sprintf("/v1/dialog/%s/message", url.PathEscape(dialogID))
	var msg Message
	if err := c.doFormPost(ctx, path, formData, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// MarkMessageRead marks a message as read.
func (c *Client) MarkMessageRead(ctx context.Context, messageID string) error {
	path := fmt.Sprintf("/v1/dialog/message/%s/read", url.PathEscape(messageID))
	return c.doFormPost(ctx, path, url.Values{}, nil)
}
