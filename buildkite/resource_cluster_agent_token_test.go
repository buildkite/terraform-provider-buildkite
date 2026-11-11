package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

func TestAccNonAdminClusterAgentToken(t *testing.T) {
	const address = "buildkite_cluster_agent_token.temporary"
	expiry := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	description := "Token acceptance test " + acctest.RandString(12)
	baseConfig := fmt.Sprintf(`
		data "buildkite_cluster" "fixture" {
			name = %q
		}
	`, testAccFixtureClusterName)
	config := func(description string, ips []string) string {
		allowedIPs, _ := json.Marshal(ips)
		return baseConfig + fmt.Sprintf(`
			resource "buildkite_cluster_agent_token" "temporary" {
				cluster_id = data.buildkite_cluster.fixture.id
				description = %q
				allowed_ip_addresses = %s
				expires_at = %q
			}
		`, description, allowedIPs, expiry.Format(time.RFC3339))
	}
	var tokenID, tokenUUID, tokenValue string
	check := func(description string, ips []string) resource.TestCheckFunc {
		checks := []resource.TestCheckFunc{
			resource.TestCheckResourceAttr("data.buildkite_cluster.fixture", "uuid", testAccFixtureClusterUUID),
			resource.TestCheckResourceAttrPair(address, "cluster_id", "data.buildkite_cluster.fixture", "id"),
			resource.TestCheckResourceAttr(address, "cluster_uuid", testAccFixtureClusterUUID),
			resource.TestCheckResourceAttr(address, "description", description),
			resource.TestCheckResourceAttr(address, "expires_at", expiry.Format(time.RFC3339)),
			resource.TestCheckResourceAttr(address, "allowed_ip_addresses.#", fmt.Sprint(len(ips))),
			resource.TestCheckResourceAttrSet(address, "uuid"),
			resource.TestCheckResourceAttrSet(address, "token"),
			func(s *terraform.State) error {
				rs, ok := s.RootModule().Resources[address]
				if !ok || rs.Primary == nil {
					return fmt.Errorf("temporary token is missing from Terraform state")
				}
				state := rs.Primary
				if tokenID == "" {
					tokenID, tokenUUID, tokenValue = state.ID, state.Attributes["uuid"], state.Attributes["token"]
				}
				if state.ID == "" || state.ID != tokenID || state.Attributes["uuid"] != tokenUUID || state.Attributes["token"] != tokenValue {
					return fmt.Errorf("temporary token identity or secret changed")
				}
				r, err := getClusterAgentTokens(context.Background(), genqlientGraphql, getenv("BUILDKITE_ORGANIZATION_SLUG"), testAccFixtureClusterUUID)
				if err != nil {
					return err
				}
				for _, edge := range r.Organization.Cluster.AgentTokens.Edges {
					node := edge.Node
					if node.Id != tokenID {
						continue
					}
					if node.Uuid != tokenUUID || node.Cluster.Uuid != testAccFixtureClusterUUID || node.Cluster.Id != state.Attributes["cluster_id"] || node.Description != description || node.AllowedIpAddresses != strings.Join(ips, " ") || node.ExpiresAt == nil || !node.ExpiresAt.Equal(expiry) {
						return fmt.Errorf("remote token does not match the expected identity, cluster, description, IP restrictions, and expiry")
					}
					return nil
				}
				return fmt.Errorf("temporary token not found remotely")
			},
		}
		for i, ip := range ips {
			checks = append(checks, resource.TestCheckResourceAttr(address, fmt.Sprintf("allowed_ip_addresses.%d", i), ip))
		}
		return resource.ComposeAggregateTestCheckFunc(checks...)
	}
	ips := []string{"192.0.2.0/28"}
	updatedIPs := []string{"198.51.100.0/25", "203.0.113.16/28"}
	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccNonAdminPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterAgentTokenDestroy,
		Steps: []resource.TestStep{
			{Config: config(description, ips), Check: check(description, ips)},
			{
				Config: config(description+" updated", updatedIPs),
				Check:  check(description+" updated", updatedIPs),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionUpdate)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
			{RefreshState: true, Check: check(description+" updated", updatedIPs)},
			{
				Config: baseConfig,
				Check: func(s *terraform.State) error {
					if _, exists := s.RootModule().Resources[address]; exists {
						return fmt.Errorf("revoked token remains in Terraform state")
					}
					r, err := getNode(context.Background(), genqlientGraphql, tokenID)
					if err != nil {
						return err
					}
					if r.Node != nil {
						return fmt.Errorf("revoked token still exists remotely")
					}
					return resource.TestCheckResourceAttr("data.buildkite_cluster.fixture", "uuid", testAccFixtureClusterUUID)(s)
				},
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectResourceAction(address, plancheck.ResourceActionDestroy)},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestAccBuildkiteClusterAgentTokenResource(t *testing.T) {
	t.Parallel()
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

func testAccCheckClusterAgentTokenDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_agent_token" {
			continue
		}
		r, err := getNode(context.Background(), genqlientGraphql, rs.Primary.ID)
		if err != nil {
			return err
		}
		if r.Node != nil {
			return fmt.Errorf("Cluster agent token still exists: %s", rs.Primary.ID)
		}
	}
	return nil
}
