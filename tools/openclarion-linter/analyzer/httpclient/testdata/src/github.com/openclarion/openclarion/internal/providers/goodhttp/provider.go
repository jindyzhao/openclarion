package goodhttp

import (
	"context"
	"net/http"
	"time"
)

type Provider struct {
	client *http.Client
}

func New() Provider {
	return Provider{client: &http.Client{Timeout: 5 * time.Second}}
}

func (p Provider) Do(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid", nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
