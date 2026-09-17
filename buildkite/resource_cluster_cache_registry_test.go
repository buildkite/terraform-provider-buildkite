package buildkite

import (
	"context"
	"encoding/json"
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
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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
	policy := `{"save":{"scopes":{"branch":true}}}`
	createdAt := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	var state clusterCacheRegistryResourceModel

	err := updateClusterCacheRegistryState(&state, CacheRegistryValues{
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
	if err != nil {
		t.Fatal(err)
	}

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
			if err := updateClusterCacheRegistryState(&state, CacheRegistryValues{Policy: &test.fromAPI}); err != nil {
				t.Fatal(err)
			}
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
		"mixed GraphQL errors":         {err: gqlerror.List{{Message: "No cache registry found"}, {Message: "Insufficient permissions"}}},
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
		"organization inaccessible":   {response: `{"data":{"organization":null}}`, wantError: true},
		"cluster inaccessible":        {response: `{"data":{"organization":{"cluster":null}}}`, wantError: true},
		"feature unavailable":         {response: `{"data":{"organization":{"cluster":{"cacheRegistries":null}}}}`, wantError: true},
		"resolver error":              {response: `{"errors":[{"message":"Cache Registries are not enabled"}]}`, wantError: true},
		"unverified empty connection": {response: `{"data":{"organization":{"cluster":{"cacheRegistries":{"edges":[],"pageInfo":{"hasNextPage":false}}}}}}`, wantError: true},
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
			values, err := resource.lookupCacheRegistry(context.Background(), &clusterCacheRegistryResourceModel{
				ID:          types.StringValue("cache-id"),
				ClusterUUID: types.StringValue("cluster-uuid"),
			})
			if (err != nil) != test.wantError {
				t.Fatalf("error = %v, wantError %t", err, test.wantError)
			}
			if (values != nil) != test.wantExists {
				t.Errorf("values = %v, want exists %t", values, test.wantExists)
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
		"object":            {value: types.StringValue(`{"save":{"scopes":{"branch":true}}}`)},
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

	configured := jsontypes.NewNormalizedValue(`{"save":{"scopes":{"branch":true}},"rules":[]}`)
	normalized := jsontypes.NewNormalizedValue(`{"rules":[],"save":{"scopes":{"branch":true}}}`)
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

func TestCacheRegistryPoliciesEquivalent(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		left, right string
		want        bool
	}{
		"empty defaults":      {`{}`, cacheRegistryEmptyPolicy, true},
		"missing scopes":      {`{"save":{},"restore":{}}`, cacheRegistryEmptyPolicy, true},
		"null rule name":      {cacheRegistryNullableNamePolicy, cacheRegistrySavePolicy, true},
		"null rule when":      {cacheRegistryNullableWhenPolicy, cacheRegistrySavePolicy, true},
		"both optional nulls": {cacheRegistryNullablePolicy, cacheRegistrySavePolicy, true},
		"named conditional rule": {
			`{"rules":[{"name":"Main","effect":"allow","action":"save","when":"claims.build_branch == 'main'"}]}`,
			`{"save":{"scopes":{}},"restore":{"scopes":[]},"rules":[{"name":"Main","effect":"allow","action":["save"],"when":"claims.build_branch == 'main'"}]}`, true,
		},
		"null object":                    {`null`, `{}`, false},
		"both null":                      {`null`, `null`, false},
		"malformed JSON":                 {`{"rules":`, `{}`, false},
		"trailing JSON":                  {`{} {}`, `{}`, false},
		"root array":                     {`[]`, `{}`, false},
		"root scalar":                    {`1`, `{}`, false},
		"null save":                      {`{"save":null}`, `{}`, false},
		"array save":                     {`{"save":[]}`, `{}`, false},
		"scalar restore":                 {`{"restore":false}`, `{}`, false},
		"both malformed sections":        {`{"restore":false}`, `{"restore":false}`, false},
		"null save scopes":               {`{"save":{"scopes":null}}`, `{}`, false},
		"object restore scopes":          {`{"restore":{"scopes":{}}}`, `{}`, false},
		"null rules":                     {`{"rules":null}`, `{}`, false},
		"object rules":                   {`{"rules":{}}`, `{}`, false},
		"null rule":                      {`{"rules":[null]}`, `{"rules":[null]}`, false},
		"different effect":               {`{"rules":[{"effect":"deny","action":"save"}]}`, cacheRegistrySavePolicy, false},
		"additional rule":                {cacheRegistryDefaultPolicy, cacheRegistrySavePolicy, false},
		"different scopes":               {`{"save":{"scopes":{"branch":true}}}`, `{"save":{"scopes":{"branch":false}}}`, false},
		"different name":                 {`{"rules":[{"name":"Save","effect":"allow","action":"save"}]}`, cacheRegistrySavePolicy, false},
		"different condition":            {`{"rules":[{"when":"false","effect":"allow","action":"save"}]}`, cacheRegistrySavePolicy, false},
		"unknown key not discarded":      {`{"extra":true}`, `{}`, false},
		"unknown rule key not discarded": {`{"rules":[{"extra":null,"effect":"allow","action":"save"}]}`, cacheRegistrySavePolicy, false},
		"large numbers stay distinct":    {`{"extra":9007199254740992}`, `{"extra":9007199254740993}`, false},
		"rule order matters": {
			`{"rules":[{"effect":"deny","action":"save"},{"effect":"allow","action":"save"}]}`,
			`{"rules":[{"effect":"allow","action":"save"},{"effect":"deny","action":"save"}]}`, false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := cacheRegistryPoliciesEquivalent(test.left, test.right); got != test.want {
				t.Errorf("equivalent(%s, %s) = %t, want %t", test.left, test.right, got, test.want)
			}
			if got := cacheRegistryPoliciesEquivalent(test.right, test.left); got != test.want {
				t.Errorf("reverse equivalence = %t, want %t", got, test.want)
			}
		})
	}
}

func TestClusterCacheRegistryApplyResponse(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"create", "update"} {
		for name, test := range map[string]struct {
			policy  *string
			message string
		}{
			"API null":          {nil, "null policy"},
			"JSON null":         {ptr("null"), "not null"},
			"malformed JSON":    {ptr(`{"rules":`), "valid JSON object"},
			"array":             {ptr(`[]`), "JSON object"},
			"malformed section": {ptr(`{"save":null}`), "save to be an object"},
			"different policy":  {ptr(cacheRegistryDefaultPolicy), "differs from the plan"},
		} {
			t.Run(operation+"/"+name, func(t *testing.T) {
				t.Parallel()
				ctx := context.Background()
				server, api := newCacheRegistryTestAPI(t)
				r := clusterCacheRegistryResource{client: NewClient(&clientConfig{
					org: "test-org", apiToken: "dummy", graphqlURL: server.URL, restURL: server.URL,
				})}
				var schema frameworkresource.SchemaResponse
				r.Schema(ctx, frameworkresource.SchemaRequest{}, &schema)
				model := clusterCacheRegistryResourceModel{
					ID: types.StringUnknown(), UUID: types.StringUnknown(), ClusterUUID: types.StringUnknown(),
					Slug: types.StringUnknown(), CreatedAt: types.StringUnknown(), UpdatedAt: types.StringUnknown(),
					ClusterID: types.StringValue("cluster-id"), Name: types.StringValue("Cache"),
					Policy: jsontypes.NewNormalizedValue(cacheRegistryNullablePolicy),
				}
				plan := tfsdk.Plan{Schema: schema.Schema}
				if diags := plan.Set(ctx, &model); diags.HasError() {
					t.Fatal(diags)
				}
				var prior tfsdk.State
				if operation == "update" {
					response := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: plan.Raw}}
					r.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &response)
					if response.Diagnostics.HasError() {
						t.Fatal(response.Diagnostics)
					}
					prior = response.State
					model.Name = types.StringValue("Renamed Cache")
					if diags := plan.Set(ctx, &model); diags.HasError() {
						t.Fatal(diags)
					}
				}
				api.mu.Lock()
				api.overridePolicy, api.responsePolicy = true, test.policy
				api.mu.Unlock()
				state := tfsdk.State{Schema: schema.Schema, Raw: plan.Raw}
				var diagnosticText string
				if operation == "create" {
					response := frameworkresource.CreateResponse{State: state}
					r.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &response)
					if !response.Diagnostics.HasError() {
						t.Fatal("expected create response diagnostic")
					}
					diagnosticText = fmt.Sprint(response.Diagnostics)
					state = response.State
				} else {
					response := frameworkresource.UpdateResponse{State: prior}
					r.Update(ctx, frameworkresource.UpdateRequest{Plan: plan, State: prior}, &response)
					if !response.Diagnostics.HasError() {
						t.Fatal("expected update response diagnostic")
					}
					diagnosticText = fmt.Sprint(response.Diagnostics)
					state = response.State
				}
				for _, text := range []string{test.message, "cache-id", "terraform plan", "Invalid Cache Registry " + operation + " response"} {
					if !strings.Contains(diagnosticText, text) {
						t.Errorf("diagnostics %s missing %q", diagnosticText, text)
					}
				}
				if name == "different policy" {
					for _, policy := range []string{cacheRegistryNullablePolicy, cacheRegistryDefaultPolicy} {
						if !strings.Contains(diagnosticText, policy) {
							t.Errorf("policy mismatch diagnostic omits %s: %s", policy, diagnosticText)
						}
					}
				}
				var got clusterCacheRegistryResourceModel
				if diags := state.Get(ctx, &got); diags.HasError() {
					t.Fatal(diags)
				}
				if got.ID.ValueString() != "cache-id" || got.UUID.ValueString() != "cache-uuid" || got.ClusterID.ValueString() != "cluster-id" || got.ClusterUUID.ValueString() != "cluster-uuid" {
					t.Fatalf("remote identity was lost: %+v", got)
				}
				wantPolicy := jsontypes.NewNormalizedPointerValue(test.policy)
				if test.policy != nil && !json.Valid([]byte(*test.policy)) {
					wantPolicy = jsontypes.NewNormalizedNull()
					if !strings.Contains(diagnosticText, *test.policy) {
						t.Errorf("diagnostics omit malformed response: %s", diagnosticText)
					}
				}
				if !got.Name.Equal(model.Name) || !got.Policy.Equal(wantPolicy) {
					t.Fatalf("state does not reflect API response: %+v", got)
				}
				if err := api.checkDestroyed(nil); err == nil {
					t.Fatal("expected a live registry after the mutation")
				}
				deleted := frameworkresource.DeleteResponse{State: state}
				r.Delete(ctx, frameworkresource.DeleteRequest{State: state}, &deleted)
				if deleted.Diagnostics.HasError() {
					t.Fatal(deleted.Diagnostics)
				}
				if err := api.checkDestroyed(nil); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestClusterCacheRegistryReadPolicy(t *testing.T) {
	t.Parallel()
	for name, test := range map[string]struct {
		policy     *string
		hideNode   bool
		wantError  bool
		wantPolicy jsontypes.Normalized
	}{
		"equivalent":                       {policy: ptr(cacheRegistrySavePolicy), wantPolicy: jsontypes.NewNormalizedValue(cacheRegistryNullablePolicy)},
		"drift":                            {policy: ptr(cacheRegistryDefaultPolicy), wantPolicy: jsontypes.NewNormalizedValue(cacheRegistryDefaultPolicy)},
		"API null":                         {wantError: true, wantPolicy: jsontypes.NewNormalizedNull()},
		"JSON null":                        {policy: ptr("null"), wantError: true, wantPolicy: jsontypes.NewNormalizedValue("null")},
		"malformed":                        {policy: ptr("{broken"), wantError: true, wantPolicy: jsontypes.NewNormalizedNull()},
		"node hidden but registry present": {hideNode: true, wantError: true, wantPolicy: jsontypes.NewNormalizedValue(cacheRegistryNullablePolicy)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			server, api := newCacheRegistryTestAPI(t)
			r := clusterCacheRegistryResource{client: NewClient(&clientConfig{
				org: "test-org", apiToken: "dummy", graphqlURL: server.URL, restURL: server.URL,
			})}
			var schema frameworkresource.SchemaResponse
			r.Schema(ctx, frameworkresource.SchemaRequest{}, &schema)
			plan := tfsdk.Plan{Schema: schema.Schema}
			if diags := plan.Set(ctx, &clusterCacheRegistryResourceModel{
				ClusterID: types.StringValue("cluster-id"), Name: types.StringValue("Cache"),
				Policy: jsontypes.NewNormalizedValue(cacheRegistryNullablePolicy),
			}); diags.HasError() {
				t.Fatal(diags)
			}
			created := frameworkresource.CreateResponse{State: tfsdk.State{Schema: schema.Schema, Raw: plan.Raw}}
			r.Create(ctx, frameworkresource.CreateRequest{Plan: plan}, &created)
			if created.Diagnostics.HasError() {
				t.Fatal(created.Diagnostics)
			}
			api.mu.Lock()
			api.registry["policy"], api.hideNode = test.policy, test.hideNode
			api.mu.Unlock()
			read := frameworkresource.ReadResponse{State: created.State}
			r.Read(ctx, frameworkresource.ReadRequest{State: created.State}, &read)
			if read.Diagnostics.HasError() != test.wantError {
				t.Fatalf("unexpected diagnostics: %v", read.Diagnostics)
			}
			var got clusterCacheRegistryResourceModel
			if diags := read.State.Get(ctx, &got); diags.HasError() {
				t.Fatal(diags)
			}
			if got.ID.ValueString() != "cache-id" || !got.Policy.Equal(test.wantPolicy) {
				t.Fatalf("unexpected refreshed state: %+v", got)
			}
			if test.hideNode {
				api.mu.Lock()
				if api.operations["getClusterCacheRegistries"] != 1 {
					t.Errorf("cluster fallback was not queried: %v", api.operations)
				}
				api.mu.Unlock()
			}
		})
	}
}

func testAccCacheRegistryPreCheck(t *testing.T) {
	if os.Getenv("BUILDKITE_CACHE_REGISTRIES_ACCEPTANCE") != "1" {
		t.Skip("live Cache Registry resources are untested; set BUILDKITE_CACHE_REGISTRIES_ACCEPTANCE=1 and TF_ACC=1 to enable acceptance tests")
	}
	testAccPreCheck(t)
}

func testAccCacheRegistryConfig(clusterName, registryName, description, policy string, optionalStrings ...string) string {
	policyAttribute := ""
	if policy != "" {
		policyAttribute = fmt.Sprintf("policy = jsonencode(%s)", policy)
	}
	stringsAttributes := fmt.Sprintf("description = %q\nemoji = %q\ncolor = %q", description, ":package:", "#BADA55")
	if len(optionalStrings) > 0 {
		stringsAttributes = optionalStrings[0]
	}
	return fmt.Sprintf(`
resource "buildkite_cluster" "cache_registry_test" {
  name = %q
}

resource "buildkite_cluster_cache_registry" "test" {
  cluster_id  = buildkite_cluster.cache_registry_test.id
  name        = %q
  %s
  %s
}
`, clusterName, registryName, stringsAttributes, policyAttribute)
}

func TestAccBuildkiteClusterCacheRegistryResource(t *testing.T) {
	config := testAccCacheRegistryConfig
	clusterName := "tf-cache-" + acctest.RandString(8)
	registryName := "Cache " + acctest.RandString(8)
	renamedRegistry := registryName + " renamed"
	resourceName := "buildkite_cluster_cache_registry.test"
	var defaultPolicy string

	testingresource.Test(t, testingresource.TestCase{
		PreCheck:                 func() { testAccCacheRegistryPreCheck(t) },
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
				Config: config(clusterName, renamedRegistry, "", `{ rules = [{ name = null, when = null, effect = "allow", action = "save" }] }`, `description = ""
emoji = ""
color = ""`),
				Check: testingresource.ComposeAggregateTestCheckFunc(
					testingresource.TestCheckResourceAttr(resourceName, "description", ""),
					testingresource.TestCheckResourceAttr(resourceName, "emoji", ""),
					testingresource.TestCheckResourceAttr(resourceName, "color", ""),
				),
			},
			{
				Config: config(clusterName, renamedRegistry, "  Case Preserved  ", `{ rules = [{ name = null, effect = "allow", action = "save" }] }`),
				Check: testingresource.ComposeAggregateTestCheckFunc(
					testingresource.TestCheckResourceAttr(resourceName, "description", "  Case Preserved  "),
					testingresource.TestCheckResourceAttr(resourceName, "emoji", ":package:"),
					testingresource.TestCheckResourceAttr(resourceName, "color", "#BADA55"),
				),
			},
			{
				Config: config(clusterName, renamedRegistry, "  Case Preserved  ", `{ rules = [{ when = null, effect = "allow", action = "save" }] }`),
			},
			{
				Config: config(clusterName, renamedRegistry, "", "", ""),
				Check: testingresource.ComposeAggregateTestCheckFunc(
					testingresource.TestCheckNoResourceAttr(resourceName, "description"),
					testingresource.TestCheckNoResourceAttr(resourceName, "emoji"),
					testingresource.TestCheckNoResourceAttr(resourceName, "color"),
					testingresource.TestCheckResourceAttrWith(resourceName, "policy", func(value string) error {
						if !cacheRegistryPoliciesEquivalent(value, cacheRegistrySavePolicy) {
							return fmt.Errorf("removing configured policy changed its rules: %s", value)
						}
						return nil
					}),
				),
			},
			{
				Config: config(clusterName, renamedRegistry, "Cleared policy", `{ save = { scopes = {} }, restore = { scopes = [] }, rules = [] }`),
				Check: testingresource.TestCheckResourceAttrWith(resourceName, "policy", func(value string) error {
					if !cacheRegistryPoliciesEquivalent(value, cacheRegistryEmptyPolicy) {
						return fmt.Errorf("policy was not cleared: %s", value)
					}
					return nil
				}),
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

		r := clusterCacheRegistryResource{client: &Client{genqlient: genqlientGraphql, organization: getenv("BUILDKITE_ORGANIZATION_SLUG")}}
		_, err := r.lookupCacheRegistry(context.Background(), &clusterCacheRegistryResourceModel{
			ID:             types.StringValue(resourceState.Primary.ID),
			OrganizationID: types.StringValue(resourceState.Primary.Attributes["organization_id"]),
			ClusterID:      types.StringValue(resourceState.Primary.Attributes["cluster_id"]),
			ClusterUUID:    types.StringValue(resourceState.Primary.Attributes["cluster_uuid"]),
		})
		if errors.Is(err, errCacheRegistryAbsent) {
			continue
		}
		if err != nil {
			return fmt.Errorf("checking destroyed cache registry: %w", err)
		}
		return fmt.Errorf("cache registry %s still exists", resourceState.Primary.ID)
	}
	return nil
}

func TestAccBuildkiteClusterCacheRegistryParentDeletion(t *testing.T) {
	config := testAccCacheRegistryConfig("tf-cache-parent-"+acctest.RandString(8), "Disposable cache", "Parent deletion acceptance", `{ rules = [{ name = null, when = null, effect = "allow", action = "save" }] }`)
	var oldClusterID, oldRegistryID string
	testingresource.Test(t, testingresource.TestCase{
		PreCheck:                 func() { testAccCacheRegistryPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterCacheRegistryDestroy,
		Steps: []testingresource.TestStep{
			{
				Config: config,
				Check: func(state *terraform.State) error {
					oldClusterID = state.RootModule().Resources["buildkite_cluster.cache_registry_test"].Primary.ID
					oldRegistryID = state.RootModule().Resources["buildkite_cluster_cache_registry.test"].Primary.ID
					_, err := deleteCluster(context.Background(), genqlientGraphql, organizationID, oldClusterID)
					if err != nil {
						return err
					}
					return testAccCheckClusterCacheRegistryDestroy(state)
				},
				ExpectNonEmptyPlan: true,
			},
			{
				Config: config,
				Check: func(state *terraform.State) error {
					if state.RootModule().Resources["buildkite_cluster.cache_registry_test"].Primary.ID == oldClusterID || state.RootModule().Resources["buildkite_cluster_cache_registry.test"].Primary.ID == oldRegistryID {
						return fmt.Errorf("deleted parent and registry were not recreated")
					}
					return nil
				},
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
