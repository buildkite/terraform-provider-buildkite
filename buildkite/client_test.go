package buildkite

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shurcooL/graphql"
)

// A failed organization lookup must not be cached. Now that transient errors can trigger a
// retry, caching an empty ID on failure would make the next retry read it back as a successful
// empty org ID and issue mutations against "".
func TestClientGetOrganizationIDNotCachedOnError(t *testing.T) {
	t.Parallel()

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			// Transient throttle delivered in a GraphQL 200 body.
			_, _ = w.Write([]byte(`{"errors":[{"message":"Cluster creation is currently busy, please try again."}]}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"organization": map[string]interface{}{"id": "org-abc"},
			},
		})
	}))
	defer server.Close()

	client := &Client{
		graphql:      graphql.NewClient(server.URL, server.Client()),
		organization: "test-org",
	}

	if _, err := client.GetOrganizationID(); err == nil {
		t.Fatal("expected error from first lookup, got nil")
	}
	if client.organizationId != nil {
		t.Fatalf("organizationId was cached after a failed lookup: %q", *client.organizationId)
	}

	id, err := client.GetOrganizationID()
	if err != nil {
		t.Fatalf("second lookup failed: %v", err)
	}
	if id == nil || *id != "org-abc" {
		got := "nil"
		if id != nil {
			got = *id
		}
		t.Fatalf("GetOrganizationID() = %q, want %q", got, "org-abc")
	}
}
