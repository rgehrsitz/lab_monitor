package thingspeak

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"
)

const baseURL = "https://api.thingspeak.com/channels"

const (
	maxResultsPerRequest = 8000
	minChunkDuration     = time.Minute
)

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

type FetchStats struct {
	Requests int
	Splits   int
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

func (c *Client) GetFeeds(ctx context.Context, channelID int, start, end time.Time) ([]Feed, FetchStats, error) {
	if end.Before(start) {
		return nil, FetchStats{}, fmt.Errorf("end before start")
	}
	feeds, stats, err := c.fetchRange(ctx, channelID, start.UTC(), end.UTC())
	if err != nil {
		return nil, FetchStats{}, err
	}
	sort.Slice(feeds, func(i, j int) bool {
		return feeds[i].CreatedAt.Before(feeds[j].CreatedAt)
	})
	return feeds, stats, nil
}

func (c *Client) fetchRange(ctx context.Context, channelID int, start, end time.Time) ([]Feed, FetchStats, error) {
	if !end.After(start) {
		return nil, FetchStats{}, nil
	}
	duration := end.Sub(start)
	feeds, err := c.fetchSingle(ctx, channelID, start, end)
	if err != nil {
		return nil, FetchStats{}, err
	}
	stats := FetchStats{Requests: 1}
	if len(feeds) < maxResultsPerRequest || duration <= minChunkDuration {
		return feeds, stats, nil
	}
	midpoint := start.Add(duration / 2)
	first, firstStats, err := c.fetchRange(ctx, channelID, start, midpoint)
	if err != nil {
		return nil, FetchStats{}, err
	}
	second, secondStats, err := c.fetchRange(ctx, channelID, midpoint, end)
	if err != nil {
		return nil, FetchStats{}, err
	}
	stats.Requests += firstStats.Requests + secondStats.Requests
	stats.Splits = 1 + firstStats.Splits + secondStats.Splits
	return mergeFeeds(first, second), stats, nil
}

func (c *Client) fetchSingle(ctx context.Context, channelID int, start, end time.Time) ([]Feed, error) {
	u, err := url.Parse(fmt.Sprintf("%s/%d/feeds.json", baseURL, channelID))
	if err != nil {
		return nil, fmt.Errorf("build url: %w", err)
	}

	q := u.Query()
	q.Set("timezone", "UTC")
	q.Set("start", start.Format(time.RFC3339))
	q.Set("end", end.Format(time.RFC3339))
	q.Set("results", strconv.Itoa(maxResultsPerRequest))
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

func mergeFeeds(a, b []Feed) []Feed {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	combined := make([]Feed, 0, len(a)+len(b))
	seen := make(map[int]struct{}, len(a)+len(b))
	add := func(feeds []Feed) {
		for _, feed := range feeds {
			if _, ok := seen[feed.EntryID]; ok {
				continue
			}
			seen[feed.EntryID] = struct{}{}
			combined = append(combined, feed)
		}
	}
	add(a)
	add(b)
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].CreatedAt.Before(combined[j].CreatedAt)
	})
	return combined
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}
