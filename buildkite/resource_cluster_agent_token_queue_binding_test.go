package buildkite

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	scopedTokenTestQueueAID              = "Q2x1c3RlclF1ZXVlLS0tcXVldWUtYQ=="
	scopedTokenTestQueueBID              = "Q2x1c3RlclF1ZXVlLS0tcXVldWUtYg=="
	scopedTokenTestQueueCID              = "Q2x1c3RlclF1ZXVlLS0tcXVldWUtYw=="
	scopedTokenTestForeignClusterQueueID = "Q2x1c3RlclF1ZXVlLS0tb3RoZXItY2x1c3Rlcg=="
	scopedTokenTestForeignOrgQueueID     = "Q2x1c3RlclF1ZXVlLS0tZm9yZWlnbi1vcmc="
	scopedTokenTestWrongTypeQueueID      = "Q2x1c3Rlci0tLW5vdC1hLXF1ZXVl"
)

func scopedTokenQueueIDsJSON(ids ...string) string {
	value, _ := json.Marshal(append([]string{}, ids...))
	return string(value)
}

func scopedTokenQueueAttributes(allowAllQueues bool, ids ...string) string {
	return fmt.Sprintf("allow_all_queues = %t\nallowed_queue_ids = %s", allowAllQueues, scopedTokenQueueIDsJSON(ids...))
}

func scopedTokenQueueErrorPattern(message string) *regexp.Regexp {
	return regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(message), " ", `\s+`))
}

func (a *fakeClusterAgentTokenAPI) setQueueAccess(allowAllQueues bool, ids ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.allowAllQueues = allowAllQueues
	a.allowedQueueIDs = append([]string{}, ids...)
}

func (a *fakeClusterAgentTokenAPI) deleteQueue(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.queues, id)
}

func (a *fakeClusterAgentTokenAPI) checkLastQueueWrite(operation, mode, ids string, count int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if len(a.writes) != count || count == 0 {
			return fmt.Errorf("token writes = %+v, want %d writes", a.writes, count)
		}
		write := a.writes[count-1]
		if write.operation != operation || write.allowAllQueues != mode {
			return fmt.Errorf("last token mutation = %s with allowAllQueues=%q, want %s with allowAllQueues=%q (empty string means omitted)", write.operation, write.allowAllQueues, operation, mode)
		}
		if err := compareScopedTokenPermissionsJSON(write.allowedQueueIDs, ids); err != nil {
			return fmt.Errorf("allowedQueueIds: %w", err)
		}
		return nil
	}
}

func checkScopedTokenQueues(allowAllQueues bool, ids ...string) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(scopedTokenTestAddress, "allow_all_queues", fmt.Sprint(allowAllQueues)),
		resource.TestCheckResourceAttr(scopedTokenTestAddress, "allowed_queue_ids.#", fmt.Sprint(len(ids))),
	}
	for _, id := range ids {
		checks = append(checks, resource.TestCheckTypeSetElemAttr(scopedTokenTestAddress, "allowed_queue_ids.*", id))
	}
	return resource.ComposeAggregateTestCheckFunc(checks...)
}

