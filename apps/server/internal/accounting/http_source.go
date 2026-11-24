package accounting

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// HTTPSource polls an accounting API. It points at the bundled mock by
// default, but speaks the contract a real provider would, so swapping in a
// QuickBooks client means replacing this file and nothing else.
type HTTPSource struct {
	name   string
	url    string
	client *http.Client
}

func NewHTTPSource(name, endpoint string, timeout time.Duration) *HTTPSource {
	return &HTTPSource{
		name:   name,
		url:    endpoint,
		client: &http.Client{Timeout: timeout},
	}
}

func (s *HTTPSource) Name() string { return s.name }

func (s *HTTPSource) FetchChanged(ctx context.Context, since time.Time, cursor string, limit int) (Page, error) {
	endpoint, err := url.Parse(s.url)
	if err != nil {
		return Page{}, fmt.Errorf("accounting: bad source url %q: %w", s.url, err)
	}

	q := endpoint.Query()
	q.Set("limit", strconv.Itoa(limit))
	if !since.IsZero() {
		q.Set("updated_since", since.UTC().Format(time.RFC3339))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Page{}, fmt.Errorf("accounting: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return Page{}, fmt.Errorf("accounting: request %s: %w", endpoint.Redacted(), err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection is reusable
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Page{}, fmt.Errorf("accounting: source returned %s: %s", resp.Status, body)
	}

	var wire WirePage
	if err := json.UnmarshalRead(resp.Body, &wire); err != nil {
		return Page{}, fmt.Errorf("accounting: decode response: %w", err)
	}

	records, err := RecordsFromWire(wire.Transactions)
	if err != nil {
		return Page{}, err
	}
	SortRecords(records)

	return Page{
		Records:    records,
		NextCursor: wire.NextCursor,
		HasMore:    wire.HasMore,
	}, nil
}
