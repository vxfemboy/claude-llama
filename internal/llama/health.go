package llama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// HealthResult is what both the doctor CLI and the llama_health MCP tool consume.
type HealthResult struct {
	OK        bool          `json:"ok"`
	URL       string        `json:"url"`
	Models    []string      `json:"models,omitempty"`
	LatencyMs int64         `json:"latency_ms"`
	Error     string        `json:"error,omitempty"`
	ErrorKind ErrorKind     `json:"error_kind,omitempty"`
	Duration  time.Duration `json:"-"`
}

// ErrorKind classifies a failed health check so callers can render an actionable hint.
type ErrorKind string

const (
	ErrKindNone       ErrorKind = ""
	ErrKindTimeout    ErrorKind = "timeout"
	ErrKindUnreach    ErrorKind = "unreachable"
	ErrKindHTTPStatus ErrorKind = "http_status"
	ErrKindDecode     ErrorKind = "decode"
)

// HasModel reports whether the configured model appears in the listing.
// The match is exact; llama.cpp normalizes display names so we don't try to be clever.
func (r HealthResult) HasModel(name string) bool {
	for _, m := range r.Models {
		if m == name {
			return true
		}
	}
	return false
}

// HealthClient is a thin /v1/models client. It deliberately does NOT share
// the chat-completions Client: health checks need their own (short) timeout
// and would otherwise inherit a 2-minute completion timeout.
type HealthClient struct {
	baseURL string
	http    *http.Client
}

func NewHealthClient(baseURL string, timeout time.Duration) *HealthClient {
	return &HealthClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: timeout},
	}
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// Props mirrors the subset of llama.cpp's /props we care about for sizing chunks.
type Props struct {
	NCtx int `json:"n_ctx"`
}

type propsResponse struct {
	NCtx                      int `json:"n_ctx"`
	DefaultGenerationSettings struct {
		NCtx int `json:"n_ctx"`
	} `json:"default_generation_settings"`
}

// FetchProps queries /props and returns the effective n_ctx. llama.cpp puts
// n_ctx at the top level on newer builds and under default_generation_settings
// on older ones; we read whichever is populated. Returns 0 + error on failure.
func (c *HealthClient) FetchProps(ctx context.Context) (Props, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/props", nil)
	if err != nil {
		return Props{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Props{}, fmt.Errorf("fetch /props: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Props{}, fmt.Errorf("/props returned HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var pr propsResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return Props{}, fmt.Errorf("decode /props: %w", err)
	}
	n := pr.NCtx
	if n == 0 {
		n = pr.DefaultGenerationSettings.NCtx
	}
	return Props{NCtx: n}, nil
}

// ChunkBudgetFromCtx converts a llama context size into a safe chunk size for
// our chars/4 token estimator. We reserve 40% of the context for the system
// prompt + reply budget and convert the remaining "real tokens" into our
// estimator's units (chunker counts chars/4, so estimated == real for English-ish text).
func ChunkBudgetFromCtx(nCtx int) int {
	if nCtx <= 0 {
		return 0
	}
	budget := nCtx * 60 / 100
	if budget < 512 {
		budget = 512
	}
	return budget
}

// Check hits /v1/models and returns a populated HealthResult. The returned
// error mirrors result.Error/ErrorKind for callers that prefer Go-style handling.
func (c *HealthClient) Check(ctx context.Context) (HealthResult, error) {
	start := time.Now()
	res := HealthResult{URL: c.baseURL}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		res.Error = err.Error()
		res.ErrorKind = ErrKindUnreach
		return res, err
	}
	resp, err := c.http.Do(req)
	res.LatencyMs = time.Since(start).Milliseconds()
	res.Duration = time.Since(start)
	if err != nil {
		res.Error = err.Error()
		res.ErrorKind = classifyTransport(err)
		return res, fmt.Errorf("llama unreachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		res.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 200))
		res.ErrorKind = ErrKindHTTPStatus
		return res, errors.New(res.Error)
	}

	var mr modelsResponse
	if err := json.Unmarshal(body, &mr); err != nil {
		res.Error = err.Error()
		res.ErrorKind = ErrKindDecode
		return res, fmt.Errorf("decode /v1/models: %w", err)
	}
	res.OK = true
	res.Models = make([]string, 0, len(mr.Data))
	for _, m := range mr.Data {
		res.Models = append(res.Models, m.ID)
	}
	return res, nil
}

func classifyTransport(err error) ErrorKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrKindTimeout
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return ErrKindTimeout
	}
	return ErrKindUnreach
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
