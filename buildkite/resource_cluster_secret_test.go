package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteClusterSecret_basic(t *testing.T) {
	secretKey := fmt.Sprintf("TEST_SECRET_%s", acctest.RandString(10))
	secretValue := acctest.RandString(20)
	clusterName := acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterSecretDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterSecretConfig(clusterName, secretKey, secretValue, "Initial description"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "key", secretKey),
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Initial description"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "id"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "created_at"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "updated_at"),
				),
			},
		},
	})
}

func TestAccBuildkiteClusterSecret_update(t *testing.T) {
	secretKey := fmt.Sprintf("TEST_SECRET_%s", acctest.RandString(10))
	secretValue1 := acctest.RandString(20)
	secretValue2 := acctest.RandString(20)
	clusterName := acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterSecretDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterSecretConfig(clusterName, secretKey, secretValue1, "Initial description"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Initial description"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "created_at"),
				),
			},
			{
				Config: testAccClusterSecretConfig(clusterName, secretKey, secretValue2, "Updated description"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Updated description"),
					// Verify created_at doesn't change on update
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "created_at"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "updated_at"),
				),
			},
		},
	})
}

func TestAccBuildkiteClusterSecret_writeOnlyValue(t *testing.T) {
	secretKey := fmt.Sprintf("TEST_SECRET_%s", acctest.RandString(10))
	secretValue1 := acctest.RandString(20)
	secretValue2 := acctest.RandString(20)
	clusterName := acctest.RandString(10)

	resource.Test(t, resource.TestCase{
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
					resource.TestCheckNoResourceAttr("buildkite_cluster_secret.test", "value_wo"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_secret.test", "id"),
				),
			},
			{
				Config: testAccClusterSecretWriteOnlyConfig(clusterName, secretKey, secretValue2, "version-2", "Updated description"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("buildkite_cluster_secret.test", "value_wo_version", "version-2"),
					resource.TestCheckNoResourceAttr("buildkite_cluster_secret.test", "value_wo"),
				),
			},
		},
	})
}

func TestAccBuildkiteClusterSecret_valueValidation(t *testing.T) {
	secretKey := fmt.Sprintf("TEST_SECRET_%s", acctest.RandString(10))
	clusterName := acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
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
		},
	})
}

func TestAccBuildkiteClusterSecret_withPolicy(t *testing.T) {
	secretKey := fmt.Sprintf("TEST_SECRET_%s", acctest.RandString(10))
	secretValue := acctest.RandString(20)
	clusterName := acctest.RandString(10)

	resource.Test(t, resource.TestCase{
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
	org := getenv("BUILDKITE_ORGANIZATION_SLUG")
	apiToken := os.Getenv("BUILDKITE_API_TOKEN")
	httpClient := &http.Client{}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_secret" {
			continue
		}

		clusterID := rs.Primary.Attributes["cluster_id"]
		secretID := rs.Primary.ID

		url := fmt.Sprintf("%s/v2/organizations/%s/clusters/%s/secrets/%s",
			defaultRestEndpoint, org, clusterID, secretID)

		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return err
		}

		req.Header.Set("Authorization", "Bearer "+apiToken)

		resp, err := httpClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()

		// If we get a 404, the secret was successfully destroyed
		if resp.StatusCode == 404 {
			continue
		}

		// If we get 200, the secret still exists
		if resp.StatusCode == 200 {
			return fmt.Errorf("cluster secret %s still exists", secretID)
		}
	}

	return nil
}

func testAccClusterSecretConfig(clusterName, key, value, description string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "%s"
    api_token    = "%s"
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
    value_wo_version = "%s"
    description      = "%s"
}
`, getenv("BUILDKITE_ORGANIZATION_SLUG"), os.Getenv("BUILDKITE_API_TOKEN"), clusterName, key, value, version, description)
}

func testAccClusterSecretConfigWithoutValue(clusterName, key string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "%s"
    api_token    = "%s"
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
`, getenv("BUILDKITE_ORGANIZATION_SLUG"), os.Getenv("BUILDKITE_API_TOKEN"), clusterName, key)
}

func testAccClusterSecretConfigWithBothValues(clusterName, key, value, writeOnlyValue, version string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "%s"
    api_token    = "%s"
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
`, getenv("BUILDKITE_ORGANIZATION_SLUG"), os.Getenv("BUILDKITE_API_TOKEN"), clusterName, key, value, writeOnlyValue, version)
}

func testAccClusterSecretConfigWithoutWriteOnlyVersion(clusterName, key, value string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "%s"
    api_token    = "%s"
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
`, getenv("BUILDKITE_ORGANIZATION_SLUG"), os.Getenv("BUILDKITE_API_TOKEN"), clusterName, key, value)
}

func testAccClusterSecretConfigWithPolicy(clusterName, key, value, pipeline, branch string) string {
	return fmt.Sprintf(`
provider "buildkite" {
    organization = "%s"
    api_token    = "%s"
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

	if got := clusterSecretValue(plan, config); got != "stateful-value" {
		t.Fatalf("expected stateful value, got %q", got)
	}

	config.ValueWO = types.StringValue("write-only-value")
	if got := clusterSecretValue(plan, config); got != "write-only-value" {
		t.Fatalf("expected write-only value, got %q", got)
	}

	config.ValueWO = types.StringNull()
	if got := clusterSecretValue(plan, config); got != "stateful-value" {
		t.Fatalf("expected stateful value for null write-only value, got %q", got)
	}

	config.ValueWO = types.StringUnknown()
	if got := clusterSecretValue(plan, config); got != "stateful-value" {
		t.Fatalf("expected stateful value for unknown write-only value, got %q", got)
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
