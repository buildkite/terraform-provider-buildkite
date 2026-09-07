package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestRfc3339Validator(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       types.String
		expectError bool
	}{
		{name: "utc", input: types.StringValue("2030-01-01T00:00:00Z"), expectError: false},
		{name: "offset", input: types.StringValue("2030-01-01T10:30:00+10:00"), expectError: false},
		{name: "fractional seconds", input: types.StringValue("2030-01-01T00:00:00.5Z"), expectError: false},
		{name: "milliseconds", input: types.StringValue("2030-01-01T00:00:00.123Z"), expectError: false},
		{name: "finer than milliseconds", input: types.StringValue("2030-01-01T00:00:00.123456Z"), expectError: true},
		{name: "null", input: types.StringNull(), expectError: false},
		{name: "unknown", input: types.StringUnknown(), expectError: false},
		{name: "date only", input: types.StringValue("2030-01-01"), expectError: true},
		{name: "no zone", input: types.StringValue("2030-01-01T00:00:00"), expectError: true},
		{name: "not a time", input: types.StringValue("tomorrow"), expectError: true},
		{name: "empty", input: types.StringValue(""), expectError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := &validator.StringResponse{}
			rfc3339Validator{}.ValidateString(context.Background(), validator.StringRequest{ConfigValue: tc.input}, resp)
			if resp.Diagnostics.HasError() != tc.expectError {
				t.Errorf("expected error %t, got %v", tc.expectError, resp.Diagnostics)
			}
		})
	}
}

