package llama

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCompleteSuccess(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"hi there"}}]}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-model", 5*time.Second)
	out, err := c.Complete(context.Background(), "be brief", "say hi")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hi there" {
		t.Errorf("Complete = %q, want %q", out, "hi there")
	}
	if gotBody["model"] != "test-model" {
		t.Errorf("request model = %v", gotBody["model"])
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	sys, _ := msgs[0].(map[string]any)
	if sys["role"] != "system" || sys["content"] != "be brief" {
		t.Errorf("unexpected system message: %v", sys)
	}
	usr, _ := msgs[1].(map[string]any)
	if usr["role"] != "user" || usr["content"] != "say hi" {
		t.Errorf("unexpected user message: %v", usr)
	}
}

func TestCompleteEmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-model", 5*time.Second)
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestCompleteHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "boom")
	}))
	defer srv.Close()

	c := New(srv.URL, "test-model", 5*time.Second)
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil {
		t.Fatal("expected error on 500, got nil")
	}
}

func TestCompleteConnectionFailure(t *testing.T) {
	// Port 1 is not listenable; Do should fail to connect.
	c := New("http://127.0.0.1:1", "test-model", 2*time.Second)
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("expected 'unreachable' message, got %v", err)
	}
}

func TestCompleteTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		io.WriteString(w, `{"choices":[{"message":{"content":"late"}}]}`)
	}))
	defer srv.Close()

	c := New(srv.URL, "test-model", 50*time.Millisecond)
	_, err := c.Complete(context.Background(), "", "hi")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected 'timeout' message, got %v", err)
	}
}
