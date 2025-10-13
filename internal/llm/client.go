package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	openai "github.com/sashabaranov/go-openai"

	"labmonitor/internal/report"
)

type Client struct {
	model  string
	client *openai.Client
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
	return &Client{
		client: openai.NewClient(apiKey),
	}, nil
}

func (c *Client) SetModel(model string) {
	c.model = model
	if c.model == "" {
		c.model = "gpt-4o"
	}
}

func (c *Client) GenerateAssessment(ctx context.Context, prompt string) (*AssessmentResult, error) {
	if c.model == "" {
		c.model = "gpt-4o"
	}
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You are an expert environmental monitoring analyst.",
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
	})
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
