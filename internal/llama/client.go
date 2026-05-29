package llama

import (
	"bytes"
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

type Client struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

func New(baseURL, model string, timeout time.Duration) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		model:      model,
		httpClient: &http.Client{Timeout: timeout},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

// Complete sends a system+user prompt to the chat completions endpoint and
// returns the assistant's reply text.
func (c *Client) Complete(ctx context.Context, system, user string) (string, error) {
	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	// Single retry on transport errors (EOF, connection reset, timeout). Real
	// llama.cpp servers crash/OOM under load and may need a moment to recover;
	// without this the first failure inside a map-reduce nukes the whole call.
	// 4xx/5xx responses are NOT retried — those are model-side decisions.
	resp, err := c.doWithRetry(req, buf)
	if err != nil {
		if isTimeout(err) {
			return "", fmt.Errorf("llama timeout contacting %s; consider reducing input size: %w", c.baseURL, err)
		}
		return "", fmt.Errorf("llama.cpp unreachable at %s; check that the server is running: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("llama returned %d: %s", resp.StatusCode, string(body))
	}
	if readErr != nil {
		return "", fmt.Errorf("read llama response body: %w", readErr)
	}

	var cr chatResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("decode llama response: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", fmt.Errorf("llama returned no choices")
	}
	return cr.Choices[0].Message.Content, nil
}

// doWithRetry sends the request, and on transport failure sleeps briefly and
// retries once. The request body is rebuilt from the cached buf so the retry
// works after the first attempt drained the reader.
func (c *Client) doWithRetry(req *http.Request, buf []byte) (*http.Response, error) {
	resp, err := c.httpClient.Do(req)
	if err == nil {
		return resp, nil
	}
	// Sleep proportional to how harsh the failure was; cap small so we don't
	// blow past the caller's timeout budget.
	time.Sleep(2 * time.Second)
	req2, rerr := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), bytes.NewReader(buf))
	if rerr != nil {
		return nil, err
	}
	req2.Header = req.Header.Clone()
	return c.httpClient.Do(req2)
}

func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) {
		return ne.Timeout()
	}
	return false
}