func TestClusterAgentTokenQueueCreateAgainstFakeAPI(t *testing.T) {
	for _, tc := range []struct {
		name, attributes, mode, ids string
		allowAllQueues              bool
		queueIDs                    []string
	}{
		{name: "omitted uses API defaults", allowAllQueues: true},
		{name: "null is omission", attributes: "allow_all_queues = null\nallowed_queue_ids = null", allowAllQueues: true},
		{name: "explicit unrestricted", attributes: scopedTokenQueueAttributes(true), mode: "true", ids: "[]", allowAllQueues: true},
		{name: "selected queues", attributes: scopedTokenQueueAttributes(false, scopedTokenTestQueueAID, scopedTokenTestQueueBID), mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueBID), queueIDs: []string{scopedTokenTestQueueAID, scopedTokenTestQueueBID}},
		{name: "deny every queue", attributes: scopedTokenQueueAttributes(false), mode: "false", ids: "[]"},
		{name: "empty selection does not restrict default mode", attributes: "allowed_queue_ids = []", ids: "[]", allowAllQueues: true},
		{name: "duplicates are a set", attributes: scopedTokenQueueAttributes(false, scopedTokenTestQueueBID, scopedTokenTestQueueAID, scopedTokenTestQueueBID), mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueBID), queueIDs: []string{scopedTokenTestQueueAID, scopedTokenTestQueueBID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, api := newFakeClusterAgentTokenAPI(t)
			config := scopedTokenTestConfig(server, "test", "permissions = [\"read_metrics\"]\n"+tc.attributes)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkDestroyed(1),
				Steps: []resource.TestStep{
					{
						Config:           config,
						ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
						Check: resource.ComposeAggregateTestCheckFunc(
							checkScopedTokenIdentity(api),
							checkScopedTokenPermissions("read_metrics"),
							checkScopedTokenQueues(tc.allowAllQueues, tc.queueIDs...),
							api.checkLastWrite("createClusterAgentToken", `["read_metrics"]`, 1),
							api.checkLastQueueWrite("createClusterAgentToken", tc.mode, tc.ids, 1),
						),
					},
					{Config: config, PlanOnly: true},
				},
			})
		})
	}
}

func TestClusterAgentTokenQueueInPlaceTransitionsAndIndependentOmissionsAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	check := func(mode, ids string, writes int, allowAllQueues bool, queueIDs ...string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			checkScopedTokenIdentity(api),
			checkScopedTokenPermissions("read_metrics"),
			checkScopedTokenQueues(allowAllQueues, queueIDs...),
			api.checkLastWrite("updateClusterAgentToken", "", writes),
			api.checkLastQueueWrite("updateClusterAgentToken", mode, ids, writes),
		)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{
				Config: scopedTokenTestConfig(server, "test", "permissions = [\"read_metrics\"]\n"+scopedTokenQueueAttributes(true)),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenQueues(true),
					api.checkLastQueueWrite("createClusterAgentToken", "true", "[]", 1),
				),
			},
			{
				Config:           scopedTokenTestConfig(server, "test", scopedTokenQueueAttributes(false, scopedTokenTestQueueAID, scopedTokenTestQueueBID)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueBID), 2, false, scopedTokenTestQueueAID, scopedTokenTestQueueBID),
			},
			{
				Config: scopedTokenTestConfig(server, "test", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: check("false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueBID), 2, false, scopedTokenTestQueueAID, scopedTokenTestQueueBID),
			},
			{
				Config:           scopedTokenTestConfig(server, "unrelated update", ""),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("", "", 3, false, scopedTokenTestQueueAID, scopedTokenTestQueueBID),
			},
			{
				Config:           scopedTokenTestConfig(server, "mode only", "allow_all_queues = false"),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("false", "", 4, false, scopedTokenTestQueueAID, scopedTokenTestQueueBID),
			},
			{
				Config:           scopedTokenTestConfig(server, "list only", "allowed_queue_ids = "+scopedTokenQueueIDsJSON(scopedTokenTestQueueAID)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), 5, false, scopedTokenTestQueueAID),
			},
			{
				Config:           scopedTokenTestConfig(server, "null mode", "allow_all_queues = null\nallowed_queue_ids = "+scopedTokenQueueIDsJSON(scopedTokenTestQueueBID)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("", scopedTokenQueueIDsJSON(scopedTokenTestQueueBID), 6, false, scopedTokenTestQueueBID),
			},
			{
				Config:           scopedTokenTestConfig(server, "null list", "allow_all_queues = false\nallowed_queue_ids = null"),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("false", "", 7, false, scopedTokenTestQueueBID),
			},
			{
				Config:           scopedTokenTestConfig(server, "deny all with list only", "allowed_queue_ids = []"),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("", "[]", 8, false),
			},
			{
				Config:           scopedTokenTestConfig(server, "empty to unrestricted with mode only", "allow_all_queues = true"),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("true", "", 9, true),
			},
			{
				Config:           scopedTokenTestConfig(server, "unrestricted to empty", scopedTokenQueueAttributes(false)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("false", "[]", 10, false),
			},
			{
				Config:           scopedTokenTestConfig(server, "empty to selected", scopedTokenQueueAttributes(false, scopedTokenTestQueueBID)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("false", scopedTokenQueueIDsJSON(scopedTokenTestQueueBID), 11, false, scopedTokenTestQueueBID),
			},
			{
				Config:           scopedTokenTestConfig(server, "selected to unrestricted", scopedTokenQueueAttributes(true)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("true", "[]", 12, true),
			},
		},
	})
}

func TestClusterAgentTokenQueueReorderingIsStableAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	check := resource.ComposeAggregateTestCheckFunc(
		checkScopedTokenIdentity(api),
		checkScopedTokenQueues(false, scopedTokenTestQueueAID, scopedTokenTestQueueBID),
		api.checkLastQueueWrite("createClusterAgentToken", "false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueBID), 1),
	)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{
				Config: scopedTokenTestConfig(server, "test", scopedTokenQueueAttributes(false, scopedTokenTestQueueBID, scopedTokenTestQueueAID, scopedTokenTestQueueBID)),
				Check:  check,
			},
			{
				Config: scopedTokenTestConfig(server, "test", scopedTokenQueueAttributes(false, scopedTokenTestQueueAID, scopedTokenTestQueueBID)),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: check,
			},
			{RefreshState: true, Check: check},
		},
	})
}

