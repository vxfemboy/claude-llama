package llama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthCheckOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"qwen-test"},{"id":"llama-test"}]}`))
	}))
	defer srv.Close()

	res, err := NewHealthClient(srv.URL, time.Second).Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || len(res.Models) != 2 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !res.HasModel("qwen-test") {
		t.Errorf("HasModel(qwen-test) = false")
	}
}

func TestHealthCheckUnreachable(t *testing.T) {
	res, err := NewHealthClient("http://127.0.0.1:1", 200*time.Millisecond).Check(context.Background())
	if err == nil {
		t.Fatal("expected error contacting closed port")
	}
	if res.OK {
		t.Errorf("OK should be false on unreachable server")
	}
	if res.ErrorKind != ErrKindUnreach && res.ErrorKind != ErrKindTimeout {
		t.Errorf("ErrorKind = %q, want unreachable or timeout", res.ErrorKind)
	}
}

func TestFetchPropsBothShapes(t *testing.T) {
	cases := map[string]struct {
		body string
		want int
	}{
		"top-level":      {`{"n_ctx":4096}`, 4096},
		"nested (older)": {`{"default_generation_settings":{"n_ctx":2048}}`, 2048},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			props, err := NewHealthClient(srv.URL, time.Second).FetchProps(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if props.NCtx != c.want {
				t.Errorf("NCtx = %d, want %d", props.NCtx, c.want)
			}
		})
	}
}

func TestChunkBudgetFromCtx(t *testing.T) {
	cases := map[int]int{
		0:     0,
		1024:  614, // 60%
		4096:  2457,
		63232: 37939,
		100:   512, // floor
	}
	for in, want := range cases {
		if got := ChunkBudgetFromCtx(in); got != want {
			t.Errorf("ChunkBudgetFromCtx(%d) = %d, want %d", in, got, want)
		}
	}
}
