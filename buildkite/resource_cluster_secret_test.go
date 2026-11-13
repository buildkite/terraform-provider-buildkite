package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"
	"time"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestClusterSecretReadPreservesRateLimitErrorAtDeadline(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("RateLimit-Reset", "5")
		http.Error(w, `{"message":"Organization API rate limit exceeded"}`, http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := NewClient(&clientConfig{restURL: server.URL, org: "test-org", maxRetries: DefaultRetryMaxAttempts})
	client.timeouts = configuredTimeouts("read", "200ms")

	// Hold the HTTP retry result briefly after its deadline. This forces the ordering where
	// the SDK waiter gives up before its callback can publish the already-captured 429.
	transport := client.http.Transport
	finished := make(chan struct{})
	client.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		defer close(finished)
		resp, err := transport.RoundTrip(req)
		if req.Context().Err() != nil {
			time.Sleep(50 * time.Millisecond)
		}
		return resp, err
	})
	t.Cleanup(func() { <-finished })

	diags := runRead(t.Context(), t, &clusterSecretResource{client: client}, map[string]tftypes.Value{
		"id":         tftypes.NewValue(tftypes.String, "secret-456"),
		"cluster_id": tftypes.NewValue(tftypes.String, "cluster-123"),
		"value":      tftypes.NewValue(tftypes.String, "state-only-secret"),
	})
	for _, want := range []string{"429", "Organization API rate limit exceeded", "context deadline exceeded"} {
		if !diagnosticsContain(diags, want) {
			t.Errorf("Read diagnostics = %v, want %q", diags, want)
		}
	}
	if diagnosticsContain(diags, "state-only-secret") {
		t.Fatal("Read diagnostics exposed the secret value")
	}
}

