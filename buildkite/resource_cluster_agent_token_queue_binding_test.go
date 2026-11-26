package buildkite

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	scopedTokenTestQueueAUUID              = "11111111-1111-4111-8111-111111111111"
	scopedTokenTestQueueAID                = "Q2x1c3RlclF1ZXVlLS0tMTExMTExMTEtMTExMS00MTExLTgxMTEtMTExMTExMTExMTEx"
	scopedTokenTestQueueBUUID              = "22222222-2222-4222-8222-222222222222"
	scopedTokenTestQueueBID                = "Q2x1c3RlclF1ZXVlLS0tMjIyMjIyMjItMjIyMi00MjIyLTgyMjItMjIyMjIyMjIyMjIy"
	scopedTokenTestQueueCUUID              = "33333333-3333-4333-8333-333333333333"
	scopedTokenTestQueueCID                = "Q2x1c3RlclF1ZXVlLS0tMzMzMzMzMzMtMzMzMy00MzMzLTgzMzMtMzMzMzMzMzMzMzMz"
	scopedTokenTestForeignClusterQueueUUID = "44444444-4444-4444-8444-444444444444"
	scopedTokenTestForeignClusterQueueID   = "Q2x1c3RlclF1ZXVlLS0tNDQ0NDQ0NDQtNDQ0NC00NDQ0LTg0NDQtNDQ0NDQ0NDQ0NDQ0"
	scopedTokenTestForeignOrgQueueUUID     = "55555555-5555-4555-8555-555555555555"
	scopedTokenTestForeignOrgQueueID       = "Q2x1c3RlclF1ZXVlLS0tNTU1NTU1NTUtNTU1NS00NTU1LTg1NTUtNTU1NTU1NTU1NTU1"
	scopedTokenTestUnknownQueueUUID        = "99999999-9999-4999-8999-999999999999"
	scopedTokenTestUnknownQueueID          = "Q2x1c3RlclF1ZXVlLS0tOTk5OTk5OTktOTk5OS00OTk5LTg5OTktOTk5OTk5OTk5OTk5"
	scopedTokenTestWrongTypeQueueID        = "Q2x1c3Rlci0tLTExMTExMTExLTExMTEtNDExMS04MTExLTExMTExMTExMTExMQ=="
)

func scopedTokenQueueIDsJSON(ids ...string) string {
	value, _ := json.Marshal(append([]string{}, ids...))
	return string(value)
}

func scopedTokenQueueAttributes(allowAllQueues bool, uuids ...string) string {
	value, _ := json.Marshal(append([]string{}, uuids...))
	return fmt.Sprintf("allow_all_queues = %t\nallowed_queue_uuids = %s", allowAllQueues, value)
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

func checkScopedTokenQueues(allowAllQueues bool, uuids ...string) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(scopedTokenTestAddress, "allow_all_queues", fmt.Sprint(allowAllQueues)),
		resource.TestCheckResourceAttr(scopedTokenTestAddress, "allowed_queue_uuids.#", fmt.Sprint(len(uuids))),
	}
	for _, uuid := range uuids {
		checks = append(checks, resource.TestCheckTypeSetElemAttr(scopedTokenTestAddress, "allowed_queue_uuids.*", uuid))
	}
	return resource.ComposeAggregateTestCheckFunc(checks...)
}

