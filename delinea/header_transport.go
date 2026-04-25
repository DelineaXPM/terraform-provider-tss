package delinea

import "net/http"

// headerTransport wraps an http.RoundTripper to inject custom headers
type headerTransport struct {
	inner   http.RoundTripper
	headers map[string]string
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.inner.RoundTrip(req)
}

// configureExtraHeaders wraps http.DefaultTransport to inject the given headers
func configureExtraHeaders(headers map[string]string) {
	if len(headers) == 0 {
		return
	}

	// Avoid double-wrapping if Configure is called multiple times
	current := http.DefaultTransport
	if ht, ok := current.(*headerTransport); ok {
		current = ht.inner
	}

	http.DefaultTransport = &headerTransport{
		inner:   current,
		headers: headers,
	}
}