func TestClusterSecretReadRecoversFromRateLimit(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			w.Header().Set("RateLimit-Reset", "5")
			http.Error(w, `{"message":"Organization API rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		_, _ = fmt.Fprint(w, `{"id":"secret-456","key":"MY_SECRET","description":"updated","created_at":"2026-10-21T15:00:00Z","updated_at":"2026-10-21T15:28:00Z"}`)
	}))
	defer server.Close()
	// Keep the production timeout and backoff defaults: the acceptance configs use these too.
	client := NewClient(&clientConfig{restURL: server.URL, org: "test-org", maxRetries: DefaultRetryMaxAttempts})
	r := &clusterSecretResource{client: client}
	state := tfsdk.State{Schema: resourceSchema(t.Context(), t, r)}
	if diags := state.Set(t.Context(), &clusterSecretResourceModel{
		ID: types.StringValue("secret-456"), ClusterID: types.StringValue("cluster-123"),
		Key: types.StringValue("MY_SECRET"), Value: types.StringValue("state-only-secret"),
	}); diags.HasError() {
		t.Fatal(diags)
	}
	resp := fwresource.ReadResponse{State: state}
	r.Read(t.Context(), fwresource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got clusterSecretResourceModel
	if diags := resp.State.Get(t.Context(), &got); diags.HasError() {
		t.Fatal(diags)
	}
	server.Close()
	if requests != 2 || got.ID.ValueString() != "secret-456" || got.Key.ValueString() != "MY_SECRET" ||
		got.Description.ValueString() != "updated" || got.UpdatedAt.ValueString() != "2026-10-21T15:28:00Z" ||
		got.Value.ValueString() != "state-only-secret" {
		t.Fatal("Read did not recover and refresh metadata while preserving the secret value")
	}
}

func TestClusterSecretReadFailureHandling(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		status int
		cancel bool
		want   string
	}{
		{name: "request deadline without response", want: "context deadline exceeded"},
		{name: "caller cancellation without response", cancel: true, want: "context canceled"},
		{name: "forbidden is not removed or retried", status: http.StatusForbidden, want: "403"},
		{name: "not found removes state", status: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				requests++
				if tc.status != 0 {
					http.Error(w, `{"message":"Request refused"}`, tc.status)
					return
				}
				if tc.cancel {
					cancel()
				}
				<-req.Context().Done()
			}))
			defer server.Close()
			client := NewClient(&clientConfig{restURL: server.URL, org: "test-org", maxRetries: DefaultRetryMaxAttempts})
			client.timeouts = configuredTimeouts("read", "200ms")
			r := &clusterSecretResource{client: client}
			state := tfsdk.State{Schema: resourceSchema(t.Context(), t, r)}
			if diags := state.Set(t.Context(), &clusterSecretResourceModel{
				ID: types.StringValue("secret-456"), ClusterID: types.StringValue("cluster-123"),
			}); diags.HasError() {
				t.Fatal(diags)
			}
			resp := fwresource.ReadResponse{State: state}
			started := time.Now()
			r.Read(ctx, fwresource.ReadRequest{State: state}, &resp)
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Errorf("Read took %s, want bounded cancellation/deadline handling", elapsed)
			}
			if tc.status == http.StatusNotFound {
				if resp.Diagnostics.HasError() || !resp.State.Raw.IsNull() {
					t.Fatalf("404 should remove state without error, got %v", resp.Diagnostics)
				}
			} else {
				if !diagnosticsContain(resp.Diagnostics, tc.want) {
					t.Errorf("Read diagnostics = %v, want %q", resp.Diagnostics, tc.want)
				}
				if !resp.State.Raw.Equal(state.Raw) {
					t.Error("failed Read changed the prior state")
				}
				if tc.status == 0 && diagnosticsContain(resp.Diagnostics, "status:") {
					t.Error("Read reported an HTTP status without a response")
				}
			}
			// Wait for the server handler before checking its request count.
			server.Close()
			if requests != 1 {
				t.Errorf("made %d requests, want 1", requests)
			}
		})
	}
}

func TestAccNonAdminClusterSecret(t *testing.T) {
	const address = "buildkite_cluster_secret.temporary"
	key := testAccResourceKey()
	initialValue, rotatedValue := acctest.RandString(24), acctest.RandString(32)
	initialPolicy := "- pipeline_slug: terraform-provider-secret-fixture\n  build_branch: main"
	updatedPolicy := "- pipeline_slug: terraform-provider-secret-fixture\n  build_branch: develop"
	baseConfig := fmt.Sprintf(`
		data "buildkite_cluster" "fixture" { name = %q }
	`, testAccFixtureClusterName)
	config := func(value, description, policy string) string {
		return baseConfig + fmt.Sprintf(`
			resource "buildkite_cluster_secret" "temporary" {
				cluster_id = data.buildkite_cluster.fixture.uuid
				key = %q
				value = %q
				description = %q
				policy = %q
			}
		`, key, value, description, policy)
	}
	var secretID, createdAt string
	check := func(value, description, policy string) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			rs, ok := s.RootModule().Resources[address]
			if !ok || rs.Primary == nil || rs.Primary.ID == "" {
				return fmt.Errorf("temporary secret is missing from Terraform state")
			}
			if secretID == "" {
				secretID, createdAt = rs.Primary.ID, rs.Primary.Attributes["created_at"]
			}
			if rs.Primary.Attributes["value"] != value {
				return fmt.Errorf("temporary secret value was not preserved in state")
			}
			if err := resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("data.buildkite_cluster.fixture", "uuid", testAccFixtureClusterUUID),
				resource.TestCheckResourceAttr(address, "id", secretID),
				resource.TestCheckResourceAttr(address, "cluster_id", testAccFixtureClusterUUID),
				resource.TestCheckResourceAttr(address, "key", key),
				resource.TestCheckResourceAttr(address, "description", description),
				resource.TestCheckResourceAttr(address, "policy", policy),
				resource.TestCheckResourceAttrSet(address, "created_at"),
				resource.TestCheckResourceAttr(address, "created_at", createdAt),
				resource.TestCheckResourceAttrSet(address, "updated_at"),
			)(s); err != nil {
				return err
			}
			client := getTestClient()
			remote, err := client.GetClusterSecret(context.Background(), client.organization, testAccFixtureClusterUUID, secretID)
			if err != nil {
				return err
			}
			if remote.ID != secretID || remote.Key != key || remote.CreatedAt != createdAt || remote.Description == nil || *remote.Description != description || remote.Policy == nil || *remote.Policy != policy {
				return fmt.Errorf("remote secret identity, creation time, description, or policy changed unexpectedly")
			}
			if remote.Value != "" {
				return fmt.Errorf("secret metadata read unexpectedly returned a value")
			}
			return nil
		}
	}
	updateChecks := resource.ConfigPlanChecks{
		PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccNonAdminPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterSecretDestroy,
		Steps: []resource.TestStep{
			{
				Config: config(initialValue, "Initial secret description", initialPolicy),
				Check:  check(initialValue, "Initial secret description", initialPolicy),
			},
			{
				Config:           config(initialValue, "Updated secret description", updatedPolicy),
				Check:            check(initialValue, "Updated secret description", updatedPolicy),
				ConfigPlanChecks: updateChecks,
			},
			{
				Config:           config(rotatedValue, "Updated secret description", updatedPolicy),
				Check:            check(rotatedValue, "Updated secret description", updatedPolicy),
				ConfigPlanChecks: updateChecks,
			},
			{RefreshState: true, Check: check(rotatedValue, "Updated secret description", updatedPolicy)},
			{
				ResourceName:            address,
				ImportState:             true,
				ImportStateIdFunc:       func(*terraform.State) (string, error) { return testAccFixtureClusterUUID + "/" + secretID, nil },
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"value"},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 || states[0].ID != secretID || states[0].Attributes["value"] != "" {
						return fmt.Errorf("secret import must preserve identity without retrieving its value")
					}
					return nil
				},
			},
			{
				Config: baseConfig,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroy)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.buildkite_cluster.fixture", "uuid", testAccFixtureClusterUUID),
					func(s *terraform.State) error {
						if _, ok := s.RootModule().Resources[address]; ok {
							return fmt.Errorf("deleted secret remains in Terraform state")
						}
						return testAccCheckClusterSecretDeleted(getTestClient(), testAccFixtureClusterUUID, secretID)
					},
				),
			},
		},
	})
}

func TestAccBuildkiteClusterSecret_writeOnlyValue(t *testing.T) {
	secretKey := testAccResourceKey()
	secretValue1 := acctest.RandString(20)
	secretValue2 := acctest.RandString(20)
	clusterName := testAccResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterSecretDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterSecretWriteOnlyConfig(clusterName, secretKey, secretValue1, "version-1", "Initial description"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "key", secretKey),
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Initial description"),
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "value_wo_version", "version-1"),
					resource.TestCheckNoResourceAttr("buildkite_cluster_secret.test", "value"),
					resource.TestCheckNoResourceAttr("buildkite_cluster_secret.test", "value_wo"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "id"),
				),
			},
			{
				Config: testAccClusterSecretWriteOnlyConfig(clusterName, secretKey, secretValue2, "version-2", "Updated description"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "value_wo_version", "version-2"),
					resource.TestCheckNoResourceAttr("buildkite_cluster_secret.test", "value"),
					resource.TestCheckNoResourceAttr("buildkite_cluster_secret.test", "value_wo"),
				),
			},
		},
	})
}

func TestAccBuildkiteClusterSecret_migrateValueToWriteOnlyValue(t *testing.T) {
	secretKey := testAccResourceKey()
	secretValue1 := acctest.RandString(20)
	secretValue2 := acctest.RandString(20)
	clusterName := testAccResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterSecretDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterSecretConfig(clusterName, secretKey, secretValue1, "Legacy value"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "key", secretKey),
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Legacy value"),
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "value", secretValue1),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "id"),
				),
			},
			{
				Config: testAccClusterSecretWriteOnlyConfig(clusterName, secretKey, secretValue2, "version-1", "Write-only value"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Write-only value"),
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "value_wo_version", "version-1"),
					resource.TestCheckNoResourceAttr("buildkite_cluster_secret.test", "value"),
					resource.TestCheckNoResourceAttr("buildkite_cluster_secret.test", "value_wo"),
				),
			},
		},
	})
}

func TestBuildkiteClusterSecret_valueValidation(t *testing.T) {
	t.Parallel()

	const secretKey = "TEST_SECRET_VALIDATION"
	const clusterName = "validation-cluster"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config:      testAccClusterSecretConfigWithoutValue(clusterName, secretKey),
				ExpectError: regexp.MustCompile("Missing Attribute Configuration"),
			},
			{
				Config:      testAccClusterSecretConfigWithBothValues(clusterName, secretKey, "legacy-value", "write-only-value", "version-1"),
				ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
			},
			{
				Config:      testAccClusterSecretConfigWithoutWriteOnlyVersion(clusterName, secretKey, "write-only-value"),
				ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
			},
			{
				Config:      testAccClusterSecretConfigWithEmptyWriteOnlyVersion(clusterName, secretKey, "write-only-value"),
				ExpectError: regexp.MustCompile("Invalid Attribute Value Length"),
			},
		},
	})
}

func TestAccBuildkiteClusterSecret_withPolicy(t *testing.T) {
	secretKey := testAccResourceKey()
	secretValue := acctest.RandString(20)
	clusterName := testAccResourceName()

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterSecretDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterSecretConfigWithPolicy(clusterName, secretKey, secretValue, "my-pipeline", "main"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "key", secretKey),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "policy"),
				),
			},
			{
				Config: testAccClusterSecretConfigWithPolicy(clusterName, secretKey, secretValue, "updated-pipeline", "develop"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "policy"),
				),
			},
		},
	})
}

func testAccCheckClusterSecretDestroy(s *terraform.State) error {
	client := getTestClient()

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_secret" {
			continue
		}

		clusterID := rs.Primary.Attributes["cluster_id"]
		secretID := rs.Primary.ID

		if err := testAccCheckClusterSecretDeleted(client, clusterID, secretID); err != nil {
			return err
		}
	}

	return nil
}

func testAccCheckClusterSecretDeleted(client *Client, clusterID, secretID string) error {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/secrets/%s", client.organization, clusterID, secretID)
	var result struct{}
	err := client.makeRequest(context.Background(), http.MethodGet, path, nil, &result)
	if isAPIStatus(err, http.StatusNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking cluster secret deletion: %w", err)
	}
	return fmt.Errorf("cluster secret %s still exists", secretID)
}

func testAccClusterSecretConfig(clusterName, key, value, description string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "%s"
    api_token    = "%s"
}

resource "buildkite_cluster" "test" {
    name        = "%s-cluster"
    description = "Test cluster for secrets"
}

resource "buildkite_cluster_secret" "test" {
    cluster_id  = buildkite_cluster.test.uuid
    key         = "%s"
    value       = "%s"
    description = "%s"
}
`, getenv("BUILDKITE_ORGANIZATION_SLUG"), os.Getenv("BUILDKITE_API_TOKEN"), clusterName, key, value, description)
}

