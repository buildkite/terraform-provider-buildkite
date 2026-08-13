package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

const testClusterUUID = "e8738c39-1f2a-4a1e-9b3c-2d4e5f6a7b8c"

func TestAccBuildkiteClusterNetworkRangesDatasource(t *testing.T) {
	// A cluster created by the test suite has no hosted agents, so the API returns an empty
	// collection. The attribute still has to be a known empty set for configurations that
	// iterate over it.
	t.Run("returns an empty set for a cluster without hosted agents", func(t *testing.T) {
		clusterName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster" {
							name = "%s"
						}

						data "buildkite_cluster_network_ranges" "cluster" {
							cluster_uuid = buildkite_cluster.cluster.uuid
						}`, clusterName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrPair("data.buildkite_cluster_network_ranges.cluster", "cluster_uuid", "buildkite_cluster.cluster", "uuid"),
						resource.TestCheckResourceAttr("data.buildkite_cluster_network_ranges.cluster", "ranges.#", "0"),
					),
				},
			},
		})
	})
}

// Exercises Read end to end against a stub API by pointing the provider at it with
// BUILDKITE_REST_URL, which is the only way to cover the model mapping and the known-empty-set
// guarantee without a hosted cluster.
func TestUnitBuildkiteClusterNetworkRangesDatasource(t *testing.T) {
	config := func(server *httptest.Server) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				organization = "test-org"
				api_token    = "test-token"
				rest_url     = %q
			}

			data "buildkite_cluster_network_ranges" "cluster" {
				cluster_uuid = %q
			}`, server.URL, testClusterUUID)
	}

	t.Run("maps ranges into state", func(t *testing.T) {
		server := newClusterNetworkRangesStub(t, http.StatusOK, `[
			{"cidr_range": "64.6.39.248/29", "kind": "NAMESPACE_MANAGED"},
			{"cidr_range": "10.0.0.0/16", "kind": "egress"}
		]`)
		defer server.Close()

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: config(server),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("data.buildkite_cluster_network_ranges.cluster", "ranges.#", "2"),
						resource.TestCheckTypeSetElemNestedAttrs("data.buildkite_cluster_network_ranges.cluster", "ranges.*", map[string]string{
							"cidr_range": "64.6.39.248/29",
							"kind":       "NAMESPACE_MANAGED",
						}),
						resource.TestCheckTypeSetElemNestedAttrs("data.buildkite_cluster_network_ranges.cluster", "ranges.*", map[string]string{
							"cidr_range": "10.0.0.0/16",
							"kind":       "egress",
						}),
					),
				},
			},
		})
	})

	// The empty case is the normal one for a cluster without hosted agents, and a null list would
	// break `for` expressions over the attribute.
	t.Run("empty collection becomes a known empty set", func(t *testing.T) {
		server := newClusterNetworkRangesStub(t, http.StatusOK, `[]`)
		defer server.Close()

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: config(server),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr("data.buildkite_cluster_network_ranges.cluster", "ranges.#", "0"),
					),
				},
			},
		})
	})

	t.Run("403 explains the permission requirement", func(t *testing.T) {
		server := newClusterNetworkRangesStub(t, http.StatusForbidden, `{"message": "You're not allowed to do that"}`)
		defer server.Close()

		resource.UnitTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: config(server),
					// Terraform hard-wraps diagnostics, so match a fragment that stays on one line.
					ExpectError: regexp.MustCompile(`This endpoint needs`),
				},
			},
		})
	})
}

func TestClusterNetworkRangesRejectsNonUUID(t *testing.T) {
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				// A GraphQL ID is the likely mistake, and the API answers it with an opaque 404, so
				// the schema rejects it at plan time instead.
				Config: `
					provider "buildkite" {
						organization = "test-org"
						api_token    = "test-token"
					}

					data "buildkite_cluster_network_ranges" "cluster" {
						cluster_uuid = "Q2x1c3Rlci0tLWU4NzM4YzM5LTFmMmEtNGExZS05YjNjLTJkNGU1ZjZhN2I4Yw=="
					}`,
				ExpectError: regexp.MustCompile(`must be a cluster UUID`),
			},
		},
	})
}

