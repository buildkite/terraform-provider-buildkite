package buildkite

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

const (
	scopedTokenTestAddress     = "buildkite_cluster_agent_token.test"
	scopedTokenTestID          = "scoped-token-graphql-id"
	scopedTokenTestUUID        = "73b3e7a9-1111-4222-8333-444455556666"
	scopedTokenTestClusterID   = "scoped-token-cluster-graphql-id"
	scopedTokenTestClusterUUID = "a941f205-1111-4222-8333-444455556666"
	scopedTokenTestSecret      = "fake-agent-token-secret"
	scopedTokenTestCatalogue   = `["run_jobs","upload_steps","upload_artifacts","download_artifacts","read_build_metadata","write_build_metadata","write_annotations","read_secrets","write_steps","read_steps","request_oidc_tokens","read_metrics","cancel_builds","ai_proxy","code_repository_access","operate_stacks","read_cache_registries","write_cache_registries","github_actions_compatibility"]`
)

type fakeClusterAgentTokenWrite struct {
	operation       string
	permissions     string
	allowAllQueues  string
	allowedQueueIDs string
}

type fakeClusterAgentTokenQueue struct {
	clusterID      string
	organizationID string
}

type fakeClusterAgentTokenAPI struct {
	t                   *testing.T
	mu                  sync.Mutex
	exists              bool
	description         string
	allowedIPAddresses  string
	expiresAt           *string
	permissions         []string
	defaultPermissions  []string
	permissionsDisabled bool
	allowAllQueues      bool
	allowedQueueIDs     []string
	queues              map[string]fakeClusterAgentTokenQueue
	responses           int
	reads               int
	creates             int
	revokes             int
	writes              []fakeClusterAgentTokenWrite
}

func newFakeClusterAgentTokenAPI(t *testing.T) (*httptest.Server, *fakeClusterAgentTokenAPI) {
	t.Helper()
	api := &fakeClusterAgentTokenAPI{
		t:                  t,
		defaultPermissions: []string{"run_jobs", "read_build_metadata", "read_metrics"},
		allowAllQueues:     true,
		allowedQueueIDs:    []string{},
		queues: map[string]fakeClusterAgentTokenQueue{
			scopedTokenTestQueueAID:              {clusterID: scopedTokenTestClusterID, organizationID: "org-id"},
			scopedTokenTestQueueBID:              {clusterID: scopedTokenTestClusterID, organizationID: "org-id"},
			scopedTokenTestQueueCID:              {clusterID: scopedTokenTestClusterID, organizationID: "org-id"},
			scopedTokenTestForeignClusterQueueID: {clusterID: "other-cluster-id", organizationID: "org-id"},
			scopedTokenTestForeignOrgQueueID:     {clusterID: "foreign-cluster-id", organizationID: "foreign-org-id"},
		},
	}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	return server, api
}

func (a *fakeClusterAgentTokenAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
		a.t.Errorf("unexpected token request: %s %s", r.Method, r.URL.Path)
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	if r.Header.Get("Authorization") != "Bearer fake" {
		a.t.Error("token request did not use the configured fake API token")
	}
	var request struct {
		OperationName string                     `json:"operationName"`
		Query         string                     `json:"query"`
		Variables     map[string]json.RawMessage `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		a.t.Errorf("decode token GraphQL request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var response any
	switch request.OperationName {
	case "":
		if strings.Join(strings.Fields(request.Query), "") != "query($slug:ID!){organization(slug:$slug){id}}" {
			a.t.Errorf("unexpected anonymous query: %s", request.Query)
			a.writeError(w, "Unexpected anonymous GraphQL query")
			return
		}
		a.checkVariable(request.Variables, "slug", "acme")
		response = map[string]any{"organization": map[string]any{"id": "org-id"}}
	case "createClusterAgentToken", "updateClusterAgentToken":
		for _, field := range []string{"allowAllQueues", "allowedQueueIds"} {
			if !strings.Contains(request.Query, field) {
				a.t.Errorf("token mutation does not request %s: %s", field, request.Query)
			}
		}
		raw, present := request.Variables["permissions"]
		rawMode, modePresent := request.Variables["allowAllQueues"]
		rawQueueIDs, queueIDsPresent := request.Variables["allowedQueueIds"]
		a.writes = append(a.writes, fakeClusterAgentTokenWrite{
			operation: request.OperationName, permissions: string(raw),
			allowAllQueues: string(rawMode), allowedQueueIDs: string(rawQueueIDs),
		})
		var permissions []string
		if present {
			if a.permissionsDisabled {
				a.writeError(w, "Scoped agent token permissions are not enabled for this organization")
				return
			}
			if string(raw) == "null" || json.Unmarshal(raw, &permissions) != nil {
				a.writeError(w, "Token permissions must be an array, not null")
				return
			}
			var catalogue []string
			if err := json.Unmarshal([]byte(scopedTokenTestCatalogue), &catalogue); err != nil {
				a.t.Errorf("decode permission catalogue: %v", err)
				return
			}
			for _, permission := range permissions {
				if !slices.Contains(catalogue, permission) {
					a.writeError(w, "Unknown agent token permission from API: "+permission)
					return
				}
			}
		}
		if a.permissionsDisabled && (modePresent || queueIDsPresent) {
			a.writeError(w, "Agent token queue bindings are not enabled for this organization")
			return
		}
		creating := request.OperationName == "createClusterAgentToken"
		allowAllQueues := a.allowAllQueues
		allowedQueueIDs := slices.Clone(a.allowedQueueIDs)
		if creating {
			allowAllQueues = true
			allowedQueueIDs = []string{}
		}
		if modePresent {
			if string(rawMode) == "null" || json.Unmarshal(rawMode, &allowAllQueues) != nil {
				a.writeError(w, "allowAllQueues must be a boolean, not null")
				return
			}
		}
		if queueIDsPresent {
			if string(rawQueueIDs) == "null" || json.Unmarshal(rawQueueIDs, &allowedQueueIDs) != nil {
				a.writeError(w, "allowedQueueIds must be an array of GraphQL queue IDs, not null")
				return
			}
			for _, id := range allowedQueueIDs {
				decoded, err := base64.StdEncoding.DecodeString(id)
				nodeType, _, ok := strings.Cut(string(decoded), "---")
				if err != nil || !ok || nodeType != "ClusterQueue" {
					a.t.Errorf("allowedQueueIds contains %q, want an encoded ClusterQueue ID", id)
					a.writeError(w, "Queue ID must identify a ClusterQueue")
					return
				}
				queue, ok := a.queues[id]
				if !ok {
					if !creating && slices.Contains(a.allowedQueueIDs, id) {
						continue
					}
					a.writeError(w, "Queue ID does not identify an available ClusterQueue: "+id)
					return
				}
				if queue.organizationID != "org-id" {
					a.writeError(w, "Queue belongs to another organization: "+id)
					return
				}
				if queue.clusterID != scopedTokenTestClusterID {
					a.writeError(w, "Queue belongs to another cluster: "+id)
					return
				}
			}
		}
		if creating && !allowAllQueues && !queueIDsPresent {
			a.writeError(w, "Creating a restricted token requires an explicit allowedQueueIds array")
			return
		}
		if allowAllQueues && len(allowedQueueIDs) != 0 {
			a.writeError(w, "All-queues access cannot be combined with queue bindings; explicitly clear allowedQueueIds")
			return
		}
		var description, allowedIPAddresses string
		if err := json.Unmarshal(request.Variables["description"], &description); err != nil {
			a.t.Errorf("description: %v", err)
			return
		}
		if err := json.Unmarshal(request.Variables["allowedIpAddresses"], &allowedIPAddresses); err != nil {
			a.t.Errorf("allowedIpAddresses: %v", err)
			return
		}
		a.checkVariable(request.Variables, "organizationId", "org-id")
		field := "clusterAgentTokenUpdate"
		if creating {
			a.checkVariable(request.Variables, "clusterId", scopedTokenTestClusterID)
			if a.exists {
				a.t.Error("an existing token was recreated")
			}
			a.creates++
			a.exists = true
			a.permissions = slices.Clone(a.defaultPermissions)
			if raw, ok := request.Variables["expiresAt"]; ok {
				if err := json.Unmarshal(raw, &a.expiresAt); err != nil {
					a.t.Errorf("expiresAt = %s: %v", raw, err)
				}
			}
			field = "clusterAgentTokenCreate"
		} else {
			a.checkVariable(request.Variables, "id", scopedTokenTestID)
			if !a.exists {
				a.writeError(w, "Token does not exist")
				return
			}
		}
		if present {
			a.permissions = make([]string, 0, len(permissions))
			for _, permission := range permissions {
				if !slices.Contains(a.permissions, permission) {
					a.permissions = append(a.permissions, permission)
				}
			}
		}
		a.allowAllQueues = allowAllQueues
		a.allowedQueueIDs = make([]string, 0, len(allowedQueueIDs))
		for _, id := range allowedQueueIDs {
			if !slices.Contains(a.allowedQueueIDs, id) {
				a.allowedQueueIDs = append(a.allowedQueueIDs, id)
			}
		}
		a.description = description
		a.allowedIPAddresses = allowedIPAddresses
		payload := map[string]any{"clusterAgentToken": a.token()}
		if field == "clusterAgentTokenCreate" {
			payload["tokenValue"] = scopedTokenTestSecret
		}
		response = map[string]any{field: payload}
	case "getClusterAgentToken", "getNode":
		a.checkVariable(request.Variables, "id", scopedTokenTestID)
		if request.OperationName == "getClusterAgentToken" {
			a.reads++
			for _, field := range []string{"ClusterToken", "permissions", "allowAllQueues", "allowedQueueIds", "organization", "slug"} {
				if !strings.Contains(request.Query, field) {
					a.t.Errorf("token node query does not request %s: %s", field, request.Query)
				}
			}
		}
		var node any
		if a.exists {
			node = a.token()
		}
		response = map[string]any{"node": node}
	case "revokeClusterAgentToken":
		a.checkVariable(request.Variables, "organizationId", "org-id")
		a.checkVariable(request.Variables, "id", scopedTokenTestID)
		for _, field := range []string{"permissions", "allowAllQueues", "allowedQueueIds"} {
			if _, present := request.Variables[field]; present {
				a.t.Errorf("revoke sent a %s variable", field)
			}
		}
		a.revokes++
		a.exists = false
		response = map[string]any{"clusterAgentTokenRevoke": map[string]any{"deletedClusterAgentTokenId": scopedTokenTestID}}
	default:
		a.t.Errorf("unexpected token GraphQL operation %q: %s", request.OperationName, request.Query)
		a.writeError(w, "Unexpected token GraphQL operation")
		return
	}
	if err := json.NewEncoder(w).Encode(map[string]any{"data": response}); err != nil {
		a.t.Errorf("encode token GraphQL response: %v", err)
	}
}

func (a *fakeClusterAgentTokenAPI) checkVariable(variables map[string]json.RawMessage, key, want string) {
	var got string
	if err := json.Unmarshal(variables[key], &got); err != nil || got != want {
		a.t.Errorf("%s = %s, want %q", key, variables[key], want)
	}
}

func (a *fakeClusterAgentTokenAPI) writeError(w http.ResponseWriter, message string) {
	if err := json.NewEncoder(w).Encode(map[string]any{
		"data": nil, "errors": []map[string]string{{"message": message}},
	}); err != nil {
		a.t.Errorf("encode token GraphQL error: %v", err)
	}
}

func (a *fakeClusterAgentTokenAPI) token() map[string]any {
	permissions := append([]string{}, a.permissions...)
	allowedQueueIDs := append([]string{}, a.allowedQueueIDs...)
	var allowAllQueues any = a.allowAllQueues
	a.responses++
	if a.responses%2 == 0 {
		slices.Reverse(permissions)
		slices.Reverse(allowedQueueIDs)
	}
	if a.permissionsDisabled {
		permissions = nil
		allowedQueueIDs = nil
		allowAllQueues = nil
	}
	return map[string]any{
		"__typename":         "ClusterToken",
		"id":                 scopedTokenTestID,
		"uuid":               scopedTokenTestUUID,
		"description":        a.description,
		"allowedIpAddresses": a.allowedIPAddresses,
		"expiresAt":          a.expiresAt,
		"permissions":        permissions,
		"allowAllQueues":     allowAllQueues,
		"allowedQueueIds":    allowedQueueIDs,
		"cluster": map[string]any{
			"id": scopedTokenTestClusterID, "uuid": scopedTokenTestClusterUUID,
			"organization": map[string]string{"slug": "acme"},
		},
	}
}

func (a *fakeClusterAgentTokenAPI) setPermissions(permissions ...string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissions = append([]string{}, permissions...)
}

func (a *fakeClusterAgentTokenAPI) setPermissionsDisabled(disabled bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.permissionsDisabled = disabled
}

func (a *fakeClusterAgentTokenAPI) checkLastWrite(operation, permissions string, count int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if len(a.writes) != count || count == 0 {
			return fmt.Errorf("token writes = %+v, want %d writes", a.writes, count)
		}
		write := a.writes[count-1]
		if write.operation != operation {
			return fmt.Errorf("last token mutation = %s, want %s", write.operation, operation)
		}
		return compareScopedTokenPermissionsJSON(write.permissions, permissions)
	}
}

func (a *fakeClusterAgentTokenAPI) checkLiveToken() resource.TestCheckFunc {
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if !a.exists || a.creates != 1 || a.revokes != 0 {
			return fmt.Errorf("token lifecycle: exists=%t creates=%d revokes=%d, want one unreplaced, unrevoked token", a.exists, a.creates, a.revokes)
		}
		return nil
	}
}

func (a *fakeClusterAgentTokenAPI) checkDestroyed(creates int) resource.TestCheckFunc {
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.exists || a.creates != creates || a.revokes != creates || (creates > 0 && a.reads == 0) {
			return fmt.Errorf("token lifecycle after destroy: exists=%t creates=%d revokes=%d node reads=%d, want %d creates, node refreshes, and final revokes only", a.exists, a.creates, a.revokes, a.reads, creates)
		}
		return nil
	}
}

func compareScopedTokenPermissionsJSON(got, want string) error {
	if want == "" || want == "null" {
		if got != want {
			return fmt.Errorf("permissions variable = %q, want %q (empty string means omitted)", got, want)
		}
		return nil
	}
	var actual, expected []string
	if err := json.Unmarshal([]byte(got), &actual); err != nil || actual == nil {
		return fmt.Errorf("permissions variable = %q, want array %s", got, want)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		return fmt.Errorf("invalid expected permissions %q: %w", want, err)
	}
	slices.Sort(actual)
	slices.Sort(expected)
	if !slices.Equal(actual, expected) {
		return fmt.Errorf("permissions variable = %s, want %s without ordering differences", got, want)
	}
	return nil
}

func scopedTokenTestConfig(server *httptest.Server, description, attributes string) string {
	return fmt.Sprintf(`
provider "buildkite" {
  organization = "acme"
  api_token = "fake"
  rest_url = %q
  graphql_url = %q
  max_retries = 0
}
resource "buildkite_cluster_agent_token" "test" {
  cluster_id = %q
  description = %q
  allowed_ip_addresses = ["192.0.2.0/24"]
  expires_at = "2099-01-01T00:00:00Z"
  %s
}
`, server.URL, server.URL+"/graphql", scopedTokenTestClusterID, description, attributes)
}

func checkScopedTokenPermissions(permissions ...string) resource.TestCheckFunc {
	checks := []resource.TestCheckFunc{
		resource.TestCheckResourceAttr(scopedTokenTestAddress, "permissions.#", fmt.Sprint(len(permissions))),
	}
	for _, permission := range permissions {
		checks = append(checks, resource.TestCheckTypeSetElemAttr(scopedTokenTestAddress, "permissions.*", permission))
	}
	return resource.ComposeAggregateTestCheckFunc(checks...)
}

func checkScopedTokenIdentity(api *fakeClusterAgentTokenAPI) resource.TestCheckFunc {
	return resource.ComposeAggregateTestCheckFunc(
		resource.TestCheckResourceAttr(scopedTokenTestAddress, "id", scopedTokenTestID),
		resource.TestCheckResourceAttr(scopedTokenTestAddress, "uuid", scopedTokenTestUUID),
		resource.TestCheckResourceAttr(scopedTokenTestAddress, "cluster_id", scopedTokenTestClusterID),
		resource.TestCheckResourceAttr(scopedTokenTestAddress, "cluster_uuid", scopedTokenTestClusterUUID),
		resource.TestCheckResourceAttr(scopedTokenTestAddress, "token", scopedTokenTestSecret),
		api.checkLiveToken(),
	)
}

func scopedTokenEmptyPlanChecks() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
}

func scopedTokenUpdatePlanChecks() resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{
		PreApply: []plancheck.PlanCheck{
			plancheck.ExpectResourceAction(scopedTokenTestAddress, plancheck.ResourceActionUpdate),
		},
		PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
	}
}

func TestClusterAgentTokenPermissionsCreateAgainstFakeAPI(t *testing.T) {
	var catalogue []string
	if err := json.Unmarshal([]byte(scopedTokenTestCatalogue), &catalogue); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, attributes, write string
		permissions             []string
	}{
		{"omitted uses API defaults", "", "", []string{"run_jobs", "read_build_metadata", "read_metrics"}},
		{"null uses API defaults", "permissions = null", "", []string{"run_jobs", "read_build_metadata", "read_metrics"}},
		{"full catalogue", "permissions = " + scopedTokenTestCatalogue, scopedTokenTestCatalogue, catalogue},
		{"restricted", `permissions = ["run_jobs", "read_secrets"]`, `["read_secrets","run_jobs"]`, []string{"run_jobs", "read_secrets"}},
		{"empty grants nothing", "permissions = []", "[]", []string{}},
		{"duplicates are a set", `permissions = ["upload_artifacts", "run_jobs", "upload_artifacts", "run_jobs"]`, `["run_jobs","upload_artifacts"]`, []string{"run_jobs", "upload_artifacts"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, api := newFakeClusterAgentTokenAPI(t)
			config := scopedTokenTestConfig(server, "test", tc.attributes)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkDestroyed(1),
				Steps: []resource.TestStep{
					{
						Config:           config,
						ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
						Check: resource.ComposeAggregateTestCheckFunc(
							checkScopedTokenIdentity(api),
							checkScopedTokenPermissions(tc.permissions...),
							api.checkLastWrite("createClusterAgentToken", tc.write, 1),
						),
					},
					{Config: config, PlanOnly: true},
				},
			})
		})
	}
}

func TestClusterAgentTokenPermissionsInPlaceUpdatesAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	restricted := `permissions = ["run_jobs", "upload_artifacts"]`
	changed := `permissions = ["read_steps", "read_secrets"]`
	check := func(operation, permissions string, writes int, scope ...string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			checkScopedTokenIdentity(api),
			checkScopedTokenPermissions(scope...),
			api.checkLastWrite(operation, permissions, writes),
		)
	}
	var catalogue []string
	if err := json.Unmarshal([]byte(scopedTokenTestCatalogue), &catalogue); err != nil {
		t.Fatal(err)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{
				Config: scopedTokenTestConfig(server, "test", restricted),
				Check:  check("createClusterAgentToken", `["run_jobs","upload_artifacts"]`, 1, "run_jobs", "upload_artifacts"),
			},
			{
				Config:           scopedTokenTestConfig(server, "test", "permissions = []"),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("updateClusterAgentToken", "[]", 2),
			},
			{
				Config:           scopedTokenTestConfig(server, "test", "permissions = "+scopedTokenTestCatalogue),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("updateClusterAgentToken", scopedTokenTestCatalogue, 3, catalogue...),
			},
			{
				Config:           scopedTokenTestConfig(server, "test", changed),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("updateClusterAgentToken", `["read_secrets","read_steps"]`, 4, "read_secrets", "read_steps"),
			},
			{
				Config: scopedTokenTestConfig(server, "test", ""),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply:             []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: check("updateClusterAgentToken", `["read_secrets","read_steps"]`, 4, "read_secrets", "read_steps"),
			},
			{
				Config:           scopedTokenTestConfig(server, "description changed", ""),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("updateClusterAgentToken", "", 5, "read_secrets", "read_steps"),
			},
			{
				Config:           scopedTokenTestConfig(server, "description changed again", "permissions = null"),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("updateClusterAgentToken", "", 6, "read_secrets", "read_steps"),
			},
			{
				PreConfig:        func() { api.setPermissions("run_jobs", "read_cache_registries") },
				Config:           scopedTokenTestConfig(server, "unmanaged scope adopted", ""),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("updateClusterAgentToken", "", 7, "run_jobs", "read_cache_registries"),
			},
		},
	})
}

func TestClusterAgentTokenPermissionsDriftAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	config := scopedTokenTestConfig(server, "test", `permissions = ["run_jobs", "upload_artifacts"]`)
	check := func(writes int) resource.TestCheckFunc {
		operation := "updateClusterAgentToken"
		if writes == 1 {
			operation = "createClusterAgentToken"
		}
		return resource.ComposeAggregateTestCheckFunc(
			checkScopedTokenIdentity(api),
			checkScopedTokenPermissions("run_jobs", "upload_artifacts"),
			api.checkLastWrite(operation, `["run_jobs","upload_artifacts"]`, writes),
		)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{Config: config, Check: check(1)},
			{
				PreConfig:        func() { api.setPermissions("run_jobs", "upload_artifacts", "read_secrets") },
				Config:           config,
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check(2),
			},
			{
				PreConfig:        func() { api.setPermissions("run_jobs") },
				Config:           config,
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check(3),
			},
			{Config: config, PlanOnly: true},
		},
	})
}

func TestClusterAgentTokenPermissionsRefreshAndImportAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	config := scopedTokenTestConfig(server, "test", "")
	importStep := resource.TestStep{
		ResourceName: scopedTokenTestAddress,
		ImportState:  true, ImportStateId: scopedTokenTestID, ImportStateVerify: true,
		ImportStateVerifyIgnore: []string{"token"},
		ImportStateCheck: func(states []*terraform.InstanceState) error {
			if len(states) != 1 || states[0].ID != scopedTokenTestID || states[0].Attributes["token"] != "" {
				return fmt.Errorf("import must recover one token by GraphQL ID without recovering its secret: %+v", states)
			}
			return nil
		},
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{Config: config, Check: checkScopedTokenIdentity(api)},
			{
				PreConfig:    func() { api.setPermissions("code_repository_access", "github_actions_compatibility") },
				RefreshState: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenPermissions("code_repository_access", "github_actions_compatibility"),
					api.checkLastWrite("createClusterAgentToken", "", 1),
				),
			},
			importStep,
			{Config: config, PlanOnly: true},
			{
				PreConfig:    func() { api.setPermissions() },
				RefreshState: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					checkScopedTokenIdentity(api),
					checkScopedTokenPermissions(),
					api.checkLastWrite("createClusterAgentToken", "", 1),
				),
			},
			importStep,
			{Config: config, PlanOnly: true},
		},
	})
}

func TestClusterAgentTokenPermissionsFeatureDisabledAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	api.setPermissionsDisabled(true)
	check := func(operation string, writes int) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			checkScopedTokenIdentity(api),
			resource.TestCheckNoResourceAttr(scopedTokenTestAddress, "permissions.#"),
			api.checkLastWrite(operation, "", writes),
		)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{
				Config:           scopedTokenTestConfig(server, "test", ""),
				ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
				Check:            check("createClusterAgentToken", 1),
			},
			{
				Config:           scopedTokenTestConfig(server, "description changed", ""),
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

func TestClusterAgentTokenPermissionsFeatureDisabledRetainsKnownStateAgainstFakeAPI(t *testing.T) {
	server, api := newFakeClusterAgentTokenAPI(t)
	check := func(operation, permissions string, writes int) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			checkScopedTokenIdentity(api),
			checkScopedTokenPermissions("run_jobs", "read_secrets"),
			api.checkLastWrite(operation, permissions, writes),
		)
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkDestroyed(1),
		Steps: []resource.TestStep{
			{
				Config: scopedTokenTestConfig(server, "test", `permissions = ["run_jobs", "read_secrets"]`),
				Check:  check("createClusterAgentToken", `["run_jobs","read_secrets"]`, 1),
			},
			{
				PreConfig:    func() { api.setPermissionsDisabled(true) },
				RefreshState: true,
				Check:        check("createClusterAgentToken", `["run_jobs","read_secrets"]`, 1),
			},
			{
				Config:           scopedTokenTestConfig(server, "description changed", ""),
				ConfigPlanChecks: scopedTokenUpdatePlanChecks(),
				Check:            check("updateClusterAgentToken", "", 2),
			},
			{
				Config:      scopedTokenTestConfig(server, "description changed", "permissions = []"),
				ExpectError: regexp.MustCompile(`Scoped agent token permissions\s+are not enabled`),
			},
			{
				Config:           scopedTokenTestConfig(server, "description changed", ""),
				ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
				Check:            check("updateClusterAgentToken", "[]", 3),
			},
		},
	})
}

func TestClusterAgentTokenPermissionsWritesRejectedByFakeAPI(t *testing.T) {
	for _, tc := range []struct {
		name, permissions, message string
		disabled                   bool
	}{
		{"unknown permission", `["future_permission_not_recognized"]`, "Unknown agent token permission from API: future_permission_not_recognized", false},
		{"disabled explicit scope", `["run_jobs"]`, "Scoped agent token permissions are not enabled", true},
		{"disabled explicit empty", `[]`, "Scoped agent token permissions are not enabled", true},
	} {
		for _, updating := range []bool{false, true} {
			operation := "createClusterAgentToken"
			if updating {
				operation = "updateClusterAgentToken"
			}
			t.Run(tc.name+"/"+operation, func(t *testing.T) {
				server, api := newFakeClusterAgentTokenAPI(t)
				api.setPermissionsDisabled(tc.disabled)
				steps := []resource.TestStep{}
				creates, writes := 0, 1
				if updating {
					creates, writes = 1, 2
					steps = append(steps, resource.TestStep{
						Config: scopedTokenTestConfig(server, "test", ""),
						Check: resource.ComposeAggregateTestCheckFunc(
							checkScopedTokenIdentity(api),
							api.checkLastWrite("createClusterAgentToken", "", 1),
						),
					})
				}
				steps = append(steps, resource.TestStep{
					Config:      scopedTokenTestConfig(server, "test", "permissions = "+tc.permissions),
					ExpectError: regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(tc.message), " ", `\s+`)),
				})
				if updating {
					permissionsCheck := checkScopedTokenPermissions("run_jobs", "read_build_metadata", "read_metrics")
					if tc.disabled {
						permissionsCheck = resource.TestCheckNoResourceAttr(scopedTokenTestAddress, "permissions.#")
					}
					steps = append(steps, resource.TestStep{
						Config:           scopedTokenTestConfig(server, "test", ""),
						ConfigPlanChecks: scopedTokenEmptyPlanChecks(),
						Check: resource.ComposeAggregateTestCheckFunc(
							checkScopedTokenIdentity(api),
							permissionsCheck,
							api.checkLastWrite(operation, tc.permissions, writes),
						),
					})
				}
				resource.UnitTest(t, resource.TestCase{
					ProtoV6ProviderFactories: protoV6ProviderFactories(),
					CheckDestroy:             api.checkDestroyed(creates),
					Steps:                    steps,
				})
				if err := api.checkLastWrite(operation, tc.permissions, writes)(nil); err != nil {
					t.Fatal(err)
				}
				if err := api.checkDestroyed(creates)(nil); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