func testAccClusterSecretWriteOnlyConfig(clusterName, key, value, version, description string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "%s"
    api_token    = "%s"
}

resource "buildkite_cluster" "test" {
    name        = "%s-cluster"
    description = "Test cluster for secrets"
}

resource "buildkite_cluster_secret" "test" {
    cluster_id       = buildkite_cluster.test.uuid
    key              = "%s"
    value_wo         = "%s"
    value_wo_version = "%s"
    description      = "%s"
}
`, getenv("BUILDKITE_ORGANIZATION_SLUG"), os.Getenv("BUILDKITE_API_TOKEN"), clusterName, key, value, version, description)
}

func testAccClusterSecretConfigWithoutValue(clusterName, key string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "test-org"
    api_token    = "test-token"
    graphql_url  = "http://127.0.0.1:1/graphql"
    rest_url     = "http://127.0.0.1:1/v2"
    max_retries  = 0
    timeouts = {
        create = "10s"
        read = "10s"
        update = "10s"
        delete = "10s"
    }
}

resource "buildkite_cluster" "test" {
    name        = "Test Cluster %s"
    description = "Test cluster for secrets"
}

resource "buildkite_cluster_secret" "test" {
    cluster_id  = buildkite_cluster.test.uuid
    key         = "%s"
    description = "Missing value"
}
`, clusterName, key)
}

