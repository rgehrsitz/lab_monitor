package thingspeak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const baseURL = "https://api.thingspeak.com/channels"

type Client struct {
	HTTPClient *http.Client
}

func NewClient() *Client {
	return &Client{HTTPClient: &http.Client{Timeout: 15 * time.Second}}
}

type Feed struct {
	CreatedAt time.Time
	EntryID   int
	Fields    map[string]float64
}

type feedResponse struct {
	Feeds []feedItem `json:"feeds"`
}

type feedItem struct {
	CreatedAt time.Time `json:"created_at"`
	EntryID   int       `json:"entry_id"`
	Field1    *string   `json:"field1"`
	Field2    *string   `json:"field2"`
	Field3    *string   `json:"field3"`
	Field4    *string   `json:"field4"`
	Field5    *string   `json:"field5"`
	Field6    *string   `json:"field6"`
	Field7    *string   `json:"field7"`
	Field8    *string   `json:"field8"`
}

func (c *Client) GetFeeds(ctx context.Context, channelID int, start, end time.Time) ([]Feed, error) {
	if end.Before(start) {
		return nil, fmt.Errorf("end before start")
	}

	u, err := url.Parse(fmt.Sprintf("%s/%d/feeds.json", baseURL, channelID))
	if err != nil {
		return nil, fmt.Errorf("build url: %w", err)
	}

	q := u.Query()
	q.Set("timezone", "UTC")
	q.Set("start", start.UTC().Format(time.RFC3339))
	q.Set("end", end.UTC().Format(time.RFC3339))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.http().Do(req)
	if err != nil {
		return nil, fmt.Errorf("perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var fr feedResponse
	if err := json.NewDecoder(resp.Body).Decode(&fr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	feeds := make([]Feed, 0, len(fr.Feeds))
	for _, item := range fr.Feeds {
		fields := make(map[string]float64, 8)
		addField := func(name string, value *string) {
			if value == nil {
				return
			}
			if f, err := strconv.ParseFloat(*value, 64); err == nil {
				fields[name] = f
			}
		}
		addField("field1", item.Field1)
		addField("field2", item.Field2)
		addField("field3", item.Field3)
		addField("field4", item.Field4)
		addField("field5", item.Field5)
		addField("field6", item.Field6)
		addField("field7", item.Field7)
		addField("field8", item.Field8)

		feeds = append(feeds, Feed{
			CreatedAt: item.CreatedAt,
			EntryID:   item.EntryID,
			Fields:    fields,
		})
	}

	return feeds, nil
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}