func TestClusterAgentTokenQueueCreateAgainstFakeAPI(t *testing.T) {
	for _, tc := range []struct {
		name, attributes, mode, ids string
		allowAllQueues              bool
		queueUUIDs                  []string
	}{
		{name: "omitted uses API defaults", allowAllQueues: true},
		{name: "null is omission", attributes: "allow_all_queues = null\nallowed_queue_uuids = null", allowAllQueues: true},
		{name: "explicit unrestricted", attributes: scopedTokenQueueAttributes(true), mode: "true", ids: "[]", allowAllQueues: true},
		{name: "selected queue UUIDs", attributes: scopedTokenQueueAttributes(false, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID), mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueBID), queueUUIDs: []string{scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID}},
		{name: "deny every queue", attributes: scopedTokenQueueAttributes(false), mode: "false", ids: "[]"},
		{name: "empty selection does not restrict default mode", attributes: "allowed_queue_uuids = []", ids: "[]", allowAllQueues: true},
		{name: "duplicates are a set", attributes: scopedTokenQueueAttributes(false, scopedTokenTestQueueBUUID, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID), mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueBID), queueUUIDs: []string{scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID}},
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
							checkScopedTokenQueues(tc.allowAllQueues, tc.queueUUIDs...),
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
	check := func(mode, ids string, writes int, allowAllQueues bool, queueUUIDs ...string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			checkScopedTokenIdentity(api),
			checkScopedTokenPermissions("read_metrics"),
			checkScopedTokenQueues(allowAllQueues, queueUUIDs...),
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
				Config:           scopedTokenTestConfig(server, "test", scopedTokenQueueAttributes(false, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueBID), 2, false, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID),
			},
			{
				Config: scopedTokenTestConfig(server, "test", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: check("false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueBID), 2, false, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID),
			},
			{
				Config:           scopedTokenTestConfig(server, "unrelated update", ""),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("", "", 3, false, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID),
			},
			{
				Config:           scopedTokenTestConfig(server, "mode only", "allow_all_queues = false"),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("false", "", 4, false, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID),
			},
			{
				Config:           scopedTokenTestConfig(server, "list only", fmt.Sprintf("allowed_queue_uuids = [%q]", scopedTokenTestQueueAUUID)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), 5, false, scopedTokenTestQueueAUUID),
			},
			{
				Config:           scopedTokenTestConfig(server, "null mode", fmt.Sprintf("allow_all_queues = null\nallowed_queue_uuids = [%q]", scopedTokenTestQueueBUUID)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("", scopedTokenQueueIDsJSON(scopedTokenTestQueueBID), 6, false, scopedTokenTestQueueBUUID),
			},
			{
				Config:           scopedTokenTestConfig(server, "null list", "allow_all_queues = false\nallowed_queue_uuids = null"),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("false", "", 7, false, scopedTokenTestQueueBUUID),
			},
			{
				Config:           scopedTokenTestConfig(server, "deny all with list only", "allowed_queue_uuids = []"),
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
				Config:           scopedTokenTestConfig(server, "empty to selected", scopedTokenQueueAttributes(false, scopedTokenTestQueueBUUID)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("false", scopedTokenQueueIDsJSON(scopedTokenTestQueueBID), 11, false, scopedTokenTestQueueBUUID),
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
		checkScopedTokenQueues(false, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID),
		api.checkLastQueueWrite("createClusterAgentToken", "false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID, scopedTokenTestQueueBID), 1),
	)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{
				Config: scopedTokenTestConfig(server, "test", scopedTokenQueueAttributes(false, scopedTokenTestQueueBUUID, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID)),
				Check:  check,
			},
			{
				Config: scopedTokenTestConfig(server, "test", scopedTokenQueueAttributes(false, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID)),
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
		name       string
		queueUUIDs []string
		queueIDs   []string
		drift      func(*fakeClusterAgentTokenAPI)
	}{
		{name: "mode only", drift: func(api *fakeClusterAgentTokenAPI) { api.setQueueAccess(true) }},
		{name: "selection only", queueUUIDs: []string{scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID}, queueIDs: []string{scopedTokenTestQueueAID, scopedTokenTestQueueBID}, drift: func(api *fakeClusterAgentTokenAPI) { api.setQueueAccess(false, scopedTokenTestQueueBID) }},
		{name: "mode and selection", queueUUIDs: []string{scopedTokenTestQueueAUUID}, queueIDs: []string{scopedTokenTestQueueAID}, drift: func(api *fakeClusterAgentTokenAPI) { api.setQueueAccess(true) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, api := newFakeClusterAgentTokenAPI(t)
			config := scopedTokenTestConfig(server, "test", scopedTokenQueueAttributes(false, tc.queueUUIDs...))
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkDestroyed(1),
				Steps: []resource.TestStep{
					{Config: config, Check: checkScopedTokenQueues(false, tc.queueUUIDs...)},
					{
						PreConfig:        func() { tc.drift(api) },
						Config:           config,
						ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
						Check: resource.ComposeAggregateTestCheckFunc(
							checkScopedTokenIdentity(api),
							checkScopedTokenPermissions("run_jobs", "read_build_metadata", "read_metrics"),
							checkScopedTokenQueues(false, tc.queueUUIDs...),
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
	check := func(allowAllQueues bool, uuids ...string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			checkScopedTokenIdentity(api),
			checkScopedTokenQueues(allowAllQueues, uuids...),
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
				Check:        check(false, scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID),
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
			config := func(uuid string) string {
				uuids := "[terraform_data.binding.output.uuid]"
				if wholeSet {
					uuids = "terraform_data.binding.output.uuids"
				}
				return scopedTokenTestConfig(server, "test", "permissions = [\"read_metrics\"]\nallow_all_queues = terraform_data.binding.output.mode\nallowed_queue_uuids = "+uuids) + fmt.Sprintf(`
resource "terraform_data" "binding" {
  input = {
    mode = false
    uuid = %q
    uuids = [%q]
  }
}
`, uuid, uuid)
			}
			check := func(operation, uuid, id string, writes int) resource.TestCheckFunc {
				return resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenPermissions("read_metrics"),
					checkScopedTokenQueues(false, uuid),
					api.checkLastQueueWrite(operation, "false", scopedTokenQueueIDsJSON(id), writes),
				)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkDestroyed(1),
				Steps: []resource.TestStep{
					{
						Config:           config(scopedTokenTestQueueAUUID),
						ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
						Check:            check("createClusterAgentToken", scopedTokenTestQueueAUUID, scopedTokenTestQueueAID, 1),
					},
					{
						Config:           config(scopedTokenTestQueueBUUID),
						ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
						Check:            check("updateClusterAgentToken", scopedTokenTestQueueBUUID, scopedTokenTestQueueBID, 2),
					},
				},
			})
		})
	}
}

func TestClusterAgentTokenQueueInvalidUUIDsRejectedBeforeMutationAgainstFakeAPI(t *testing.T) {
	for _, tc := range []struct {
		name, value string
	}{
		{name: "GraphQL queue ID", value: scopedTokenTestQueueAID},
		{name: "GraphQL ID for another node type", value: scopedTokenTestWrongTypeQueueID},
		{name: "queue key", value: "default"},
		{name: "malformed UUID", value: "11111111-1111-4111-8111-11111111111g"},
		{name: "uppercase UUID", value: strings.ToUpper(scopedTokenTestClusterUUID)},
		{name: "compact UUID", value: strings.ReplaceAll(scopedTokenTestQueueAUUID, "-", "")},
		{name: "padded UUID", value: " " + scopedTokenTestQueueAUUID},
		{name: "empty UUID", value: ""},
	} {
		for _, updating := range []bool{false, true} {
			operation := "createClusterAgentToken"
			if updating {
				operation = "updateClusterAgentToken"
			}
			t.Run(tc.name+"/"+operation, func(t *testing.T) {
				server, api := newFakeClusterAgentTokenAPI(t)
				config := scopedTokenTestConfig(server, "original", "permissions = [\"read_metrics\"]\n"+scopedTokenQueueAttributes(false, scopedTokenTestQueueAUUID))
				steps := []resource.TestStep{}
				creates := 0
				if updating {
					creates = 1
					steps = append(steps, resource.TestStep{Config: config, Check: checkScopedTokenIdentity(api)})
				}
				steps = append(steps, resource.TestStep{
					Config:      scopedTokenTestConfig(server, "must not be stored", "permissions = [\"read_secrets\"]\n"+scopedTokenQueueAttributes(false, tc.value)),
					ExpectError: scopedTokenQueueErrorPattern("must be a canonical lowercase queue UUID"),
				})
				if updating {
					steps = append(steps, resource.TestStep{
						Config:           config,
						ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
						Check: resource.ComposeAggregateTestCheckFunc(
							checkScopedTokenIdentity(api),
							resource.TestCheckResourceAttr(scopedTokenTestAddress, "description", "original"),
							checkScopedTokenPermissions("read_metrics"),
							checkScopedTokenQueues(false, scopedTokenTestQueueAUUID),
							api.checkLastWrite("createClusterAgentToken", `["read_metrics"]`, 1),
							api.checkLastQueueWrite("createClusterAgentToken", "false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), 1),
						),
					})
				}
				resource.UnitTest(t, resource.TestCase{
					ProtoV6ProviderFactories: protoV6ProviderFactories(),
					CheckDestroy:             api.checkDestroyed(creates),
					Steps:                    steps,
				})
				api.mu.Lock()
				defer api.mu.Unlock()
				if len(api.writes) != creates {
					t.Fatalf("invalid UUID reached a token mutation: %+v", api.writes)
				}
			})
		}
	}
}

func TestClusterAgentTokenQueueInvalidCreatesAgainstFakeAPI(t *testing.T) {
	for _, tc := range []struct {
		name, attributes, mode, ids, message string
	}{
		{name: "restricted requires explicit selection", attributes: "allow_all_queues = false", mode: "false", message: "Creating a restricted token requires an explicit allowedQueueIds array"},
		{name: "explicit unrestricted conflicts with selection", attributes: scopedTokenQueueAttributes(true, scopedTokenTestQueueAUUID), mode: "true", ids: scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), message: "All-queues access cannot be combined with queue bindings"},
		{name: "default unrestricted conflicts with selection", attributes: fmt.Sprintf("allowed_queue_uuids = [%q]", scopedTokenTestQueueAUUID), ids: scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), message: "All-queues access cannot be combined with queue bindings"},
		{name: "unknown queue", attributes: scopedTokenQueueAttributes(false, scopedTokenTestUnknownQueueUUID), mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestUnknownQueueID), message: "Queue ID does not identify an available ClusterQueue"},
		{name: "foreign cluster", attributes: scopedTokenQueueAttributes(false, scopedTokenTestForeignClusterQueueUUID), mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestForeignClusterQueueID), message: "Queue belongs to another cluster"},
		{name: "foreign organization", attributes: scopedTokenQueueAttributes(false, scopedTokenTestForeignOrgQueueUUID), mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestForeignOrgQueueID), message: "Queue belongs to another organization"},
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
		{name: "foreign cluster queue cannot change permissions or description", attributes: scopedTokenQueueAttributes(false, scopedTokenTestForeignClusterQueueUUID), permissions: `["read_secrets"]`, mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestForeignClusterQueueID), message: "Queue belongs to another cluster"},
		{name: "foreign organization queue cannot change permissions or description", attributes: scopedTokenQueueAttributes(false, scopedTokenTestForeignOrgQueueUUID), permissions: `["read_secrets"]`, mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestForeignOrgQueueID), message: "Queue belongs to another organization"},
		{name: "unknown queue cannot change permissions or description", attributes: scopedTokenQueueAttributes(false, scopedTokenTestUnknownQueueUUID), permissions: `["read_secrets"]`, mode: "false", ids: scopedTokenQueueIDsJSON(scopedTokenTestUnknownQueueID), message: "Queue ID does not identify an available ClusterQueue"},
		{name: "invalid permission cannot change queue mode or selection", attributes: scopedTokenQueueAttributes(true), permissions: `["unknown_permission"]`, mode: "true", ids: "[]", message: "Unknown agent token permission from API: unknown_permission"},
		{name: "unrestricted must explicitly clear bound queues", attributes: "allow_all_queues = true", permissions: `["read_secrets"]`, mode: "true", message: "All-queues access cannot be combined with queue bindings"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, api := newFakeClusterAgentTokenAPI(t)
			config := scopedTokenTestConfig(server, "original", "permissions = [\"read_metrics\"]\n"+scopedTokenQueueAttributes(false, scopedTokenTestQueueAUUID))
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
							checkScopedTokenQueues(false, scopedTokenTestQueueAUUID),
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
		queueUUIDs     []string
		queueIDs       []string
		missingQueue   bool
	}{
		{name: "selected", queueUUIDs: []string{scopedTokenTestQueueAUUID, scopedTokenTestQueueBUUID}, queueIDs: []string{scopedTokenTestQueueAID, scopedTokenTestQueueBID}},
		{name: "deny every queue"},
		{name: "unrestricted", allowAllQueues: true},
		{name: "deleted bound queue is retained", queueUUIDs: []string{scopedTokenTestQueueAUUID}, queueIDs: []string{scopedTokenTestQueueAID}, missingQueue: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, api := newFakeClusterAgentTokenAPI(t)
			attributes := "permissions = [\"read_metrics\"]\n" + scopedTokenQueueAttributes(tc.allowAllQueues, tc.queueUUIDs...)
			config := scopedTokenTestConfig(server, "test", attributes)
			check := resource.ComposeAggregateTestCheckFunc(
				checkScopedTokenIdentity(api),
				checkScopedTokenPermissions("read_metrics"),
				checkScopedTokenQueues(tc.allowAllQueues, tc.queueUUIDs...),
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
							if len(states) != 1 || states[0].ID != scopedTokenTestID || states[0].Attributes["token"] != "" || states[0].Attributes["allow_all_queues"] != fmt.Sprint(tc.allowAllQueues) || states[0].Attributes["allowed_queue_uuids.#"] != fmt.Sprint(len(tc.queueUUIDs)) {
								return fmt.Errorf("import must recover queue metadata without recovering the secret: %+v", states)
							}
							var got []string
							for key, value := range states[0].Attributes {
								if strings.HasPrefix(key, "allowed_queue_uuids.") && key != "allowed_queue_uuids.#" {
									got = append(got, value)
								}
							}
							want := slices.Clone(tc.queueUUIDs)
							slices.Sort(got)
							slices.Sort(want)
							if !slices.Equal(got, want) {
								return fmt.Errorf("imported queue UUIDs = %v, want %v", got, want)
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
	config := scopedTokenTestConfig(server, "test", scopedTokenQueueAttributes(false, scopedTokenTestQueueAUUID))
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{Config: config, Check: checkScopedTokenQueues(false, scopedTokenTestQueueAUUID)},
			{
				PreConfig: func() {
					api.deleteQueue(scopedTokenTestQueueAID)
					api.deleteQueue(scopedTokenTestQueueCID)
				},
				Config:           scopedTokenTestConfig(server, "retains explicitly managed missing binding", scopedTokenQueueAttributes(false, scopedTokenTestQueueAUUID)),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenQueues(false, scopedTokenTestQueueAUUID),
					api.checkLastQueueWrite("updateClusterAgentToken", "false", scopedTokenQueueIDsJSON(scopedTokenTestQueueAID), 2),
				),
			},
			{
				Config:           scopedTokenTestConfig(server, "retains omitted missing binding", ""),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenQueues(false, scopedTokenTestQueueAUUID),
					api.checkLastQueueWrite("updateClusterAgentToken", "", "", 3),
				),
			},
			{
				Config:      scopedTokenTestConfig(server, "must not add missing queue", scopedTokenQueueAttributes(false, scopedTokenTestQueueAUUID, scopedTokenTestQueueCUUID)),
				ExpectError: scopedTokenQueueErrorPattern("Queue ID does not identify an available ClusterQueue"),
			},
			{
				Config:           scopedTokenTestConfig(server, "retains omitted missing binding", ""),
				ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
				Check: resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenQueues(false, scopedTokenTestQueueAUUID),
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
			resource.TestCheckNoResourceAttr(scopedTokenTestAddress, "allowed_queue_uuids.#"),
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
				Config:           scopedTokenTestConfig(server, "legacy update", "allow_all_queues = null\nallowed_queue_uuids = null"),
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
			checkScopedTokenQueues(false, scopedTokenTestQueueAUUID),
			api.checkLastQueueWrite(operation, mode, ids, writes),
		)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{
				Config: scopedTokenTestConfig(server, "test", "permissions = [\"read_metrics\"]\n"+scopedTokenQueueAttributes(false, scopedTokenTestQueueAUUID)),
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
		{name: "explicit empty selection", attributes: "allowed_queue_uuids = []", ids: "[]"},
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
							resource.TestCheckNoResourceAttr(scopedTokenTestAddress, "allowed_queue_uuids.#"),
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