func TestClusterAgentTokenQueueDriftAgainstFakeAPI(t *testing.T) {
	for _, tc := range []struct {
		name     string
		queueIDs []string
		drift    func(*fakeClusterAgentTokenAPI)
	}{
		{name: "mode only", drift: func(api *fakeClusterAgentTokenAPI) { api.setQueueAccess(true) }},
		{name: "selection only", queueIDs: []string{scopedTokenTestQueueAID, scopedTokenTestQueueBID}, drift: func(api *fakeClusterAgentTokenAPI) { api.setQueueAccess(false, scopedTokenTestQueueBID) }},
		{name: "mode and selection", queueIDs: []string{scopedTokenTestQueueAID}, drift: func(api *fakeClusterAgentTokenAPI) { api.setQueueAccess(true) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, api := newFakeClusterAgentTokenAPI(t)
			config := scopedTokenTestConfig(server, "test", scopedTokenQueueAttributes(false, tc.queueIDs...))
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkDestroyed(1),
				Steps: []resource.TestStep{
					{Config: config, Check: checkScopedTokenQueues(false, tc.queueIDs...)},
					{
						PreConfig:        func() { tc.drift(api) },
						Config:           config,
						ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
						Check: resource.ComposeAggregateTestCheckFunc(
							checkScopedTokenIdentity(api),
							checkScopedTokenPermissions("run_jobs", "read_build_metadata", "read_metrics"),
							checkScopedTokenQueues(false, tc.queueIDs...),
							api.checkLastWrite("updateClusterAgentToken", "", 2),
							api.checkLastQueueWrite("updateClusterAgentToken", "false", scopedTokenQueueIDsJSON(tc.queueIDs...), 2),
						),
					},
					{Config: config, PlanOnly: true},
				},
			})
		})
	}
}