func TestExpiresAtFromAPI(t *testing.T) {
	t.Parallel()

	remote := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		remote  *time.Time
		current types.String
		want    types.String
	}{
		{name: "no expiry", remote: nil, current: types.StringNull(), want: types.StringNull()},
		{name: "expiry removed remotely", remote: nil, current: types.StringValue("2030-01-01T00:00:00Z"), want: types.StringNull()},
		{name: "same instant keeps the configured format", remote: &remote, current: types.StringValue("2030-01-01T10:00:00+10:00"), want: types.StringValue("2030-01-01T10:00:00+10:00")},
		{name: "unset in state", remote: &remote, current: types.StringNull(), want: types.StringValue("2030-01-01T00:00:00Z")},
		{name: "different instant", remote: &remote, current: types.StringValue("2029-01-01T00:00:00Z"), want: types.StringValue("2030-01-01T00:00:00Z")},
	}
	// values the validator accepts must read back unchanged once the API has stored them
	for _, configured := range []string{"2030-01-01T00:00:00Z", "2030-01-01T00:00:00+00:00", "2030-01-01T10:00:00+10:00", "2030-01-01T00:00:00.5Z", "2030-01-01T00:00:00.123Z"} {
		stored, err := time.Parse(time.RFC3339, configured)
		if err != nil {
			t.Fatal(err)
		}
		stored = stored.UTC().Truncate(time.Millisecond)
		tests = append(tests, struct {
			name    string
			remote  *time.Time
			current types.String
			want    types.String
		}{"round trip " + configured, &stored, types.StringValue(configured), types.StringValue(configured)})
		// and an import (nothing configured) keeps the fraction
		tests = append(tests, struct {
			name    string
			remote  *time.Time
			current types.String
			want    types.String
		}{"import " + configured, &stored, types.StringNull(), types.StringValue(stored.Format(time.RFC3339Nano))})
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := expiresAtFromAPI(tc.remote, tc.current); !got.Equal(tc.want) {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestAccBuildkiteClusterAgentTokenResource(t *testing.T) {
	configBasic := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_test" {
			name = "Test cluster %s"
			description = "Acceptance testing cluster"
		}

		resource "buildkite_cluster_agent_token" "foobar" {
			cluster_id = buildkite_cluster.cluster_test.id
			description = "Acceptance Test %s"
		}

		`, fields[0], fields[1])
	}

	configAllowedIPsBasic := func(name, description string, allowed_ip_addresses []string) string {
		config := `

		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_test" {
			name = "Test cluster %s"
			description = "Acceptance testing cluster"
		}

		resource "buildkite_cluster_agent_token" "foobar" {
			cluster_id = buildkite_cluster.cluster_test.id
			description = "Acceptance Test %s"
			allowed_ip_addresses = %v
		}
		`

		marshalled_ips, _ := json.Marshal(allowed_ip_addresses)

		return fmt.Sprintf(config, name, description, string(marshalled_ips))
	}

	t.Run("creates a cluster agent token", func(t *testing.T) {
		var ct clusterAgentTokenResourceModel
		clusterName := acctest.RandString(10)
		tokenDesc := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
			// Confirm the token has the correct values in Buildkite's system
			testAccCheckClusterAgentTokenRemoteValues(&ct, fmt.Sprintf("Acceptance Test %s", tokenDesc)),
			// Confirm the token has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", tokenDesc)),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, tokenDesc),
					Check:  check,
				},
				{
					RefreshState: true,
					PlanOnly:     true,
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the token has the correct values in terraform state
						resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", tokenDesc)),
					),
				},
			},
		})
	})

	t.Run("creates a cluster agent token with an expiry", func(t *testing.T) {
		clusterName := acctest.RandString(10)
		tokenDesc := acctest.RandString(10)
		expiry := time.Now().UTC().AddDate(0, 0, 30).Truncate(time.Second)
		config := func(expiresAt time.Time) string {
			return configBasic(clusterName, tokenDesc) + fmt.Sprintf(`
			resource "buildkite_cluster_agent_token" "expiring" {
				cluster_id = buildkite_cluster.cluster_test.id
				description = "Acceptance Test expiring %s"
				expires_at = "%s"
			}
			`, tokenDesc, expiresAt.Format(time.RFC3339Nano))
		}
		// Confirm the token has the expiry in Buildkite's system
		checkRemoteExpiry := func(expiresAt time.Time) resource.TestCheckFunc {
			return func(s *terraform.State) error {
				var ct clusterAgentTokenResourceModel
				if err := testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.expiring", &ct)(s); err != nil {
					return err
				}
				if ct.ExpiresAt.ValueString() != expiresAt.Format(time.RFC3339Nano) {
					return fmt.Errorf("Remote Cluster agent token expiry (%s) doesn't match expected value (%s)", ct.ExpiresAt.ValueString(), expiresAt.Format(time.RFC3339Nano))
				}
				return nil
			}
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(expiry),
					Check: resource.ComposeAggregateTestCheckFunc(
						checkRemoteExpiry(expiry),
						resource.TestCheckResourceAttr("buildkite_cluster_agent_token.expiring", "expires_at", expiry.Format(time.RFC3339Nano)),
						resource.TestCheckNoResourceAttr("buildkite_cluster_agent_token.foobar", "expires_at"),
					),
				},
				{
					// the expiry can't be updated, so changing it replaces the token
					Config: config(expiry.AddDate(0, 0, 30)),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_cluster_agent_token.expiring", plancheck.ResourceActionReplace),
						},
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
					Check: checkRemoteExpiry(expiry.AddDate(0, 0, 30)),
				},
				{
					// the API keeps milliseconds, so a fractional expiry must read back unchanged
					Config: config(expiry.Add(123 * time.Millisecond)),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectEmptyPlan(),
						},
					},
					Check: checkRemoteExpiry(expiry.Add(123 * time.Millisecond)),
				},
			},
		})
	})

	t.Run("creates a cluster agent token with allowed IPs", func(t *testing.T) {
		var ct clusterAgentTokenResourceModel
		clusterName := acctest.RandString(10)
		tokenDesc := acctest.RandString(10)
		allowedIps := []string{"10.100.1.0/28"}

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
			// Confirm the token has the correct values in Buildkite's system
			testAccCheckClusterAgentTokenRemoteValues(&ct, fmt.Sprintf("Acceptance Test %s", tokenDesc)),
			// Confirm the token has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", tokenDesc)),
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "allowed_ip_addresses.#", "1"),
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "allowed_ip_addresses.0", "10.100.1.0/28"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
			Steps: []resource.TestStep{
				{
					Config: configAllowedIPsBasic(clusterName, tokenDesc, allowedIps),
					Check:  check,
				},
			},
		})
	})

	t.Run("updates a cluster agent token", func(t *testing.T) {
		var ct clusterAgentTokenResourceModel
		clusterName := acctest.RandString(10)
		tokenDesc := acctest.RandString(10)
		updatedTokenDesc := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
			// Confirm the token has the correct values in Buildkite's system
			testAccCheckClusterAgentTokenRemoteValues(&ct, fmt.Sprintf("Acceptance Test %s", tokenDesc)),
			// Confirm the token has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", tokenDesc)),
		)

		ckecUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm the token exists in the buildkite API
			testAccCheckClusterAgentTokenExists("buildkite_cluster_agent_token.foobar", &ct),
			// Confirm the token has the correct values in Buildkite's system
			testAccCheckClusterAgentTokenRemoteValues(&ct, fmt.Sprintf("Acceptance Test %s", updatedTokenDesc)),
			// Confirm the token has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_agent_token.foobar", "description", fmt.Sprintf("Acceptance Test %s", updatedTokenDesc)),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, tokenDesc),
					Check:  check,
				},
				{
					Config: configBasic(clusterName, updatedTokenDesc),
					Check:  ckecUpdated,
				},
			},
		})
	})
}

func testAccCheckClusterAgentTokenExists(resourceName string, ct *clusterAgentTokenResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}
		clusterTokens, err := getClusterAgentTokens(
			context.Background(),
			genqlientGraphql,
			getenv("BUILDKITE_ORGANIZATION_SLUG"),
			resourceState.Primary.Attributes["cluster_uuid"],
		)
		if err != nil {
			return fmt.Errorf("Error fetching Cluster Agent Tokens from graphql API: %v", err)
		}

		// Obtain the ClusterAgentTokenResourceModel
		for _, edge := range clusterTokens.Organization.Cluster.AgentTokens.Edges {
			if edge.Node.Id == resourceState.Primary.ID {
				ct.Id = types.StringValue(edge.Node.Id)
				ct.Uuid = types.StringValue(edge.Node.Uuid)
				ct.Description = types.StringValue(edge.Node.Description)
				ct.ExpiresAt = expiresAtFromAPI(edge.Node.ExpiresAt, types.StringNull())
				break
			}
		}

		// If ClusterAgentTokenResourceModel isnt set from the token slice
		if ct.Id.ValueString() == "" {
			return fmt.Errorf("No Cluster agent token found with graphql id: %s", resourceState.Primary.ID)
		}

		return nil
	}
}

func testAccCheckClusterAgentTokenRemoteValues(ct *clusterAgentTokenResourceModel, description string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if ct.Description.ValueString() != description {
			return fmt.Errorf("Remote Cluster agent token description (%s) doesn't match expected value (%s)", ct.Description, description)
		}

		return nil
	}
}

func testAccCheckClusterAgentTokenDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_agent_token" {
			continue
		}
	}
	return nil
}
