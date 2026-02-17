package memos

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestAddMessage(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/add/message" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok"}`))
	}))
	defer ts.Close()

	c, err := GetWithConfig(map[string]interface{}{"base_url": ts.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := c.AddMessage(context.Background(), "agent1", schema.Message{Role: schema.User, Content: "hello"}); err != nil {
		t.Fatal(err)
	}
}

func TestGetMessages(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/get/messages" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		resp := map[string]interface{}{
			"data": map[string]interface{}{
				"messages": []map[string]interface{}{{"role": "user", "content": "u1"}, {"role": "assistant", "content": "a1"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	c, err := GetWithConfig(map[string]interface{}{"base_url": ts.URL})
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := c.GetMessages(context.Background(), "agent1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
}