func TestClusterAgentTokenQueueOmittedStateAdoptionAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	config := scopedTokenTestConfig(server, "test", "")
	check := func(allowAllQueues bool, ids ...string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			checkScopedTokenIdentity(api),
			checkScopedTokenQueues(allowAllQueues, ids...),
			api.checkLastQueueWrite("createClusterAgentToken", "", "", 1),
		)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{Config: config, Check: check(true)},
			{
				PreConfig:    func() { api.setQueueAccess(false, scopedTokenTestQueueAID, scopedTokenTestQueueBID) },
				RefreshState: true,
				Check:        check(false, scopedTokenTestQueueAID, scopedTokenTestQueueBID),
			},
			{Config: config, PlanOnly: true},
			{
				PreConfig:    func() { api.setQueueAccess(false) },
				RefreshState: true,
				Check:        check(false),
			},
			{Config: config, PlanOnly: true},
			{
				PreConfig:    func() { api.setQueueAccess(true) },
				RefreshState: true,
				Check:        check(true),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestClusterAgentTokenQueueComputedReferencesAgainstFakeAPI(t *testing.T) {
	for _, wholeSet := range []bool{false, true} {
		name := "unknown element"
		if wholeSet {
			name = "unknown set"
		}
		t.Run(name, func(t *testing.T) {
			server, api := newFakeClusterAgentTokenAPI(t)
			config := func(id string) string {
				ids := "[terraform_data.binding.output.id]"
				if wholeSet {
					ids = "terraform_data.binding.output.ids"
				}
				return scopedTokenTestConfig(server, "test", "permissions = [\"read_metrics\"]\nallow_all_queues = terraform_data.binding.output.mode\nallowed_queue_ids = "+ids) + fmt.Sprintf(`
resource "terraform_data" "binding" {
  input = {
    mode = false
    id = %q
    ids = [%q]
  }
}
`, id, id)
			}
			check := func(operation, id string, writes int) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenPermissions("read_metrics"),
					checkScopedTokenQueues(false, id),
					api.checkLastQueueWrite(operation, "false", scopedTokenQueueIDsJSON(id), writes),
				)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkDestroyed(1),
				Steps: []resource.TestStep{
					{
						Config:           config(scopedTokenTestQueueAID),
						ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
						Check:            check("createClusterAgentToken", scopedTokenTestQueueAID, 1),
					},
					{
						Config:           config(scopedTokenTestQueueBID),
						ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
						Check:            check("updateClusterAgentToken", scopedTokenTestQueueBID, 2),
					},
				},
			})
		})
	}
}