func testAccClusterSecretConfigWithBothValues(clusterName, key, value, writeOnlyValue, version string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "test-org"
    api_token    = "test-token"
    graphql_url  = "http://127.0.0.1:1/graphql"
    rest_url     = "http://127.0.0.1:1/v2"
    max_retries  = 0
    timeouts = {
        create = "10s"
        read = "10s"
        update = "10s"
        delete = "10s"
    }
}

resource "buildkite_cluster" "test" {
    name        = "Test Cluster %s"
    description = "Test cluster for secrets"
}

resource "buildkite_cluster_secret" "test" {
    cluster_id       = buildkite_cluster.test.uuid
    key              = "%s"
    value            = "%s"
    value_wo         = "%s"
    value_wo_version = "%s"
    description      = "Both values"
}
`, clusterName, key, value, writeOnlyValue, version)
}

func testAccClusterSecretConfigWithoutWriteOnlyVersion(clusterName, key, value string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "test-org"
    api_token    = "test-token"
    graphql_url  = "http://127.0.0.1:1/graphql"
    rest_url     = "http://127.0.0.1:1/v2"
    max_retries  = 0
    timeouts = {
        create = "10s"
        read = "10s"
        update = "10s"
        delete = "10s"
    }
}

resource "buildkite_cluster" "test" {
    name        = "Test Cluster %s"
    description = "Test cluster for secrets"
}

resource "buildkite_cluster_secret" "test" {
    cluster_id  = buildkite_cluster.test.uuid
    key         = "%s"
    value_wo    = "%s"
    description = "Missing write-only version"
}
`, clusterName, key, value)
}

