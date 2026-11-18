package buildkite

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccNonAdminClusterRead(t *testing.T) {
	name := testAccFixtureClusterName
	uuid := testAccFixtureClusterUUID
	teamSlug := testAccFixturePipelineTeamSlug
	check := resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr("data.buildkite_cluster.fixture", "name", name),
		resource.TestCheckResourceAttr("data.buildkite_cluster.fixture", "uuid", uuid),
		resource.TestCheckResourceAttr("data.buildkite_cluster.fixture", "id", base64.StdEncoding.EncodeToString([]byte("Cluster---"+uuid))),
		resource.TestCheckResourceAttrSet("data.buildkite_team.maintainer", "uuid"),
		func(s *terraform.State) error {
			team := s.RootModule().Resources["data.buildkite_team.maintainer"]
			// Match the known maintainer by identity, not list order or count.
			// A forbidden REST lookup must not pass as an actually empty list.
			err := resource.TestCheckTypeSetElemNestedAttrs("data.buildkite_cluster.fixture", "maintainers.*", map[string]string{
				"actor_uuid": team.Primary.Attributes["uuid"],
				"actor_type": "team",
				"actor_slug": teamSlug,
			})(s)
			if err != nil {
				return fmt.Errorf("expected cluster-maintainer team %q; check the fixture assignment, token owner's team membership, and read_clusters scope: %w", teamSlug, err)
			}
			return nil
		},
	)
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccNonAdminPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
					data "buildkite_cluster" "fixture" {
						name = %q
					}
					data "buildkite_team" "maintainer" {
						slug = %q
					}
				`, name, teamSlug),
				Check: check,
			},
			{
				RefreshState: true,
				Check:        check,
			},
		},
	})
}

func TestAccBuildkiteClusterDatasource(t *testing.T) {
	t.Parallel()

	t.Run("can find a cluster", func(t *testing.T) {
		clusterName := testAccResourceName()
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
			// The cluster went away between the page read and this call. The list is not empty, it
			// is moot, and there is no cluster left to report it against.
			name:        "the cluster is gone by the time its maintainers are read",
			maintainers: stubResponse{status: http.StatusNotFound, body: `{"message":"Not Found"}`},
			wantError:   "Unable to fetch cluster maintainers",
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
			sch := datasourceSchema(ctx, t, c)

			config := nullObjectWith(ctx, t, sch.Type(), map[string]tftypes.Value{
				"name": tftypes.NewValue(tftypes.String, "a-cluster"),
			})

			req := datasource.ReadRequest{Config: tfsdk.Config{Schema: sch, Raw: config}}
			resp := datasource.ReadResponse{State: tfsdk.State{Schema: sch, Raw: config}}

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