func TestClusterAgentTokenQueueInvalidCreatesAgainstFakeAPI(t *testing.T) {
	for _, tc := range []struct {
		name, attributes, mode, ids, message string
	}{
		{name: "restricted requires explicit selection", attributes: "allow_all_queues = false", mode: "false", message: "Creating a restricted token requires an explicit allowedQueueIds array"},
		{name: "explicit unrestricted conflicts with selection", attributes: scopedTokenQueueAttributes(true, scopedTokenTestQueueAID), mode: "true", ids: scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), message: "All-queues access cannot be combined with queue bindings"},
		{name: "default unrestricted conflicts with selection", attributes: "allowed_queue_ids = " + scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), ids: scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), message: "All-queues access cannot be combined with queue bindings"},
		{name: "unknown queue", attributes: scopedTokenQueueAttributes(false, "Q2x1c3RlclF1ZXVlLS0tdW5rbm93bg=="), mode: "false", ids: scopedTokenQueueIDsJSON("Q2x1c3RlclF1ZXVlLS0tdW5rbm93bg=="), message: "Queue ID does not identify an available ClusterQueue"},
		{name: "wrong node type", attributes: scopedTokenQueueAttributes(false, scopedTokenTestWrongTypeQueueID), mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestWrongTypeQueueID), message: "Queue ID must identify a ClusterQueue, not a Cluster"},
		{name: "foreign cluster", attributes: scopedTokenQueueAttributes(false, scopedTokenTestForeignClusterQueueID), mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestForeignClusterQueueID), message: "Queue belongs to another cluster"},
		{name: "foreign organization", attributes: scopedTokenQueueAttributes(false, scopedTokenTestForeignOrgQueueID), mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestForeignOrgQueueID), message: "Queue belongs to another organization"},
		{name: "UUID is not a GraphQL ID", attributes: scopedTokenQueueAttributes(false, "81444444-1111-4222-8333-444455556666"), mode: "false", ids: scopedTokenQueueIDsJSON("81444444-1111-4222-8333-444455556666"), message: "Queue ID does not identify an available ClusterQueue"},
		{name: "key is not a GraphQL ID", attributes: scopedTokenQueueAttributes(false, "default"), mode: "false", ids: scopedTokenQueueIDsJSON("default"), message: "Queue ID does not identify an available ClusterQueue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, api := newFakeClusterAgentTokenAPI(t)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkDestroyed(0),
				Steps: []resource.TestStep{{
					Config:      scopedTokenTestConfig(server, "test", tc.attributes),
					ExpectError: scopedTokenQueueErrorPattern(tc.message),
				}},
			})
			if err := api.checkLastQueueWrite("createClusterAgentToken", tc.mode, tc.ids, 1)(nil); err != nil {
				t.Fatal(err)
			}
			if err := api.checkDestroyed(0)(nil); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestClusterAgentTokenQueueInvalidUpdatesAreAtomicAgainstFakeAPI(t *testing.T) {
	for _, tc := range []struct {
		name, attributes, permissions, mode, ids, message string
	}{
		{name: "invalid queue cannot change permissions or description", attributes: scopedTokenQueueAttributes(false, scopedTokenTestForeignClusterQueueID), permissions: `["read_secrets"]`, mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestForeignClusterQueueID), message: "Queue belongs to another cluster"},
		{name: "invalid permission cannot change queue mode or selection", attributes: scopedTokenQueueAttributes(true), permissions: `["unknown_permission"]`, mode: "true", ids: "[]", message: "Unknown agent token permission from API: unknown_permission"},
		{name: "unrestricted must explicitly clear bound queues", attributes: "allow_all_queues = true", permissions: `["read_secrets"]`, mode: "true", message: "All-queues access cannot be combined with queue bindings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, api := newFakeClusterAgentTokenAPI(t)
			config := scopedTokenTestConfig(server, "original", "permissions = [\"read_metrics\"]\n"+scopedTokenQueueAttributes(false, scopedTokenTestQueueAID))
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkDestroyed(1),
				Steps: []resource.TestStep{
					{Config: config, Check: checkScopedTokenIdentity(api)},
					{
						Config:      scopedTokenTestConfig(server, "must not be stored", "permissions = "+tc.permissions+"\n"+tc.attributes),
						ExpectError: scopedTokenQueueErrorPattern(tc.message),
					},
					{
						Config: config,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply:             []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
							PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						},
						Check: resource.ComposeAggregateTestCheckFunc(
							checkScopedTokenIdentity(api),
							resource.TestCheckResourceAttr(scopedTokenTestAddress, "description", "original"),
							checkScopedTokenPermissions("read_metrics"),
							checkScopedTokenQueues(false, scopedTokenTestQueueAID),
							api.checkLastWrite("updateClusterAgentToken", tc.permissions, 2),
							api.checkLastQueueWrite("updateClusterAgentToken", tc.mode, tc.ids, 2),
						),
					},
				},
			})
		})
	}
}

