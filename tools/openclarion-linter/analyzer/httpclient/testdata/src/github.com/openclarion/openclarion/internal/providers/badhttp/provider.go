package badhttp

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

func directCalls(ctx context.Context) {
	_, _ = http.Get("https://example.invalid")                                           // want "production code must not use net/http.Get directly"
	_, _ = http.Head("https://example.invalid")                                          // want "production code must not use net/http.Head directly"
	_, _ = http.Post("https://example.invalid", "text/plain", strings.NewReader("body")) // want "production code must not use net/http.Post directly"
	_, _ = http.PostForm("https://example.invalid", url.Values{})                        // want "production code must not use net/http.PostForm directly"
	_ = http.DefaultClient                                                               // want "production code must not use net/http.DefaultClient directly"
	_, _ = http.NewRequestWithContext(ctx, http.MethodGet, "https://example.invalid", nil)
}
