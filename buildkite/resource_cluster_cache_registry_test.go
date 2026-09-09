package buildkite

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	genqlient "github.com/Khan/genqlient/graphql"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	testingresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

func TestClusterCacheRegistrySchema(t *testing.T) {
	t.Parallel()

	var response frameworkresource.SchemaResponse
	clusterCacheRegistryResource{}.Schema(context.Background(), frameworkresource.SchemaRequest{}, &response)

	required := []string{"cluster_id", "name"}
	for _, name := range required {
		attribute := response.Schema.Attributes[name]
		if !attribute.IsRequired() {
			t.Errorf("attribute %q must be required", name)
		}
	}

	computed := []string{"id", "uuid", "cluster_uuid", "slug", "created_at", "updated_at"}
	for _, name := range computed {
		attribute := response.Schema.Attributes[name]
		if !attribute.IsComputed() {
			t.Errorf("attribute %q must be computed", name)
		}
	}

	clusterID := response.Schema.Attributes["cluster_id"].(resource_schema.StringAttribute)
	if len(clusterID.PlanModifiers) != 1 {
		t.Fatalf("cluster_id has %d plan modifiers, want RequiresReplace", len(clusterID.PlanModifiers))
	}

	policy := response.Schema.Attributes["policy"].(resource_schema.StringAttribute)
	if !policy.IsOptional() || !policy.IsComputed() {
		t.Fatal("policy must be optional and computed")
	}
	if _, ok := policy.CustomType.(jsontypes.NormalizedType); !ok {
		t.Fatalf("policy custom type is %T, want jsontypes.NormalizedType", policy.CustomType)
	}
	if len(policy.PlanModifiers) != 1 {
		t.Fatalf("policy has %d plan modifiers, want UseStateForUnknown", len(policy.PlanModifiers))
	}
}

func TestUpdateClusterCacheRegistryState(t *testing.T) {
	t.Parallel()

	description := "Shared cache"
	emoji := ":package:"
	color := "#BADA55"
	policy := `{"retention":{"max_age_days":30}}`
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	var state clusterCacheRegistryResourceModel

	updateClusterCacheRegistryState(&state, CacheRegistryValues{
		Id:          "cache-id",
		Uuid:        "cache-uuid",
		Name:        "Primary cache",
		Slug:        "primary-cache",
		Description: &description,
		Emoji:       &emoji,
		Color:       &color,
		Policy:      &policy,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		CacheRegistryCluster: CacheRegistryValuesCacheRegistryCluster{
			Id:   "cluster-id",
			Uuid: "cluster-uuid",
		},
	})

	checks := map[string]string{
		"id":           state.ID.ValueString(),
		"uuid":         state.UUID.ValueString(),
		"cluster_id":   state.ClusterID.ValueString(),
		"cluster_uuid": state.ClusterUUID.ValueString(),
		"name":         state.Name.ValueString(),
		"slug":         state.Slug.ValueString(),
		"description":  state.Description.ValueString(),
		"emoji":        state.Emoji.ValueString(),
		"color":        state.Color.ValueString(),
		"policy":       state.Policy.ValueString(),
		"created_at":   state.CreatedAt.ValueString(),
		"updated_at":   state.UpdatedAt.ValueString(),
	}
	want := map[string]string{
		"id": "cache-id", "uuid": "cache-uuid", "cluster_id": "cluster-id", "cluster_uuid": "cluster-uuid",
		"name": "Primary cache", "slug": "primary-cache", "description": description, "emoji": emoji, "color": color,
		"policy": policy, "created_at": "2026-01-02T03:04:05Z", "updated_at": "2026-01-02T04:04:05Z",
	}
	for name, got := range checks {
		if got != want[name] {
			t.Errorf("%s = %q, want %q", name, got, want[name])
		}
	}
}