func TestClusterAgentTokenQueueRefreshAndImportAgainstFakeAPI(t *testing.T) {
	for _, tc := range []struct {
		name           string
		allowAllQueues bool
		queueIDs       []string
		missingQueue   bool
	}{
		{name: "selected", queueIDs: []string{scopedTokenTestQueueAID, scopedTokenTestQueueBID}},
		{name: "deny every queue"},
		{name: "unrestricted", allowAllQueues: true},
		{name: "deleted bound queue is retained", queueIDs: []string{scopedTokenTestQueueAID}, missingQueue: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, api := newFakeClusterAgentTokenAPI(t)
			attributes := "permissions = [\"read_metrics\"]\n" + scopedTokenQueueAttributes(tc.allowAllQueues, tc.queueIDs...)
			config := scopedTokenTestConfig(server, "test", attributes)
			check := resource.ComposeAggregateTestCheckFunc(
				checkScopedTokenIdentity(api),
				checkScopedTokenPermissions("read_metrics"),
				checkScopedTokenQueues(tc.allowAllQueues, tc.queueIDs...),
				api.checkLastQueueWrite("createClusterAgentToken", fmt.Sprint(tc.allowAllQueues), scopedTokenQueueIDsJSON(tc.queueIDs...), 1),
			)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkDestroyed(1),
				Steps: []resource.TestStep{
					{Config: config, Check: check},
					{
						PreConfig: func() {
							if tc.missingQueue {
								api.deleteQueue(scopedTokenTestQueueAID)
							}
						},
						RefreshState: true,
						Check:        check,
					},
					{
						ResourceName: scopedTokenTestAddress,
						ImportState:  true, ImportStateId: scopedTokenTestID, ImportStateVerify: true,
						ImportStateVerifyIgnore: []string{"token"},
						ImportStateCheck: func(states []*terraform.InstanceState) error {
							if len(states) != 1 || states[0].ID != scopedTokenTestID || states[0].Attributes["token"] != "" || states[0].Attributes["allow_all_queues"] != fmt.Sprint(tc.allowAllQueues) || states[0].Attributes["allowed_queue_ids.#"] != fmt.Sprint(len(tc.queueIDs)) {
								return fmt.Errorf("import must recover queue metadata without recovering the secret: %+v", states)
							}
							return nil
						},
					},
					{Config: config, PlanOnly: true},
				},
			})
		})
	}
}

func TestClusterAgentTokenQueueMissingBindingsArePreservedButNotAddedAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	config := scopedTokenTestConfig(server, "test", scopedTokenQueueAttributes(false, scopedTokenTestQueueAID))
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{Config: config, Check: checkScopedTokenQueues(false, scopedTokenTestQueueAID)},
			{
				PreConfig: func() {
					api.deleteQueue(scopedTokenTestQueueAID)
					api.deleteQueue(scopedTokenTestQueueCID)
				},
				Config:           scopedTokenTestConfig(server, "retains explicitly managed missing binding", scopedTokenQueueAttributes(false, scopedTokenTestQueueAID)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenQueues(false, scopedTokenTestQueueAID),
					api.checkLastQueueWrite("updateClusterAgentToken", "false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), 2),
				),
			},
			{
				Config:           scopedTokenTestConfig(server, "retains omitted missing binding", ""),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenQueues(false, scopedTokenTestQueueAID),
					api.checkLastQueueWrite("updateClusterAgentToken", "", "", 3),
				),
			},
			{
				Config:      scopedTokenTestConfig(server, "must not add missing queue", scopedTokenQueueAttributes(false, scopedTokenTestQueueAID, scopedTokenTestQueueCID)),
				ExpectError: scopedTokenQueueErrorPattern("Queue ID does not identify an available ClusterQueue"),
			},
			{
				Config:           scopedTokenTestConfig(server, "retains omitted missing binding", ""),
				ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenQueues(false, scopedTokenTestQueueAID),
					api.checkLastQueueWrite("updateClusterAgentToken", "false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueCID), 4),
				),
			},
		},
	})
}

func TestClusterAgentTokenQueueFeatureDisabledLegacyMetadataAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	api.setPermissionsDisabled(true)
	check := func(operation string, writes int) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			checkScopedTokenIdentity(api),
			resource.TestCheckNoResourceAttr(scopedTokenTestAddress, "allow_all_queues"),
			resource.TestCheckNoResourceAttr(scopedTokenTestAddress, "allowed_queue_ids.#"),
			api.checkLastQueueWrite(operation, "", "", writes),
		)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{
				Config:           scopedTokenTestConfig(server, "legacy", ""),
				ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
				Check:            check("createClusterAgentToken", 1),
			},
			{
				Config:           scopedTokenTestConfig(server, "legacy update", "allow_all_queues = null\nallowed_queue_ids = null"),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("updateClusterAgentToken", 2),
			},
			{RefreshState: true, Check: check("updateClusterAgentToken", 2)},
			{
				ResourceName: scopedTokenTestAddress,
				ImportState:  true, ImportStateId: scopedTokenTestID, ImportStateVerify: true,
				ImportStateVerifyIgnore: []string{"token"},
			},
		},
	})
}

