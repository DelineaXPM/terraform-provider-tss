package delinea

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHeaderTransport(t *testing.T) {
	headers := map[string]string{
		"X-Test-Header":  "test-value",
		"CF-Access-Id":   "client-id",
		"CF-Access-Auth": "client-secret",
	}

	// Mock server to check received headers
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range headers {
			if r.Header.Get(k) != v {
				t.Errorf("Expected header %s to be %s, got %s", k, v, r.Header.Get(k))
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create the transport
	transport := &headerTransport{
		inner:   http.DefaultTransport,
		headers: headers,
	}

	client := &http.Client{
		Transport: transport,
	}

	// Execute request
	req, _ := http.NewRequest("GET", server.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.Status)
	}
}