func testAccClusterSecretConfigWithEmptyWriteOnlyVersion(clusterName, key, value string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "test-org"
    api_token    = "test-token"
    graphql_url  = "http://127.0.0.1:1/graphql"
    rest_url     = "http://127.0.0.1:1/v2"
    max_retries  = 0
    timeouts = {
        create = "10s"
        read = "10s"
        update = "10s"
        delete = "10s"
    }
}

resource "buildkite_cluster" "test" {
    name        = "Test Cluster %s"
    description = "Test cluster for secrets"
}

resource "buildkite_cluster_secret" "test" {
    cluster_id       = buildkite_cluster.test.uuid
    key              = "%s"
    value_wo         = "%s"
    value_wo_version = ""
    description      = "Empty write-only version"
}
`, clusterName, key, value)
}

func testAccClusterSecretConfigWithPolicy(clusterName, key, value, pipeline, branch string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "%s"
    api_token    = "%s"
}

resource "buildkite_cluster" "test" {
    name        = "%s-cluster"
    description = "Test cluster for secrets"
}

resource "buildkite_cluster_secret" "test" {
    cluster_id  = buildkite_cluster.test.uuid
    key         = "%s"
    value       = "%s"
    description = "Secret with policy"
    
    policy = <<-EOT
- pipeline_slug: %s
  build_branch: %s
EOT
}
`, getenv("BUILDKITE_ORGANIZATION_SLUG"), os.Getenv("BUILDKITE_API_TOKEN"), clusterName, key, value, pipeline, branch)
}

func TestClusterSecretValue(t *testing.T) {
	t.Parallel()

	plan := clusterSecretResourceModel{
		Value: types.StringValue("stateful-value"),
	}
	config := clusterSecretResourceModel{}

	if got, err := clusterSecretValue(plan, config); err != nil || got != "stateful-value" {
		t.Fatalf("expected stateful value, got %q, err: %v", got, err)
	}

	config.ValueWO = types.StringValue("write-only-value")
	if got, err := clusterSecretValue(plan, config); err != nil || got != "write-only-value" {
		t.Fatalf("expected write-only value, got %q, err: %v", got, err)
	}

	config.ValueWO = types.StringNull()
	if got, err := clusterSecretValue(plan, config); err != nil || got != "stateful-value" {
		t.Fatalf("expected stateful value for null write-only value, got %q, err: %v", got, err)
	}

}

func TestClusterSecretValueErrorsWhenWriteOnlyValueUnavailable(t *testing.T) {
	t.Parallel()

	plan := clusterSecretResourceModel{
		Value:          types.StringNull(),
		ValueWOVersion: types.StringValue("version-1"),
	}
	config := clusterSecretResourceModel{
		ValueWO: types.StringNull(),
	}

	if got, err := clusterSecretValue(plan, config); err == nil {
		t.Fatalf("expected error, got value %q", got)
	}

	config.ValueWO = types.StringUnknown()
	if got, err := clusterSecretValue(plan, config); err == nil {
		t.Fatalf("expected error, got value %q", got)
	}
}

func TestShouldUpdateClusterSecretValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		plan  clusterSecretResourceModel
		state clusterSecretResourceModel
		want  bool
	}{
		{
			name:  "legacy value unchanged",
			plan:  clusterSecretResourceModel{Value: types.StringValue("same")},
			state: clusterSecretResourceModel{Value: types.StringValue("same")},
			want:  false,
		},
		{
			name:  "legacy value changed",
			plan:  clusterSecretResourceModel{Value: types.StringValue("new")},
			state: clusterSecretResourceModel{Value: types.StringValue("old")},
			want:  true,
		},
		{
			name:  "write-only version unchanged",
			plan:  clusterSecretResourceModel{ValueWOVersion: types.StringValue("same-version")},
			state: clusterSecretResourceModel{ValueWOVersion: types.StringValue("same-version")},
			want:  false,
		},
		{
			name:  "write-only version changed",
			plan:  clusterSecretResourceModel{ValueWOVersion: types.StringValue("new-version")},
			state: clusterSecretResourceModel{ValueWOVersion: types.StringValue("old-version")},
			want:  true,
		},
		{
			name: "write-only value change without version change",
			plan: clusterSecretResourceModel{
				ValueWOVersion: types.StringValue("same-version"),
			},
			state: clusterSecretResourceModel{
				ValueWOVersion: types.StringValue("same-version"),
			},
			want: false,
		},
		{
			name: "legacy value to write-only value",
			plan: clusterSecretResourceModel{
				Value:          types.StringNull(),
				ValueWOVersion: types.StringValue("version-1"),
			},
			state: clusterSecretResourceModel{
				Value: types.StringValue("legacy-value"),
			},
			want: true,
		},
		{
			name: "write-only value to legacy value",
			plan: clusterSecretResourceModel{
				Value: types.StringValue("legacy-value"),
			},
			state: clusterSecretResourceModel{
				Value:          types.StringNull(),
				ValueWOVersion: types.StringValue("version-1"),
			},
			want: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := shouldUpdateClusterSecretValue(tc.plan, tc.state); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

// Unit tests for reservedSecretKeyPrefixValidator — no API access required.

func TestReservedSecretKeyPrefixValidator(t *testing.T) {
	t.Parallel()

	v := reservedSecretKeyPrefixValidator{}

	tests := []struct {
		name        string
		input       string
		expectError bool
	}{
		// valid keys
		{name: "plain key", input: "MY_SECRET", expectError: false},
		{name: "api key", input: "API_KEY", expectError: false},
		{name: "buildkite mid-string", input: "APP_BUILDKITE_TOKEN", expectError: false},
		{name: "bk mid-string", input: "APP_BK_TOKEN", expectError: false},
		{name: "single letter", input: "X", expectError: false},

		// reserved prefix: buildkite variants
		{name: "BUILDKITE_ uppercase", input: "BUILDKITE_TOKEN", expectError: true},
		{name: "buildkite_ lowercase", input: "buildkite_token", expectError: true},
		{name: "Buildkite_ mixed case", input: "Buildkite_Token", expectError: true},
		{name: "BUILDKITE no underscore", input: "BUILDKITETOKEN", expectError: true},

		// reserved prefix: bk variants
		{name: "BK_ uppercase", input: "BK_SECRET", expectError: true},
		{name: "bk_ lowercase", input: "bk_secret", expectError: true},
		{name: "Bk_ mixed case", input: "Bk_Secret", expectError: true},
		{name: "BK no underscore", input: "BKSECRET", expectError: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := &validator.StringResponse{}
			v.ValidateString(context.Background(), validator.StringRequest{
				ConfigValue: types.StringValue(tc.input),
			}, resp)

			if got := resp.Diagnostics.HasError(); got != tc.expectError {
				if tc.expectError {
					t.Errorf("input %q: expected validation error but got none", tc.input)
				} else {
					t.Errorf("input %q: expected no error but got: %s", tc.input, resp.Diagnostics.Errors())
				}
			}
		})
	}
}

func TestReservedSecretKeyPrefixValidator_NullAndUnknown(t *testing.T) {
	t.Parallel()

	v := reservedSecretKeyPrefixValidator{}

	t.Run("null value is skipped", func(t *testing.T) {
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{
			ConfigValue: types.StringNull(),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("expected no error for null value, got: %s", resp.Diagnostics.Errors())
		}
	})

	t.Run("unknown value is skipped", func(t *testing.T) {
		resp := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{
			ConfigValue: types.StringUnknown(),
		}, resp)
		if resp.Diagnostics.HasError() {
			t.Errorf("expected no error for unknown value, got: %s", resp.Diagnostics.Errors())
		}
	})
}
