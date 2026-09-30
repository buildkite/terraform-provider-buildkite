package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// configuredTimeouts is the provider's timeouts block with one attribute set.
func configuredTimeouts(attribute, value string) timeouts.Value {
	return timeouts.Value{Object: types.ObjectValueMust(
		map[string]attr.Type{attribute: types.StringType},
		map[string]attr.Value{attribute: types.StringValue(value)},
	)}
}

// "0s" and "-1h" parse, so validation lets them through, and a retry helper or context handed one
// gives up before the request is sent.
func TestTimeoutAccessorsTreatNonPositiveAsUnset(t *testing.T) {
	t.Parallel()

	accessors := map[string]func(*Client, context.Context) (time.Duration, diag.Diagnostics){
		"create": (*Client).createTimeout,
		"read":   (*Client).readTimeout,
		"update": (*Client).updateTimeout,
		"delete": (*Client).deleteTimeout,
	}

	for _, configured := range []string{"0s", "-1h"} {
		for attribute, accessor := range accessors {
			t.Run(fmt.Sprintf("%s=%s", attribute, configured), func(t *testing.T) {
				t.Parallel()

				client := &Client{timeouts: configuredTimeouts(attribute, configured)}

				timeout, diags := accessor(client, t.Context())
				if diags.HasError() {
					t.Fatalf("%sTimeout diagnostics = %v", attribute, diags)
				}
				if timeout != DefaultTimeout {
					t.Errorf("%sTimeout with %s configured = %s, want %s", attribute, configured, timeout, DefaultTimeout)
				}
			})
		}
	}

	t.Run("positive value is used as configured", func(t *testing.T) {
		t.Parallel()

		client := &Client{timeouts: configuredTimeouts("read", "45s")}
		if timeout, _ := client.readTimeout(t.Context()); timeout != 45*time.Second {
			t.Errorf("readTimeout with 45s configured = %s, want 45s", timeout)
		}
	})
}

// makeRequest derives its deadline from the read timeout, so a zero used as-is failed every REST
// call without sending it.
func TestMakeRequestWithNonPositiveReadTimeoutReachesTheServer(t *testing.T) {
	t.Parallel()

	for _, configured := range []string{"0s", "-1h"} {
		t.Run(configured, func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t, stubResponse{status: http.StatusOK, body: `{}`})
			defer server.Close()

			client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
			client.timeouts = configuredTimeouts("read", configured)

			var response map[string]any
			err := client.makeRequest(context.Background(), http.MethodGet, "/v2/organizations/test-org/clusters", nil, &response)
			if err != nil {
				t.Fatalf("makeRequest() with read=%s error = %v", configured, err)
			}
			if got := requests.Load(); got != 1 {
				t.Errorf("Made %d requests with read=%s, want 1", got, configured)
			}
		})
	}
}

// NewClient already skipped a non-positive value for the per-attempt timeout, which left attempts
// with no limit at all. It now gets the same fallback as everything else.
func TestNewClientTreatsNonPositiveReadTimeoutAsUnset(t *testing.T) {
	t.Parallel()

	for _, configured := range []string{"0s", "-1h"} {
		t.Run(configured, func(t *testing.T) {
			t.Parallel()

			client := NewClient(&clientConfig{
				apiToken:   "test",
				graphqlURL: "https://example.com",
				restURL:    "https://example.com",
				org:        "test-org",
				userAgent:  "test",
				maxRetries: 1,
				timeouts:   configuredTimeouts("read", configured),
			})

			for name, retryClient := range map[string]*retryablehttp.Client{
				"rest":    client.restRetry,
				"graphql": client.graphqlRetry,
			} {
				if got := retryClient.HTTPClient.Timeout; got != DefaultTimeout {
					t.Errorf("%s per-attempt timeout with read=%s = %s, want %s", name, configured, got, DefaultTimeout)
				}
			}
		})
	}
}