func TestUpdateClusterCacheRegistryStatePolicy(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		configured string
		fromAPI    string
		want       string
	}{
		"expanded defaults keep the configured text": {
			configured: `{"save":{"scopes":{"branch":true}}}`,
			fromAPI:    `{"save":{"scopes":{"branch":true}},"restore":{"scopes":[]},"rules":[]}`,
			want:       `{"save":{"scopes":{"branch":true}}}`,
		},
		"widened rule actions keep the configured text": {
			configured: `{"rules":[{"effect":"allow","action":"save"}]}`,
			fromAPI:    `{"save":{"scopes":{}},"restore":{"scopes":[]},"rules":[{"effect":"allow","action":["save"]}]}`,
			want:       `{"rules":[{"effect":"allow","action":"save"}]}`,
		},
		"a genuinely different policy takes the API value": {
			configured: `{"rules":[]}`,
			fromAPI:    `{"save":{"scopes":{}},"restore":{"scopes":[]},"rules":[{"effect":"deny","action":["save"]}]}`,
			want:       `{"save":{"scopes":{}},"restore":{"scopes":[]},"rules":[{"effect":"deny","action":["save"]}]}`,
		},
		"documented example keeps the configured text": {
			configured: `{"save":{"scopes":{"branch":true}},"restore":{"scopes":[{"branch":"main"}]},"rules":[{"effect":"allow","action":"save"},{"effect":"allow","action":"restore"}]}`,
			fromAPI:    `{"save":{"scopes":{"branch":true}},"restore":{"scopes":[{"branch":"main"}]},"rules":[{"effect":"allow","action":["save"]},{"effect":"allow","action":["restore"]}]}`,
			want:       `{"save":{"scopes":{"branch":true}},"restore":{"scopes":[{"branch":"main"}]},"rules":[{"effect":"allow","action":"save"},{"effect":"allow","action":"restore"}]}`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			state := clusterCacheRegistryResourceModel{Policy: jsontypes.NewNormalizedValue(test.configured)}
			updateClusterCacheRegistryState(&state, CacheRegistryValues{Policy: &test.fromAPI})
			if got := state.Policy.ValueString(); got != test.want {
				t.Errorf("policy = %q, want %q", got, test.want)
			}
		})
	}
}

func TestIsCacheRegistryNotFoundError(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		err  error
		want bool
	}{
		"cache registry GraphQL error": {err: gqlerror.List{{Message: "No cache registry found"}}, want: true},
		"organization GraphQL error":   {err: gqlerror.List{{Message: "No organization found"}}},
		"plain cache registry error":   {err: errors.New("No cache registry found")},
		"other GraphQL error":          {err: gqlerror.List{{Message: "Insufficient permissions"}}},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := isCacheRegistryNotFoundError(test.err); got != test.want {
				t.Errorf("isCacheRegistryNotFoundError() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestCacheRegistryExistsInCluster(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		response   string
		wantExists bool
		wantError  bool
	}{
		"organization inaccessible": {response: `{"data":{"organization":null}}`, wantError: true},
		"cluster inaccessible":      {response: `{"data":{"organization":{"cluster":null}}}`, wantError: true},
		"feature unavailable":       {response: `{"data":{"organization":{"cluster":{"cacheRegistries":null}}}}`, wantError: true},
		"resolver error":            {response: `{"errors":[{"message":"Cache Registries are not enabled"}]}`, wantError: true},
		"registry absent":           {response: `{"data":{"organization":{"cluster":{"cacheRegistries":{"edges":[],"pageInfo":{"hasNextPage":false}}}}}}`},
		"registry present":          {response: `{"data":{"organization":{"cluster":{"cacheRegistries":{"edges":[{"node":{"id":"cache-id"}}],"pageInfo":{"hasNextPage":false}}}}}}`, wantExists: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(test.response))
			}))
			defer server.Close()

			resource := clusterCacheRegistryResource{client: &Client{
				genqlient:    genqlient.NewClient(server.URL, server.Client()),
				organization: "test-org",
			}}
			exists, err := resource.cacheRegistryExistsInCluster(context.Background(), DefaultTimeout, clusterCacheRegistryResourceModel{
				ID:          types.StringValue("cache-id"),
				ClusterUUID: types.StringValue("cluster-uuid"),
			})
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %t", err, test.wantError)
			}
			if exists != test.wantExists {
				t.Errorf("exists = %t, want %t", exists, test.wantExists)
			}
		})
	}
}

