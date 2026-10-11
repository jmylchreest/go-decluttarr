package arrapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueuePagination(t *testing.T) {
	for _, tc := range []struct {
		name, second string
		wantErr      bool
	}{
		{"complete", `{"page":2,"totalRecords":2,"records":[{"id":2}]}`, false},
		{"empty incomplete", `{"page":2,"totalRecords":2,"records":[]}`, true},
		{"server repeats page", `{"page":1,"totalRecords":2,"records":[{"id":1}]}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Query().Get("page") == "1" {
					fmt.Fprint(w, `{"page":1,"totalRecords":2,"records":[{"id":1}]}`)
				} else {
					fmt.Fprint(w, tc.second)
				}
			}))
			defer s.Close()
			c := NewClient(ClientConfig{BaseURL: s.URL})
			defer c.Close()
			queue, err := c.GetQueue(context.Background())
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v", err)
			}
			if calls != 2 {
				t.Fatalf("calls=%d", calls)
			}
			if !tc.wantErr && len(queue) != 2 {
				t.Fatalf("queue=%v", queue)
			}
			if tc.wantErr && queue != nil {
				t.Fatal("partial queue returned as usable")
			}
		})
	}
}
