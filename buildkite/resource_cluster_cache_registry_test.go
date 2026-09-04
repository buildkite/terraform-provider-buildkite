package buildkite

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resource_schema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	testingresource "github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
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

	testingresource.Test(t, testingresource.TestCase{
		PreCheck:                 localPreCheck,
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []testingresource.TestStep{
			{
				Config: config(clusterName, registryName, "Created by Terraform", ""),
				Check: testingresource.ComposeAggregateTestCheckFunc(
					testingresource.TestCheckResourceAttr(resourceName, "name", registryName),
					testingresource.TestCheckResourceAttrSet(resourceName, "id"),
					testingresource.TestCheckResourceAttrSet(resourceName, "uuid"),
					testingresource.TestCheckResourceAttrSet(resourceName, "cluster_uuid"),
					testingresource.TestCheckResourceAttrSet(resourceName, "slug"),
					testingresource.TestCheckResourceAttrSet(resourceName, "policy"),
					testingresource.TestCheckResourceAttrSet(resourceName, "created_at"),
					testingresource.TestCheckResourceAttrSet(resourceName, "updated_at"),
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