func TestCacheRegistryPolicy(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		value     types.String
		wantError bool
	}{
		"object":            {value: types.StringValue(`{"retention":{"max_age_days":30}}`)},
		"empty object":      {value: types.StringValue(`{}`)},
		"malformed":         {value: types.StringValue(`{"retention":`), wantError: true},
		"array":             {value: types.StringValue(`[]`), wantError: true},
		"null":              {value: types.StringValue(`null`), wantError: true},
		"terraform null":    {value: types.StringNull()},
		"terraform unknown": {value: types.StringUnknown()},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			response := &validator.StringResponse{}
			cacheRegistryPolicyValidator{}.ValidateString(context.Background(), validator.StringRequest{Path: path.Root("policy"), ConfigValue: test.value}, response)
			if response.Diagnostics.HasError() != test.wantError {
				t.Fatalf("HasError() = %t, want %t: %v", response.Diagnostics.HasError(), test.wantError, response.Diagnostics)
			}
		})
	}

	configured := jsontypes.NewNormalizedValue(`{"retention":{"max_age_days":30},"enabled":true}`)
	normalized := jsontypes.NewNormalizedValue(`{"enabled":true,"retention":{"max_age_days":30}}`)
	equal, diagnostics := configured.StringSemanticEquals(context.Background(), normalized)
	if diagnostics.HasError() || !equal {
		t.Fatalf("equivalent policies were not semantically equal: %v", diagnostics)
	}

	if !cacheRegistryPoliciesEquivalent(
		`{"save":{"scopes":{"branch":true}}}`,
		`{"rules":[],"restore":{"scopes":[]},"save":{"scopes":{"branch":true}}}`,
	) {
		t.Fatal("API-expanded policy must be equivalent to its abbreviated configuration")
	}
	if cacheRegistryPoliciesEquivalent(`{"rules":[]}`, `{"rules":[{"effect":"allow","action":"save"}]}`) {
		t.Fatal("policies with different rules must not be equivalent")
	}
	if !cacheRegistryPoliciesEquivalent(
		`{"rules":[{"effect":"allow","action":"save"},{"effect":"allow","action":"restore"}]}`,
		`{"save":{"scopes":{}},"restore":{"scopes":[]},"rules":[{"effect":"allow","action":["save"]},{"effect":"allow","action":["restore"]}]}`,
	) {
		t.Fatal("API-normalized rule actions must be equivalent to scalar configuration")
	}
}

