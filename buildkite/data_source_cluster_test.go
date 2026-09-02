package buildkite

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteClusterDatasource(t *testing.T) {
	t.Run("timeout reading cluster", func(t *testing.T) {
		t.Skip()
		clusterName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						provider "buildkite" {
							timeouts = {
								read = "0s"
							}
						}
						resource "buildkite_cluster" "cluster" {
							name = "%s"
						}
						data "buildkite_cluster" "default" {
							name = buildkite_cluster.cluster.name
						}`, clusterName),
					ExpectError: regexp.MustCompile(`timeout while waiting for state to become 'success'`),
				},
			},
		})
	})

	t.Run("can find a cluster", func(t *testing.T) {
		clusterName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster" {
							name = "%s"
							color = "#f1efff"
						}
						data "buildkite_cluster" "cluster" {
								name = buildkite_cluster.cluster.name
						}`, clusterName),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttrPair("data.buildkite_cluster.cluster", "id", "buildkite_cluster.cluster", "id"),
						resource.TestCheckResourceAttr("data.buildkite_cluster.cluster", "color", "#f1efff"),
						resource.TestCheckResourceAttrSet("data.buildkite_cluster.cluster", "maintainers.#"),
					),
				},
			},
		})
	})

	t.Run("errors if cannot find cluster", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: `data "buildkite_cluster" "default" {
								name = "doesn't exist"
							}`,
					ExpectError: regexp.MustCompile("Unable to find Cluster"),
				},
			},
		})
	})
}

// The single-cluster data source has the same fabricated empty list as buildkite_clusters, on the
// same call. A refusal reports no maintainers; anything else leaves the list unknown, and an empty
// one would be consumed as though the API had said so.
func TestClusterDatasourceReadClassifiesAFailedMaintainerRead(t *testing.T) {
	t.Parallel()

	clusterFound := stubResponse{status: http.StatusOK, body: `{"data":{"organization":{"clusters":{
		"pageInfo": {"endCursor": "", "hasNextPage": false},
		"edges": [{"node": {
			"id": "cluster-id", "uuid": "cluster-uuid", "name": "a-cluster",
			"description": null, "emoji": null, "color": null
		}}]
	}}}}`}

	tests := []struct {
		name        string
		maintainers stubResponse
		// Empty means the read has to succeed, with the maintainers named here.
		wantError       string
		wantMaintainers []string
	}{
		{
			name:            "the maintainers are readable",
			maintainers:     stubResponse{status: http.StatusOK, body: `[{"id":"permission-uuid","actor":{"id":"actor-uuid","type":"team","name":"a-team"}}]`},
			wantMaintainers: []string{"permission-uuid"},
		},
		{
			name:        "the maintainers call is refused",
			maintainers: stubResponse{status: http.StatusForbidden, body: `{"message":"Forbidden"}`},
		},
		{
			name:        "the maintainers call fails for any other reason",
			maintainers: stubResponse{status: http.StatusInternalServerError, body: `{"message":"boom"}`},
			wantError:   "Unable to fetch cluster maintainers",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, _ := newRetryStub(t, clusterFound, testCase.maintainers)
			defer server.Close()

			c := &clusterDatasource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}

			ctx := t.Context()
			schema := datasourceSchema(ctx, t, c)

			config := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
				"name": tftypes.NewValue(tftypes.String, "a-cluster"),
			})

			req := datasource.ReadRequest{Config: tfsdk.Config{Schema: schema, Raw: config}}
			resp := datasource.ReadResponse{State: tfsdk.State{Schema: schema, Raw: config}}

			c.Read(ctx, req, &resp)

			if testCase.wantError != "" {
				if !diagnosticsContain(resp.Diagnostics, testCase.wantError) {
					t.Fatalf("Read() diagnostics = %v, want %q: the API never answered, so the list is unknown", resp.Diagnostics, testCase.wantError)
				}
				return
			}

			if resp.Diagnostics.HasError() {
				t.Fatalf("Read() diagnostics = %v, want the cluster read", resp.Diagnostics)
			}

			var read clusterDatasourceModel
			if diags := resp.State.Get(ctx, &read); diags.HasError() {
				t.Fatalf("Reading the recorded state = %v", diags)
			}

			got := make([]string, 0, len(read.Maintainers))
			for _, maintainer := range read.Maintainers {
				got = append(got, maintainer.PermissionUUID.ValueString())
			}
			if !slices.Equal(got, testCase.wantMaintainers) {
				t.Errorf("Recorded maintainers = %v, want %v", got, testCase.wantMaintainers)
			}
		})
	}
}