func TestClusterAgentTokenQueueFeatureDisabledRetainsKnownMetadataAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	check := func(operation, mode, ids string, writes int) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			checkScopedTokenIdentity(api),
			checkScopedTokenPermissions("read_metrics"),
			checkScopedTokenQueues(false, scopedTokenTestQueueAID),
			api.checkLastQueueWrite(operation, mode, ids, writes),
		)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{
				Config: scopedTokenTestConfig(server, "test", "permissions = [\"read_metrics\"]\n"+scopedTokenQueueAttributes(false, scopedTokenTestQueueAID)),
				Check:  check("createClusterAgentToken", "false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), 1),
			},
			{
				PreConfig:    func() { api.setPermissionsDisabled(true) },
				RefreshState: true,
				Check:        check("createClusterAgentToken", "false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), 1),
			},
			{
				Config:           scopedTokenTestConfig(server, "unrelated update", ""),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("updateClusterAgentToken", "", "", 2),
			},
			{
				Config:      scopedTokenTestConfig(server, "rejected explicit write", "allow_all_queues = false"),
				ExpectError: scopedTokenQueueErrorPattern("Agent token queue bindings are not enabled"),
			},
			{
				Config:           scopedTokenTestConfig(server, "unrelated update", ""),
				ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
				Check:            check("updateClusterAgentToken", "false", "", 3),
			},
		},
	})
}

func TestClusterAgentTokenQueueFeatureDisabledWritesRejectedAgainstFakeAPI(t *testing.T) {
	for _, tc := range []struct {
		name, attributes, mode, ids string
	}{
		{name: "explicit false", attributes: "allow_all_queues = false", mode: "false"},
		{name: "explicit true", attributes: "allow_all_queues = true", mode: "true"},
		{name: "explicit empty selection", attributes: "allowed_queue_ids = []", ids: "[]"},
	} {
		for _, updating := range []bool{false, true} {
			operation := "createClusterAgentToken"
			if updating {
				operation = "updateClusterAgentToken"
			}
			t.Run(tc.name+"/"+operation, func(t *testing.T) {
				server, api := newFakeClusterAgentTokenAPI(t)
				api.setPermissionsDisabled(true)
				steps := []resource.TestStep{}
				creates, writes := 0, 1
				if updating {
					creates, writes = 1, 2
					steps = append(steps, resource.TestStep{Config: scopedTokenTestConfig(server, "legacy", "")})
				}
				steps = append(steps, resource.TestStep{
					Config:      scopedTokenTestConfig(server, "rejected queue write", tc.attributes),
					ExpectError: scopedTokenQueueErrorPattern("Agent token queue bindings are not enabled"),
				})
				if updating {
					steps = append(steps, resource.TestStep{
						Config:           scopedTokenTestConfig(server, "legacy", ""),
						ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
						Check: resource.ComposeAggregateTestCheckFunc(
							checkScopedTokenIdentity(api),
							resource.TestCheckResourceAttr(scopedTokenTestAddress, "description", "legacy"),
							resource.TestCheckNoResourceAttr(scopedTokenTestAddress, "allow_all_queues"),
							resource.TestCheckNoResourceAttr(scopedTokenTestAddress, "allowed_queue_ids.#"),
							api.checkLastQueueWrite(operation, tc.mode, tc.ids, writes),
						),
					})
				}
				resource.UnitTest(t, resource.TestCase{
					ProtoV6ProviderFactories: protoV6ProviderFactories(),
					CheckDestroy:             api.checkDestroyed(creates),
					Steps:                    steps,
				})
				if err := api.checkLastQueueWrite(operation, tc.mode, tc.ids, writes)(nil); err != nil {
					t.Fatal(err)
				}
				if err := api.checkDestroyed(creates)(nil); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