func TestAccBuildkiteClusterCacheRegistryResource(t *testing.T) {
	localPreCheck := func() {
		testAccPreCheck(t)
		if endpoint := os.Getenv("BUILDKITE_GRAPHQL_URL"); !strings.Contains(endpoint, "graphql.buildkite.localhost") {
			t.Skip("cache registry acceptance tests require the local Buildkite GraphQL endpoint")
		}
	}

	config := func(clusterName, registryName, description, policy string) string {
		policyAttribute := ""
		if policy != "" {
			policyAttribute = fmt.Sprintf("policy = jsonencode(%s)", policy)
		}
		return fmt.Sprintf(`
resource "buildkite_cluster" "cache_registry_test" {
  name = %q
}

resource "buildkite_cluster_cache_registry" "test" {
  cluster_id  = buildkite_cluster.cache_registry_test.id
  name        = %q
  description = %q
  emoji       = ":package:"
  color       = "#BADA55"
  %s
}
`, clusterName, registryName, description, policyAttribute)
	}

	clusterName := "tf-cache-" + acctest.RandString(8)
	registryName := "Cache " + acctest.RandString(8)
	renamedRegistry := registryName + " renamed"
	resourceName := "buildkite_cluster_cache_registry.test"
	var defaultPolicy string

	testingresource.Test(t, testingresource.TestCase{
		PreCheck:                 localPreCheck,
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterCacheRegistryDestroy,
		Steps: []testingresource.TestStep{
			{
				Config: config(clusterName, registryName, "Created by Terraform", ""),
				Check: testingresource.ComposeAggregateTestCheckFunc(
					testingresource.TestCheckResourceAttr(resourceName, "name", registryName),
					testingresource.TestCheckResourceAttrSet(resourceName, "id"),
					testingresource.TestCheckResourceAttrSet(resourceName, "uuid"),
					testingresource.TestCheckResourceAttrSet(resourceName, "cluster_uuid"),
					testingresource.TestCheckResourceAttrSet(resourceName, "slug"),
					testingresource.TestCheckResourceAttrWith(resourceName, "policy", func(value string) error {
						defaultPolicy = value
						return nil
					}),
					testingresource.TestCheckResourceAttrSet(resourceName, "created_at"),
					testingresource.TestCheckResourceAttrSet(resourceName, "updated_at"),
				),
			},
			{
				Config: config(clusterName, registryName, "Updated without policy", ""),
				Check: testingresource.ComposeAggregateTestCheckFunc(
					testingresource.TestCheckResourceAttr(resourceName, "description", "Updated without policy"),
					testingresource.TestCheckResourceAttrWith(resourceName, "policy", func(value string) error {
						if value != defaultPolicy {
							return fmt.Errorf("policy changed during unrelated update: got %q, want %q", value, defaultPolicy)
						}
						return nil
					}),
				),
			},
			{
				Config: config(clusterName, renamedRegistry, "Updated by Terraform", `{ save = { scopes = { branch = true } }, restore = { scopes = [] }, rules = [] }`),
				Check: testingresource.ComposeAggregateTestCheckFunc(
					testingresource.TestCheckResourceAttr(resourceName, "name", renamedRegistry),
					testingresource.TestCheckResourceAttr(resourceName, "description", "Updated by Terraform"),
					testingresource.TestCheckResourceAttrSet(resourceName, "policy"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"policy"},
			},
			{
				Config:   config(clusterName, renamedRegistry, "Updated by Terraform", `{ rules = [], restore = { scopes = [] }, save = { scopes = { branch = true } } }`),
				PlanOnly: true,
			},
			{
				Config:             config(clusterName, renamedRegistry, "Updated by Terraform", `{ save = { scopes = { branch = true } }, restore = { scopes = [] }, rules = [] }`),
				Check:              testAccDeleteClusterCacheRegistryOutOfBand(resourceName),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccCheckClusterCacheRegistryDestroy(state *terraform.State) error {
	for _, resourceState := range state.RootModule().Resources {
		if resourceState.Type != "buildkite_cluster_cache_registry" {
			continue
		}

		result, err := getCacheRegistryByNode(context.Background(), genqlientGraphql, resourceState.Primary.ID)
		if err != nil {
			return fmt.Errorf("checking destroyed cache registry: %w", err)
		}
		if cacheRegistry, ok := result.Node.(*getCacheRegistryByNodeNodeCacheRegistry); ok && cacheRegistry != nil {
			return fmt.Errorf("cache registry %s still exists", resourceState.Primary.ID)
		}
	}
	return nil
}

func testAccDeleteClusterCacheRegistryOutOfBand(name string) testingresource.TestCheckFunc {
	return func(state *terraform.State) error {
		registry, ok := state.RootModule().Resources[name]
		if !ok {
			return fmt.Errorf("cache registry %s not found in state", name)
		}
		_, err := deleteCacheRegistry(context.Background(), genqlientGraphql, organizationID, registry.Primary.ID)
		return err
	}
}
