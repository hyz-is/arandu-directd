// Command directd-catalog reads Direct Data's public API documentation and
// writes catalog/catalog.json, the catalogue the package's products, request
// builders and response types are generated from.
//
// It reads only https://directd.com.br/central-de-ajuda/apis/catalogo and the
// page of each API under it, one page at a time with a pause between
// requests. It never contacts the API host or the portal: a request there is
// billed or authenticated, and the crawler refuses both hosts by name. A 401,
// 403 or 429 stops the crawl without retrying.
//
//	go run ./cmd/directd-catalog -out catalog/catalog.json
//	go generate ./...
//
// A page that cannot be read leaves its entry marked incomplete, with the
// reason, and the crawl goes on; nothing is invented to fill the gap.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/hyz-is/arandu-directd/internal/catalog"
)

func main() {
	out := flag.String("out", "catalog/catalog.json", "where to write the catalogue")
	delay := flag.Duration("delay", 1500*time.Millisecond, "pause between page requests")
	date := flag.String("date", time.Now().Format("2006-01-02"), "crawl date recorded in the catalogue, YYYY-MM-DD")
	only := flag.String("index-only", "", "read only the index and the API pages already saved in this directory (offline re-parse)")
	save := flag.String("save", "", "directory to keep every fetched page in, for a later offline re-parse")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, *out, *delay, *date, *only, *save); err != nil {
		fmt.Fprintln(os.Stderr, "directd-catalog:", err)
		os.Exit(1)
	}
}

// source reads a page from the network or from a saved directory.
type source struct {
	fetcher *catalog.Fetcher
	offline string
	save    string
}

func (s source) get(ctx context.Context, name, url string) ([]byte, error) {
	if s.offline != "" {
		return os.ReadFile(filepath.Join(s.offline, name+".html"))
	}
	body, err := s.fetcher.Get(ctx, url)
	if err == nil && s.save != "" {
		if werr := os.WriteFile(filepath.Join(s.save, name+".html"), body, 0o600); werr != nil {
			return nil, werr
		}
	}
	return body, err
}

func run(ctx context.Context, out string, delay time.Duration, date, offline, save string) error {
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return fmt.Errorf("-date %q is not YYYY-MM-DD", date)
	}
	if save != "" {
		if err := os.MkdirAll(save, 0o700); err != nil {
			return err
		}
	}
	src := source{fetcher: catalog.NewFetcher(delay), offline: offline, save: save}

	index, err := src.get(ctx, "_index", catalog.IndexURL)
	if err != nil {
		return fmt.Errorf("reading the index: %w", err)
	}
	items, err := catalog.IndexItems(index)
	if err != nil {
		return fmt.Errorf("parsing the index: %w", err)
	}
	fmt.Fprintf(os.Stderr, "index: %d APIs\n", len(items))

	apis := make([]catalog.API, 0, len(items))
	for i, item := range items {
		api := catalog.FromIndex(item)
		if api.Slug == "" {
			apis = append(apis, api)
			continue
		}
		page, err := src.get(ctx, api.Slug, api.DocURL)
		var status *catalog.StatusError
		switch {
		case errors.As(err, &status) && status.Stops():
			return fmt.Errorf("stopping the crawl, not retrying: %w", err)
		case ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			api.MarkIncomplete("the documentation page could not be read: " + err.Error())
		default:
			pageItem, params, perr := catalog.PageParams(page)
			if perr != nil {
				api.MarkIncomplete("the documentation page could not be parsed: " + perr.Error())
				break
			}
			api.ApplyParams(params)
			if pageItem != nil {
				if u, _ := pageItem["url"].(string); u != "" && u != api.URLTemplate {
					api.Warnings = append(api.Warnings, fmt.Sprintf("the page's endpoint template %q differs from the index's", u))
				}
			}
		}
		apis = append(apis, api)
		fmt.Fprintf(os.Stderr, "%3d/%d %s params=%d incomplete=%v\n", i+1, len(items), api.Slug, len(api.Params), api.Incomplete)
	}

	cat := catalog.Assemble(apis, date)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cat); err != nil {
		return err
	}
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s: %d APIs\n", out, cat.Count)
	return nil
}
