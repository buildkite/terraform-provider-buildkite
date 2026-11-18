package buildkite

import (
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteClustersDatasource(t *testing.T) {
	t.Run("clusters data source can be loaded with defaults", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: `data "buildkite_clusters" "clusters" {}`,
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttrSet("data.buildkite_clusters.clusters", "clusters.0.name"),
						resource.TestCheckResourceAttrSet("data.buildkite_clusters.clusters", "clusters.0.maintainers.#"),
					),
				},
			},
		})
	})
}

// A failed maintainers call used to be reported as a cluster with no maintainers, which whatever
// depends on the list then consumes as fact. A refusal is a fair reading of "none you may see";
// unexpected failures must not be reported as an empty maintainers list.
func TestUpdateClustersDatasourceStateClassifiesAFailedMaintainerRead(t *testing.T) {
	t.Parallel()

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
			// A token without manage_cluster. The cluster itself read fine, so reporting no
			// maintainers is more useful than failing the whole data source.
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

			server, _ := newRetryStub(t, testCase.maintainers)
			defer server.Close()

			client := newRetryTestClient(t, server.URL, 0, time.Millisecond)

			edge := GetOrganizationClustersOrganizationClustersClusterConnectionEdgesClusterEdge{
				Node: GetOrganizationClustersOrganizationClustersClusterConnectionEdgesClusterEdgeNodeCluster{
					Id:   "cluster-id",
					Uuid: "cluster-uuid",
					Name: "a-cluster",
				},
			}

			var state clustersDatasourceModel
			resp := &datasource.ReadResponse{}

			updateClustersDatasourceState(t.Context(), client, resp, &state, edge)

			if testCase.wantError != "" {
				if !diagnosticsContain(resp.Diagnostics, testCase.wantError) {
					t.Fatalf("Diagnostics = %v, want %q: the API never answered, so the list is unknown", resp.Diagnostics, testCase.wantError)
				}
				if len(state.Clusters) != 0 {
					t.Errorf("Recorded %d clusters, want none: the maintainers are unknown, so the cluster cannot be reported", len(state.Clusters))
				}
				return
			}

			if resp.Diagnostics.HasError() {
				t.Fatalf("Diagnostics = %v, want the cluster recorded", resp.Diagnostics)
			}
			if len(state.Clusters) != 1 {
				t.Fatalf("Recorded %d clusters, want 1", len(state.Clusters))
			}

			got := make([]string, 0, len(state.Clusters[0].Maintainers))
			for _, maintainer := range state.Clusters[0].Maintainers {
				got = append(got, maintainer.PermissionUUID.ValueString())
			}
			if !slices.Equal(got, testCase.wantMaintainers) {
				t.Errorf("Recorded maintainers = %v, want %v", got, testCase.wantMaintainers)
			}
		})
	}
}

func TestClustersDatasourceReadHandlesFailedMaintainerRead(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t,
				stubResponse{status: http.StatusOK, body: `{"data":{"organization":{"clusters":{
					"pageInfo": {"endCursor": "", "hasNextPage": false},
					"edges": [
						{"node": {"id": "cluster-1-id", "uuid": "cluster-1-uuid", "name": "first", "description": null, "emoji": null, "color": null}},
						{"node": {"id": "cluster-2-id", "uuid": "cluster-2-uuid", "name": "second", "description": null, "emoji": null, "color": null}}
					]
				}}}}`},
				stubResponse{status: status, body: `{"message":"maintainer read failed"}`},
				stubResponse{status: http.StatusOK, body: `[{"id":"second-permission","actor":{"id":"team-id","type":"team","name":"a-team"}}]`},
			)
			defer server.Close()

			c := &clustersDatasource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}
			ctx := t.Context()
			sch := datasourceSchema(ctx, t, c)
			config := nullObjectWith(ctx, t, sch.Type(), nil)
			req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: config}}
			resp := datasource.ReadResponse{State: tfsdk.State{Schema: sch, Raw: config}}

			c.Read(ctx, req, &resp)

			if status == http.StatusInternalServerError {
				if !diagnosticsContain(resp.Diagnostics, "Unable to fetch cluster maintainers") {
					t.Fatalf("Read() diagnostics = %v, want the maintainer failure reported", resp.Diagnostics)
				}
				if got := requests.Load(); got != 2 {
					t.Errorf("Made %d requests, want 2: stop after the first failed maintainer read", got)
				}
				return
			}

			if resp.Diagnostics.HasError() {
				t.Fatalf("Read() diagnostics = %v, want the deleted cluster omitted", resp.Diagnostics)
			}
			var state clustersDatasourceModel
			if diags := resp.State.Get(ctx, &state); diags.HasError() {
				t.Fatalf("Read() state diagnostics = %v", diags)
			}
			if len(state.Clusters) != 1 || state.Clusters[0].UUID.ValueString() != "cluster-2-uuid" {
				t.Fatalf("Read() clusters = %v, want only the surviving second cluster", state.Clusters)
			}
			if maintainers := state.Clusters[0].Maintainers; len(maintainers) != 1 || maintainers[0].PermissionUUID.ValueString() != "second-permission" {
				t.Errorf("Read() maintainers = %v, want second-permission", maintainers)
			}
		})
	}
}
