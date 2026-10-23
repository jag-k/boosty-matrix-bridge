package boosty

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
)

const historyPageSize = 20

// GetDialogMessages fetches a directional page. Offset is a message ID, not
// a numeric row offset. Older pages arrive newest first from the API.
func (c *Client) GetDialogMessages(ctx context.Context, dialogID, offset string, older bool, limit int, signedQuery string) (*MessageListData, error) {
	if limit <= 0 || limit > historyPageSize {
		limit = historyPageSize
	}
	query := url.Values{"limit": {strconv.Itoa(limit)}, "offset": {offset}, "reverse": {strconv.FormatBool(older)}}
	path := fmt.Sprintf("/v1/dialog/%s/message/?%s", url.PathEscape(dialogID), query.Encode())
	var result MessageListData
	if err := c.doJSON(ctx, http.MethodGet, path, nil, "", &result); err != nil {
		return nil, err
	}
	signMessageMedia(result.Data, signedQuery)
	result.Data = normalizeMessages(result.Data)
	return &result, nil
}

// GetMessageHistory gets recent history, older history, or every message after
// an anchor. Catch-up must drain all newer pages because bridgev2 does not
// follow forward pagination cursors.
func (c *Client) GetMessageHistory(ctx context.Context, dialogID, anchor string, forward bool, count int) (*DialogDetailResponse, error) {
	detail, err := c.GetDialog(ctx, dialogID)
	if err != nil {
		return nil, err
	}
	if count <= 0 {
		count = historyPageSize
	}
	if anchor == "" {
		detail.Messages.Data = normalizeMessages(detail.Messages.Data)
		for len(detail.Messages.Data) < count && !detail.Messages.Extra.IsFirst {
			if len(detail.Messages.Data) == 0 {
				return nil, fmt.Errorf("initial history page is empty but incomplete")
			}
			oldest := strconv.Itoa(detail.Messages.Data[0].ID)
			page, err := c.GetDialogMessages(ctx, dialogID, oldest, true, count-len(detail.Messages.Data), detail.SignedQuery)
			if err != nil {
				return nil, err
			}
			older, err := historyPageAfter(page, oldest, true)
			if err != nil {
				return nil, err
			}
			detail.Messages.Data = normalizeMessages(append(older, detail.Messages.Data...))
			detail.Messages.Extra.IsFirst = page.Extra.IsLast
		}
		if len(detail.Messages.Data) > count {
			detail.Messages.Data = detail.Messages.Data[len(detail.Messages.Data)-count:]
			detail.Messages.Extra.IsFirst = false
		}
		return detail, nil
	}
	// Fail on malformed persisted cursors instead of treating them as a row offset.
	if _, err := strconv.Atoi(anchor); err != nil {
		return nil, fmt.Errorf("invalid history message cursor")
	}
	detail.Messages.Data = nil
	offset := anchor
	for {
		page, err := c.GetDialogMessages(ctx, dialogID, offset, !forward, count, detail.SignedQuery)
		if err != nil {
			return nil, err
		}
		messages, err := historyPageAfter(page, offset, !forward)
		if err != nil {
			return nil, err
		}
		detail.Messages.Data = normalizeMessages(append(detail.Messages.Data, messages...))
		detail.Messages.Extra.IsLast = page.Extra.IsLast
		detail.Messages.Extra.IsFirst = !forward && page.Extra.IsLast
		if !forward || page.Extra.IsLast {
			break
		}
		offset = strconv.Itoa(messages[len(messages)-1].ID)
	}
	return detail, nil
}

func historyPageAfter(page *MessageListData, offset string, older bool) ([]Message, error) {
	id, err := strconv.Atoi(offset)
	if err != nil {
		return nil, fmt.Errorf("invalid history message cursor")
	}
	var messages []Message
	for _, m := range page.Data {
		if (older && m.ID < id) || (!older && m.ID > id) {
			messages = append(messages, m)
		}
	}
	if len(messages) == 0 && !page.Extra.IsLast {
		return nil, fmt.Errorf("history pagination made no progress")
	}
	return messages, nil
}

func normalizeMessages(messages []Message) []Message {
	slices.SortFunc(messages, func(a, b Message) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return slices.CompactFunc(messages, func(a, b Message) bool { return a.ID == b.ID })
}
