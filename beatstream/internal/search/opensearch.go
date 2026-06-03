package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const indexName = "tracks"

type Client struct {
	endpoint   string
	httpClient *http.Client
}

func New(endpoint string) *Client {
	return &Client{
		endpoint:   strings.TrimRight(endpoint, "/"),
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) EnsureIndex(ctx context.Context) error {
	mapping := map[string]any{
		"settings": map[string]any{
			"number_of_shards":   1,
			"number_of_replicas": 0,
		},
		"mappings": map[string]any{
			"properties": map[string]any{
				"id":          map[string]string{"type": "keyword"},
				"title":       map[string]string{"type": "text"},
				"artist_name": map[string]string{"type": "text"},
				"play_count":  map[string]string{"type": "long"},
				"status":      map[string]string{"type": "keyword"},
				"suggest": map[string]any{
					"type": "completion",
				},
			},
		},
	}

	body, _ := json.Marshal(mapping)
	req, _ := http.NewRequestWithContext(ctx, "PUT", c.endpoint+"/"+indexName, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("create index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusBadRequest {
		// Index already exists — not an error.
		b, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(b), "resource_already_exists_exception") {
			return nil
		}
		return fmt.Errorf("create index: %s", string(b))
	}
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create index status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (c *Client) Index(ctx context.Context, doc TrackDocument) error {
	payload := map[string]any{
		"id":          doc.ID,
		"title":       doc.Title,
		"artist_name": doc.ArtistName,
		"play_count":  doc.PlayCount,
		"status":      doc.Status,
		"suggest": map[string]any{
			"input": []string{doc.Title, doc.ArtistName},
		},
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/%s/_doc/%s", c.endpoint, indexName, doc.ID)
	req, _ := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("index doc: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("index doc status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]TrackResult, error) {
	q := map[string]any{
		"size": limit,
		"query": map[string]any{
			"bool": map[string]any{
				"must": []any{
					map[string]any{
						"multi_match": map[string]any{
							"query":     query,
							"fields":    []string{"title^3", "artist_name^2"},
							"fuzziness": "AUTO",
						},
					},
				},
				"filter": []any{
					map[string]any{
						"term": map[string]string{"status": "ready"},
					},
				},
			},
		},
		"sort": []any{
			"_score",
			map[string]any{"play_count": "desc"},
		},
	}

	body, _ := json.Marshal(q)
	url := fmt.Sprintf("%s/%s/_search", c.endpoint, indexName)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search status %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Hits struct {
			Hits []struct {
				Source TrackResult `json:"_source"`
				Score  float64    `json:"_score"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}

	results := make([]TrackResult, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		r := hit.Source
		r.Score = hit.Score
		results = append(results, r)
	}
	return results, nil
}

func (c *Client) Autocomplete(ctx context.Context, prefix string, limit int) ([]string, error) {
	q := map[string]any{
		"suggest": map[string]any{
			"track-suggest": map[string]any{
				"prefix": prefix,
				"completion": map[string]any{
					"field":           "suggest",
					"size":            limit,
					"skip_duplicates": true,
				},
			},
		},
	}

	body, _ := json.Marshal(q)
	url := fmt.Sprintf("%s/%s/_search", c.endpoint, indexName)
	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("autocomplete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("autocomplete status %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Suggest map[string][]struct {
			Options []struct {
				Text string `json:"text"`
			} `json:"options"`
		} `json:"suggest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode autocomplete: %w", err)
	}

	var suggestions []string
	if entries, ok := result.Suggest["track-suggest"]; ok && len(entries) > 0 {
		for _, opt := range entries[0].Options {
			suggestions = append(suggestions, opt.Text)
		}
	}
	return suggestions, nil
}
