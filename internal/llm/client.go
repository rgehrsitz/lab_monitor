package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"net"
	"os"
	"time"

	openai "github.com/sashabaranov/go-openai"

	"labmonitor/internal/report"
)

type Client struct {
	model  string
	client *openai.Client
	// configuration (timeouts, retries)
	initialTimeout   time.Duration
	extensionTimeout time.Duration
	requestTimeout   time.Duration
	toneTimeout      time.Duration
	maxRetries       int
	retryBackoff     time.Duration
	eventEmitter     func(meta map[string]any)
}

type AssessmentResult struct {
	Assessment report.Assessment
	Raw        string
}

func NewClient() (*Client, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, errors.New("OPENAI_API_KEY is not set")
	}
	return &Client{client: openai.NewClient(apiKey)}, nil
}

func (c *Client) SetModel(model string) {
	c.model = model
	if c.model == "" {
		c.model = "gpt-4o"
	}
}

// Configure runtime parameters for timeouts/retries (defaults applied if zero values)
func (c *Client) Configure(initialTO, extensionTO, requestTO, toneTO time.Duration, maxRetries int, backoff time.Duration) {
	if initialTO <= 0 {
		initialTO = 45 * time.Second
	}
	if extensionTO <= 0 {
		extensionTO = 25 * time.Second
	}
	if requestTO <= 0 {
		requestTO = 20 * time.Second
	}
	if toneTO <= 0 {
		// fall back to request timeout
		toneTO = requestTO
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	if backoff <= 0 {
		backoff = 400 * time.Millisecond
	}
	c.initialTimeout = initialTO
	c.extensionTimeout = extensionTO
	c.requestTimeout = requestTO
	c.toneTimeout = toneTO
	c.maxRetries = maxRetries
	c.retryBackoff = backoff
}

// SetEventEmitter sets a callback invoked after each LLM attempt (success or terminal failure) with metadata.
func (c *Client) SetEventEmitter(fn func(meta map[string]any)) { c.eventEmitter = fn }

// classifyRetryable determines if an error is transient/ retryable.
func classifyRetryable(err error) bool {
	if err == nil {
		return false
	}
	// Unwrap to see if it's a context deadline
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		if apiErr.HTTPStatusCode == 429 || (apiErr.HTTPStatusCode >= 500 && apiErr.HTTPStatusCode <= 599) {
			return true
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return true
		}
		// treat temporary network glitches as retryable
		if netErr.Temporary() {
			return true
		}
	}
	return false
}

func (c *Client) callWithRetry(ctx context.Context, req openai.ChatCompletionRequest, timeout time.Duration, extended bool, attemptLabel string) (openai.ChatCompletionResponse, error) {
	if c.model == "" {
		c.model = "gpt-4o"
	}
	req.Model = c.model
	var lastErr error
	var empty openai.ChatCompletionResponse
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		start := time.Now()
		resp, err := c.client.CreateChatCompletion(callCtx, req)
		latency := time.Since(start)
		cancel()
		if err == nil {
			// Log structured line (stdout for now)
			usage := ""
			if (resp.Usage != openai.Usage{}) {
				usage = fmt.Sprintf(" prompt_tokens=%d completion_tokens=%d total_tokens=%d", resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.TotalTokens)
			}
			fmt.Printf("[llm] success model=%s extended=%t kind=%s attempt=%d latency_ms=%d%s\n", c.model, extended, attemptLabel, attempt+1, latency.Milliseconds(), usage)
			if c.eventEmitter != nil {
				c.eventEmitter(map[string]any{"model": c.model, "extended": extended, "kind": attemptLabel, "attempt": attempt + 1, "latency_ms": latency.Milliseconds(), "success": true, "prompt_tokens": resp.Usage.PromptTokens, "completion_tokens": resp.Usage.CompletionTokens, "total_tokens": resp.Usage.TotalTokens})
			}
			return resp, nil
		}
		lastErr = err
		retryable := classifyRetryable(err)
		fmt.Printf("[llm] error model=%s extended=%t kind=%s attempt=%d latency_ms=%d retryable=%t err=%v\n", c.model, extended, attemptLabel, attempt+1, latency.Milliseconds(), retryable, err)
		if !retryable || attempt == c.maxRetries {
			if c.eventEmitter != nil {
				c.eventEmitter(map[string]any{"model": c.model, "extended": extended, "kind": attemptLabel, "attempt": attempt + 1, "latency_ms": latency.Milliseconds(), "success": false, "error": err.Error(), "retryable": retryable})
			}
			break
		}
		// exponential backoff with jitter
		pow := math.Pow(2, float64(attempt))
		base := float64(c.retryBackoff.Milliseconds()) * pow
		jitter := rand.Float64()*0.3 + 0.85 // 0.85-1.15
		sleepMs := time.Duration(base*jitter) * time.Millisecond
		select {
		case <-time.After(sleepMs):
		case <-ctx.Done():
			return empty, ctx.Err()
		}
	}
	return empty, lastErr
}

func (c *Client) GenerateAssessment(ctx context.Context, prompt string, extended bool) (*AssessmentResult, error) {
	// choose timeout
	to := c.initialTimeout
	if extended {
		to = c.extensionTimeout
	}
	req := openai.ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: "You are an expert environmental monitoring analyst."},
			{Role: openai.ChatMessageRoleUser, Content: prompt},
		},
	}
	resp, err := c.callWithRetry(ctx, req, to, extended, "assessment")
	if err != nil {
		return nil, fmt.Errorf("openai request: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, errors.New("openai returned no choices")
	}
	content := resp.Choices[0].Message.Content
	var assessment report.Assessment
	if err := json.Unmarshal([]byte(content), &assessment); err != nil {
		return nil, fmt.Errorf("parse assessment: %w", err)
	}
	return &AssessmentResult{Assessment: assessment, Raw: content}, nil
}

// TransformAssessmentTone rewrites textual fields of an existing assessment JSON
// (summary, lab details, recommendations) to match a target style while keeping
// structure and non-textual data identical.
func (c *Client) TransformAssessmentTone(ctx context.Context, assessmentJSON string, personality string, snarkLevel int) (string, error) {
	to := c.toneTimeout
	sys := "You rewrite JSON values for summary/details/recommendations to adjust tone. Keep structure identical and return only JSON (no markdown). Do not add/remove fields. Do not change status values, lab names, or recommendations count. For humorous/snarky styles, you may add tasteful, relevant emoji beyond status icons where it enhances tone; keep it professional and not excessive."
	style := fmt.Sprintf("personality=%s snark_level=%d", personality, snarkLevel)
	user := fmt.Sprintf("Style spec: %s\nRewrite only the textual fields (summary, lab details, recommendations) to match the style while preserving meaning.\nJSON:\n%s", style, assessmentJSON)
	req := openai.ChatCompletionRequest{
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: sys},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
	}
	resp, err := c.callWithRetry(ctx, req, to, false, "tone")
	if err != nil {
		return "", fmt.Errorf("openai rewrite: %w", err)
	}
	if len(resp.Choices) == 0 {
		return "", errors.New("openai returned no choices")
	}
	return resp.Choices[0].Message.Content, nil
}
