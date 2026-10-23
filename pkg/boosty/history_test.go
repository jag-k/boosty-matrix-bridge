package boosty

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
)

// The live API returns limit+1 rows and older pages in descending order.
// Fixtures also overlap boundaries so catch-up cannot duplicate events.
func TestHistoryPaging(t *testing.T) {
	for _, tc := range []struct {
		name, anchor string
		forward      bool
		count        int
		want         []int
		first        bool
	}{
		{"initial collects older pages", "", true, 5, []int{2, 3, 4, 5, 6}, false},
		{"initial reaches beginning", "", true, 50, []int{1, 2, 3, 4, 5, 6}, true},
		{"catchup drains all forward pages", "1", true, 2, []int{2, 3, 4, 5, 6}, false},
		{"backward page", "5", false, 2, []int{3, 4}, false},
		{"backward end", "3", false, 2, []int{1, 2}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(AuthData{})
			c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				var data any
				if r.URL.Path == "/v1/dialog/10" {
					data = DialogDetailResponse{SignedQuery: "?sign=abc", Messages: MessageListData{Data: []Message{{ID: 5}, {ID: 6}}, Extra: MessageListExtra{IsLast: true}}}
				} else {
					if r.URL.Path != "/v1/dialog/10/message/" {
						t.Fatalf("wrong endpoint: %s", r.URL.Path)
					}
					q := r.URL.Query()
					offset, _ := strconv.Atoi(q.Get("offset"))
					older := q.Get("reverse") == "true"
					if q.Get("limit") == "" {
						t.Fatal("missing limit")
					}
					var ids []int
					if older {
						for id := offset - 1; id >= 1 && len(ids) < 2; id-- {
							ids = append(ids, id)
						}
					} else {
						for id := offset; id <= 6 && len(ids) < 3; id++ {
							ids = append(ids, id)
						}
					}
					page := MessageListData{}
					for _, id := range ids {
						page.Data = append(page.Data, Message{ID: id, Data: []ContentBlock{{Type: ContentTypeFile, URL: "https://cdn.example/file"}}})
					}
					page.Extra.IsLast = len(ids) == 0 || (older && ids[len(ids)-1] == 1) || (!older && ids[len(ids)-1] == 6)
					data = page
				}
				raw, _ := json.Marshal(data)
				return response(200, string(raw)), nil
			})
			got, err := c.GetMessageHistory(context.Background(), "10", tc.anchor, tc.forward, tc.count)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Messages.Data) != len(tc.want) {
				t.Fatalf("got %v want %v", got.Messages.Data, tc.want)
			}
			for i, m := range got.Messages.Data {
				if m.ID != tc.want[i] {
					t.Fatalf("message %d got %d want %d", i, m.ID, tc.want[i])
				}
				for _, b := range m.Data {
					if b.URL != "https://cdn.example/file?sign=abc" {
						t.Fatal("page attachment lost dialog signature")
					}
				}
			}
			if got.Messages.Extra.IsFirst != tc.first {
				t.Fatalf("wrong history beginning flag: %v", got.Messages.Extra.IsFirst)
			}
		})
	}
}

func TestHistoryRejectsStuckCursor(t *testing.T) {
	c := NewClient(AuthData{})
	c.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/v1/dialog/10" {
			return response(200, `{"messages":{"data":[]}}`), nil
		}
		return response(200, `{"data":[{"id":5}],"extra":{"isLast":false}}`), nil
	})
	if _, err := c.GetMessageHistory(context.Background(), "10", "5", true, 20); err == nil {
		t.Fatal("stuck API cursor should fail, not spin or mark history complete")
	}
}
