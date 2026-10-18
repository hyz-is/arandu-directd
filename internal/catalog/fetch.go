package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// DocumentationHosts are the only hosts the crawler reads. The API host and
// the portal are refused by name as well, because a request to either one is
// either billed or authenticated.
var DocumentationHosts = map[string]bool{
	"directd.com.br":     true,
	"www.directd.com.br": true,
}

// RefusedHosts are hosts the crawler never contacts.
var RefusedHosts = map[string]bool{
	"apiv3.directd.com.br": true,
	"app.directd.com.br":   true,
}

// ErrRefusedHost is a URL outside the public documentation.
var ErrRefusedHost = errors.New("catalog: host is not the public documentation")

// maxPage bounds a documentation page's size.
const maxPage = 16 << 20

// Fetcher reads public documentation pages, one at a time, with a pause
// between requests.
type Fetcher struct {
	// Client is the HTTP client. It must not follow redirects off the
	// documentation hosts; New sets that up.
	Client *http.Client
	// Delay is the pause before every request after the first.
	Delay time.Duration
	// UserAgent identifies the crawler.
	UserAgent string

	last time.Time
}

// NewFetcher returns a Fetcher that refuses redirects off the documentation
// hosts.
func NewFetcher(delay time.Duration) *Fetcher {
	return &Fetcher{
		Client: &http.Client{
			Timeout: 60 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if err := CheckURL(req.URL); err != nil {
					return err
				}
				if len(via) > 5 {
					return errors.New("catalog: too many redirects")
				}
				return nil
			},
		},
		Delay:     delay,
		UserAgent: "arandu-directd-catalog (+https://github.com/hyz-is/arandu-directd)",
	}
}

// CheckURL refuses any URL that is not https on a documentation host.
func CheckURL(u *url.URL) error {
	host := u.Hostname()
	if RefusedHosts[host] || !DocumentationHosts[host] || u.Scheme != "https" {
		return fmt.Errorf("%w: %s://%s%s", ErrRefusedHost, u.Scheme, u.Host, u.Path)
	}
	return nil
}

// Get reads one page.
func (f *Fetcher) Get(ctx context.Context, raw string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if err := CheckURL(u); err != nil {
		return nil, err
	}
	if !f.last.IsZero() {
		if wait := f.Delay - time.Since(f.last); wait > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
	}
	f.last = time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "text/html")
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &StatusError{Path: u.Path, Code: resp.StatusCode}
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxPage))
}

// StatusError is a page that did not answer 200.
type StatusError struct {
	// Path is the page path.
	Path string
	// Code is the HTTP status.
	Code int
}

// Error describes the status.
func (e *StatusError) Error() string { return fmt.Sprintf("catalog: %s answered %d", e.Path, e.Code) }

// Stops reports whether the crawl must stop rather than go on: an
// authorization or rate refusal is never retried or worked around.
func (e *StatusError) Stops() bool {
	return e.Code == http.StatusUnauthorized || e.Code == http.StatusForbidden || e.Code == http.StatusTooManyRequests
}
