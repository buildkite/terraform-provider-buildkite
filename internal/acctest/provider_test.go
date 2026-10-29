package acctest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

func TestPreCheckOrganizationFallback(t *testing.T) {
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	t.Setenv("BUILDKITE_ORGANIZATION", "fallback-org")
	// Register restoration before testing the unset-variable fallback.
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "")
	if err := os.Unsetenv("BUILDKITE_ORGANIZATION_SLUG"); err != nil {
		t.Fatal(err)
	}
	PreCheck(t)
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "explicit-org")
	PreCheck(t)
}

func TestProviderFactories(t *testing.T) {
	factories := ProviderFactories()
	factory, ok := factories["buildkite"]
	if !ok || len(factories) != 1 {
		t.Fatalf("provider factories = %v, want buildkite", factories)
	}
	first, err := factory()
	if err != nil {
		t.Fatal(err)
	}
	second, err := factory()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("factory reused a provider server")
	}
	response, err := first.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if response.ResourceSchemas["buildkite_agent_token"] == nil {
		t.Fatal("agent-token resource missing from provider schema")
	}
	for _, diagnostic := range response.Diagnostics {
		if diagnostic.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("provider schema diagnostic: %s: %s", diagnostic.Summary, diagnostic.Detail)
		}
	}
}

func TestGraphQLClient(t *testing.T) {
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/custom/graphql" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("authorization = %q", got)
		}
		if got := r.Header.Get("User-Agent"); got != "testing" {
			t.Errorf("user agent = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":{"viewer":{"id":"test-viewer"}}}`)
	}))
	defer server.Close()
	t.Setenv("BUILDKITE_GRAPHQL_URL", server.URL+"/custom/graphql")
	client := GraphQLClient()
	if requests.Load() != 0 {
		t.Fatal("constructing the client made an API request")
	}
	var query struct {
		Viewer struct{ ID string }
	}
	if err := client.Query(context.Background(), &query, nil); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 || query.Viewer.ID != "test-viewer" {
		t.Fatalf("requests = %d, viewer ID = %q", requests.Load(), query.Viewer.ID)
	}
}

func TestGraphQLClientEmptyEndpoint(t *testing.T) {
	t.Setenv("BUILDKITE_GRAPHQL_URL", "")
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	var query struct{ ID string }
	if err := GraphQLClient().Query(context.Background(), &query, nil); err == nil {
		t.Fatal("an explicitly empty endpoint should not fall back to production")
	}
}