func TestGetClusterNetworkRanges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/v2/organizations/test-org/clusters/cluster-123/network_ranges" {
			t.Errorf("Expected path /v2/organizations/test-org/clusters/cluster-123/network_ranges, got %s", r.URL.Path)
		}

		if _, err := w.Write([]byte(`[
			{"cidr_range": "64.6.39.248/29", "kind": "NAMESPACE_MANAGED"},
			{"cidr_range": "10.0.0.0/16", "kind": "egress"}
		]`)); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	d := newTestClusterNetworkRangesDatasource(server)

	ranges, err := d.getNetworkRanges(context.Background(), "cluster-123")
	if err != nil {
		t.Fatalf("getNetworkRanges failed: %v", err)
	}

	if len(ranges) != 2 {
		t.Fatalf("Expected 2 ranges, got %d", len(ranges))
	}
	if got := derefString(ranges[0].CidrRange); got != "64.6.39.248/29" {
		t.Errorf("Expected CidrRange 64.6.39.248/29, got %s", got)
	}
	if got := derefString(ranges[0].Kind); got != "NAMESPACE_MANAGED" {
		t.Errorf("Expected Kind NAMESPACE_MANAGED, got %s", got)
	}
	if got := derefString(ranges[1].CidrRange); got != "10.0.0.0/16" {
		t.Errorf("Expected CidrRange 10.0.0.0/16, got %s", got)
	}
}

// A missing or null field from the upstream service must stay nil so it reaches state as null
// rather than as an empty string that would silently become an empty CIDR in a firewall rule.
func TestGetClusterNetworkRangesNullFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := w.Write([]byte(`[{"cidr_range": null, "kind": null}, {"kind": "egress"}]`)); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	d := newTestClusterNetworkRangesDatasource(server)

	ranges, err := d.getNetworkRanges(context.Background(), "cluster-123")
	if err != nil {
		t.Fatalf("getNetworkRanges failed: %v", err)
	}

	if len(ranges) != 2 {
		t.Fatalf("Expected 2 ranges, got %d", len(ranges))
	}
	if ranges[0].CidrRange != nil {
		t.Errorf("Expected nil CidrRange for an explicit null, got %q", *ranges[0].CidrRange)
	}
	if ranges[0].Kind != nil {
		t.Errorf("Expected nil Kind for an explicit null, got %q", *ranges[0].Kind)
	}
	if ranges[1].CidrRange != nil {
		t.Errorf("Expected nil CidrRange for an absent key, got %q", *ranges[1].CidrRange)
	}
	if got := derefString(ranges[1].Kind); got != "egress" {
		t.Errorf("Expected Kind egress, got %s", got)
	}
}

func derefString(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

// A server error propagates to the caller. Note this exercises error propagation only: the retry
// and backoff for 5xx live in the shared client built by NewClient, and go-retryablehttp discards
// the response body before giving up, so the API's own message does not survive in production.
func TestGetClusterNetworkRangesServerErrorPropagates(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := w.Write([]byte(`{"message": "Could not load network ranges: boom"}`)); err != nil {
			t.Fatalf("failed to write response: %v", err)
		}
	}))
	defer server.Close()

	d := newTestClusterNetworkRangesDatasource(server)

	_, err := d.getNetworkRanges(context.Background(), "cluster-123")
	if err == nil {
		t.Fatal("Expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "network_ranges") {
		t.Errorf("Expected error to identify the failed request, got %s", err.Error())
	}
}

func newClusterNetworkRangesStub(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()

	wantPath := fmt.Sprintf("/v2/organizations/test-org/clusters/%s/network_ranges", testClusterUUID)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Errorf("Expected path %s, got %s", wantPath, r.URL.Path)
		}
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("failed to write response: %v", err)
		}
	}))
}

func newTestClusterNetworkRangesDatasource(server *httptest.Server) *clusterNetworkRangesDatasource {
	return &clusterNetworkRangesDatasource{
		client: &Client{
			http:         server.Client(),
			restURL:      server.URL,
			organization: "test-org",
		},
	}
}
