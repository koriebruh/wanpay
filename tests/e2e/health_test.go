//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

func TestHealth(t *testing.T) {
	// Health endpoint returns a non-standard JSON body (not the apiResp envelope).
	// Just verify the HTTP status code.
	r, err := http.Get(testSrv.URL + "/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	r.Body.Close()
	if r.StatusCode != http.StatusOK {
		t.Errorf("health: status=%d, want 200", r.StatusCode)
	}
}
