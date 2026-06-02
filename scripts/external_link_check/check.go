// Command external_link_check inventories external HTTP(S) links in governed
// Markdown and optionally performs a live liveness check.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var externalLinkRE = regexp.MustCompile(`https?://[^\s<>"'\]\)]+`)

type config struct {
	Root    string
	Live    bool
	Timeout time.Duration
}

type linkEvidence struct {
	URL   string
	Files []string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.Root, "root", ".", "repository root")
	flag.BoolVar(&cfg.Live, "live", os.Getenv("OPENCLARION_EXTERNAL_LINKS_LIVE") == "1", "perform live HTTP liveness checks")
	flag.DurationVar(&cfg.Timeout, "timeout", 10*time.Second, "per-request timeout for live checks")
	flag.Parse()

	if err := run(cfg, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[external-links] %v\n", err)
		os.Exit(1)
	}
}

func run(cfg config, stdout io.Writer) error {
	if cfg.Timeout <= 0 {
		return errors.New("--timeout must be greater than zero")
	}
	files, err := governedMarkdownFiles(cfg.Root)
	if err != nil {
		return err
	}
	links, err := collectExternalLinks(files, cfg.Root)
	if err != nil {
		return err
	}
	if !cfg.Live {
		fmt.Fprintf(stdout, "[external-links] OK (%d unique external links inventoried across %d files; live=false)\n", len(links), len(files))
		return nil
	}
	checked, skipped, err := checkLiveLinks(context.Background(), links, cfg.Timeout)
	if err != nil {
		return err
	}
	if skipped > 0 {
		fmt.Fprintf(stdout, "[external-links] OK (%d unique external links checked across %d files; %d reserved example links skipped)\n", checked, len(files), skipped)
		return nil
	}
	fmt.Fprintf(stdout, "[external-links] OK (%d unique external links checked across %d files)\n", checked, len(files))
	return nil
}

func governedMarkdownFiles(root string) ([]string, error) {
	root = filepath.Clean(root)
	candidates := []string{
		"README.md",
		"DEVELOPMENT_WORKFLOW.md",
		"CONTRIBUTING.md",
		"GOVERNANCE.md",
		"SECURITY.md",
		"CODE_OF_CONDUCT.md",
		"DCO.md",
		"MAINTAINERS.md",
	}
	var files []string
	for _, candidate := range candidates {
		path := filepath.Join(root, filepath.FromSlash(candidate))
		if _, err := os.Stat(path); err == nil {
			files = append(files, path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	docsRoot := filepath.Join(root, "docs")
	if _, err := os.Stat(docsRoot); err == nil {
		if err := filepath.WalkDir(docsRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if entry.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(entry.Name(), ".md") {
				files = append(files, path)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func collectExternalLinks(files []string, root string) ([]linkEvidence, error) {
	byURL := map[string]map[string]struct{}{}
	for _, file := range files {
		raw, err := os.ReadFile(file) // #nosec G304 -- files come from governedMarkdownFiles under the requested root.
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		for _, match := range externalLinkRE.FindAllString(string(raw), -1) {
			url := trimURL(match)
			if url == "" {
				continue
			}
			filesForURL := byURL[url]
			if filesForURL == nil {
				filesForURL = map[string]struct{}{}
				byURL[url] = filesForURL
			}
			filesForURL[rel] = struct{}{}
		}
	}
	out := make([]linkEvidence, 0, len(byURL))
	for url, fileSet := range byURL {
		files := make([]string, 0, len(fileSet))
		for file := range fileSet {
			files = append(files, file)
		}
		sort.Strings(files)
		out = append(out, linkEvidence{URL: url, Files: files})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].URL < out[j].URL
	})
	return out, nil
}

func trimURL(url string) string {
	return strings.TrimRight(url, ".,;:")
}

func checkLiveLinks(ctx context.Context, links []linkEvidence, timeout time.Duration) (int, int, error) {
	client := &http.Client{Timeout: timeout}
	var failures []string
	checked := 0
	skipped := 0
	for _, link := range links {
		if reservedExampleURL(link.URL) {
			skipped++
			continue
		}
		checked++
		status, err := probeURL(ctx, client, link.URL)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s (%s): %v", link.URL, strings.Join(link.Files, ","), err))
			continue
		}
		if !acceptableStatus(status) {
			failures = append(failures, fmt.Sprintf("%s (%s): HTTP %d", link.URL, strings.Join(link.Files, ","), status))
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return checked, skipped, fmt.Errorf("external link liveness failures:\n%s", strings.Join(failures, "\n"))
	}
	return checked, skipped, nil
}

func reservedExampleURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	return host == "example.com" ||
		host == "example.net" ||
		host == "example.org" ||
		host == "example" ||
		strings.HasSuffix(host, ".example")
}

func probeURL(ctx context.Context, client *http.Client, url string) (int, error) {
	status, err := requestStatus(ctx, client, http.MethodHead, url)
	if err == nil && status != http.StatusMethodNotAllowed {
		return status, nil
	}
	return requestStatus(ctx, client, http.MethodGet, url)
}

func requestStatus(ctx context.Context, client *http.Client, method, url string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "openclarion-external-link-check/1")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func acceptableStatus(status int) bool {
	if status >= 200 && status < 400 {
		return true
	}
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusMethodNotAllowed, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}
