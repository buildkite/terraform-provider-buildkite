package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// This fake models the current installation-free REST contract plus the agreed
// by-ID rename API. It is not evidence that those endpoints are deployed.
type fakeReleasesAPI struct {
	t       *testing.T
	mu      sync.Mutex
	records map[string]map[string]any
}

func newFakeReleasesAPI(t *testing.T) (*httptest.Server, *fakeReleasesAPI) {
	a := &fakeReleasesAPI{t: t, records: map[string]map[string]any{}}
	s := httptest.NewServer(a)
	t.Cleanup(s.Close)
	return s, a
}

func (a *fakeReleasesAPI) seed(route, name string, extra map[string]any) string {
	id := uuid.NewString()
	record := map[string]any{"id": id, "graphql_id": "opaque-graphql-" + id, "name": name, "archived_at": nil, "created_at": "2026-11-18T00:00:00Z"}
	for k, v := range extra {
		record[k] = v
	}
	a.records[route+"/"+id] = record
	return id
}

func (a *fakeReleasesAPI) named(route, name, target string) map[string]any {
	for key, record := range a.records {
		if strings.HasPrefix(key, route+"/") && record["name"] == name && (route != "component-instances" || record["target_id"] == target) {
			return record
		}
	}
	return nil
}

func (a *fakeReleasesAPI) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	const base = "/v2/organizations/acme/"
	if !strings.HasPrefix(req.URL.Path, base) {
		a.t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
		http.NotFound(w, req)
		return
	}
	parts := strings.Split(strings.TrimPrefix(req.URL.Path, base), "/")
	if len(parts) >= 3 && parts[1] == "by-id" {
		parts[2] = strings.ToLower(parts[2])
	}
	route := parts[0]
	if !slices.Contains([]string{"artifacts", "targets", "components", "component-instances"}, route) {
		a.t.Errorf("unexpected route: %s", req.URL.Path)
		http.NotFound(w, req)
		return
	}
	var body map[string]any
	if req.Method != http.MethodGet {
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			a.t.Errorf("invalid JSON request: %s", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	rejectUnknown := func(allowed ...string) bool {
		for key := range body {
			if !slices.Contains(allowed, key) {
				a.t.Errorf("unexpected %s payload field %q", req.URL.Path, key)
				w.WriteHeader(http.StatusUnprocessableEntity)
				return true
			}
		}
		return false
	}
	if req.Method == http.MethodPost && len(parts) == 1 {
		allowed := []string{"name"}
		extra := map[string]any{}
		switch route {
		case "targets":
			allowed = append(allowed, "labels")
			extra["labels"] = body["labels"]
		case "components":
			allowed = append(allowed, "artifact")
			ref, _ := body["artifact"].(map[string]any)
			artifact := a.records["artifacts/"+strings.ToLower(fmt.Sprint(ref["id"]))]
			if artifact == nil || artifact["archived_at"] != nil {
				w.WriteHeader(http.StatusConflict)
				return
			}
			extra["artifact"] = map[string]any{"id": artifact["id"], "graphql_id": artifact["graphql_id"], "name": artifact["name"]}
		case "component-instances":
			allowed = append(allowed, "target_id", "component_name")
			target := a.records["targets/"+strings.ToLower(fmt.Sprint(body["target_id"]))]
			component := a.named("components", fmt.Sprint(body["component_name"]), "")
			if target == nil || component == nil || target["archived_at"] != nil || component["archived_at"] != nil {
				w.WriteHeader(http.StatusConflict)
				return
			}
			extra["target_id"], extra["component_id"] = target["id"], component["id"]
			if _, ok := body["name"]; !ok {
				body["name"] = component["name"]
			}
		}
		if rejectUnknown(allowed...) {
			return
		}
		name, ok := body["name"].(string)
		if !ok || name == "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		if a.named(route, name, strings.ToLower(fmt.Sprint(body["target_id"]))) != nil {
			w.WriteHeader(http.StatusConflict)
			return
		}
		id := a.seed(route, name, extra)
		writeFakeJSON(w, http.StatusCreated, a.records[route+"/"+id])
		return
	}
	var record map[string]any
	if len(parts) >= 3 && parts[1] == "by-id" {
		record = a.records[route+"/"+parts[2]]
		if len(parts) == 5 && parts[3] == "instances" {
			record = a.named("component-instances", parts[4], parts[2])
		}
	} else if len(parts) == 2 {
		record = a.named(route, parts[1], "")
	}
	if record == nil {
		http.NotFound(w, req)
		return
	}
	switch {
	case req.Method == http.MethodGet:
		writeFakeJSON(w, http.StatusOK, record)
	case req.Method == http.MethodPatch && len(parts) == 3 && parts[1] == "by-id":
		allowed := []string{"name"}
		if route == "targets" {
			allowed = append(allowed, "labels")
		}
		if rejectUnknown(allowed...) {
			return
		}
		if name, exists := body["name"]; exists {
			other := a.named(route, fmt.Sprint(name), fmt.Sprint(record["target_id"]))
			if other != nil && other["id"] != record["id"] {
				w.WriteHeader(http.StatusConflict)
				return
			}
			record["name"] = name
		}
		if labels, exists := body["labels"]; exists {
			if record["archived_at"] != nil {
				w.WriteHeader(http.StatusConflict)
				return
			}
			record["labels"] = labels
		}
		writeFakeJSON(w, http.StatusOK, record)
	case req.Method == http.MethodPost && len(parts) == 4 && (parts[3] == "archive" || parts[3] == "unarchive"):
		if rejectUnknown() {
			return
		}
		if parts[3] == "archive" {
			record["archived_at"] = "2026-11-18T01:00:00Z"
			if route == "targets" {
				for _, instance := range a.records {
					if instance["target_id"] == record["id"] && instance["archived_at"] == nil {
						instance["archived_at"] = record["archived_at"]
					}
				}
			}
		} else {
			if route == "component-instances" && a.records["targets/"+fmt.Sprint(record["target_id"])]["archived_at"] != nil {
				w.WriteHeader(http.StatusConflict)
				return
			}
			record["archived_at"] = nil
		}
		writeFakeJSON(w, http.StatusOK, record)
	default:
		a.t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

var releasesRoutes = map[string]string{
	"buildkite_deployment_artifact.test":   "artifacts",
	"buildkite_target.test":                "targets",
	"buildkite_component.test":             "components",
	"buildkite_component_instance.test":    "component-instances",
	"buildkite_component_instance.default": "component-instances",
}

func releasesConfig(provider, name, labels string, restore bool) string {
	return provider + fmt.Sprintf(`
resource "buildkite_deployment_artifact" "test" {
  name = "%[1]s-artifact"
  allow_unarchive_on_create = %[3]t
}
resource "buildkite_target" "test" {
  name = "%[1]s-target"
  %[2]s
  allow_unarchive_on_create = %[3]t
}
resource "buildkite_component" "test" {
  name = "%[1]s-component"
  artifact_uuid = buildkite_deployment_artifact.test.uuid
  allow_unarchive_on_create = %[3]t
}
resource "buildkite_component_instance" "test" {
  name = "%[1]s-instance"
  target_uuid = buildkite_target.test.uuid
  component_uuid = buildkite_component.test.uuid
  allow_unarchive_on_create = %[3]t
}
resource "buildkite_component_instance" "default" {
  target_uuid = buildkite_target.test.uuid
  component_uuid = buildkite_component.test.uuid
  allow_unarchive_on_create = %[3]t
}
`, name, labels, restore)
}

// The same Terraform steps run offline and against an opted-in real API. Every
// apply checks the REST representation as well as state and an empty refresh plan.
func releasesLifecycle(t *testing.T, provider string, client *Client) resource.TestCase {
	t.Helper()
	prefix := "tf-" + uuid.NewString()
	ids := map[string]string{}
	check := func(name string, labels map[string]string, restore bool) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			for address, route := range releasesRoutes {
				state := s.RootModule().Resources[address].Primary.Attributes
				if ids[address] == "" {
					ids[address] = state["uuid"]
				}
				if state["uuid"] != ids[address] {
					return fmt.Errorf("%s changed UUID: %s -> %s", address, ids[address], state["uuid"])
				}
				var remote releasesRegistration
				if err := client.makeRequest(context.Background(), http.MethodGet, fmt.Sprintf("/v2/organizations/%s/%s/by-id/%s", client.organization, route, ids[address]), nil, &remote); err != nil {
					return err
				}
				wantName := name + "-" + strings.TrimSuffix(route, "s")
				if route == "component-instances" {
					wantName = name + "-instance"
				}
				if address == "buildkite_component_instance.default" {
					wantName = prefix + "-component" // Assigned once; Component rename never renames Instances.
				}
				if remote.UUID != ids[address] || remote.ID != state["id"] || remote.Name != wantName || remote.ArchivedAt != nil || state["name"] != wantName || state["allow_unarchive_on_create"] != fmt.Sprint(restore) {
					return fmt.Errorf("unexpected %s state/remote: %#v / %#v (want name %s)", address, state, remote, wantName)
				}
				if route == "targets" && !reflect.DeepEqual(remote.Labels, labels) {
					return fmt.Errorf("labels = %#v, want %#v", remote.Labels, labels)
				}
				if route == "components" && remote.Artifact.UUID != s.RootModule().Resources["buildkite_deployment_artifact.test"].Primary.Attributes["uuid"] {
					return fmt.Errorf("wrong Artifact binding: %#v", remote)
				}
				if route == "component-instances" && (remote.TargetUUID != s.RootModule().Resources["buildkite_target.test"].Primary.Attributes["uuid"] || remote.ComponentUUID != s.RootModule().Resources["buildkite_component.test"].Primary.Attributes["uuid"]) {
					return fmt.Errorf("wrong Instance bindings: %#v", remote)
				}
			}
			return nil
		}
	}
	checkArchived := func(_ *terraform.State) error {
		for address, route := range releasesRoutes {
			var remote releasesRegistration
			if err := client.makeRequest(context.Background(), http.MethodGet, fmt.Sprintf("/v2/organizations/%s/%s/by-id/%s", client.organization, route, ids[address]), nil, &remote); err != nil {
				return err
			}
			if remote.UUID != ids[address] || remote.ArchivedAt == nil {
				return fmt.Errorf("%s was not archived with its original UUID: %#v", address, remote)
			}
		}
		return nil
	}
	initial := releasesConfig(provider, prefix, `labels = { region = "au", tier = "critical" }`, false)
	renamed := releasesConfig(provider, prefix+"-renamed", `labels = { region = "eu" }`, false)
	cleared := releasesConfig(provider, prefix+"-renamed", "", false)
	restored := releasesConfig(provider, prefix+"-renamed", `labels = { restored = "yes" }`, true)
	// A defaulted Instance retains its original name even after Component rename,
	// so recovery must name that reserved identity explicitly.
	restored = strings.Replace(restored, `resource "buildkite_component_instance" "default" {`, `resource "buildkite_component_instance" "default" {
  name = "`+prefix+`-component"`, 1)
	empty := resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
	steps := []resource.TestStep{{Config: initial, ConfigPlanChecks: empty, Check: check(prefix, map[string]string{"region": "au", "tier": "critical"}, false)}}
	for _, address := range []string{"buildkite_deployment_artifact.test", "buildkite_target.test", "buildkite_component.test", "buildkite_component_instance.test", "buildkite_component_instance.default"} {
		address := address
		steps = append(steps, resource.TestStep{
			ResourceName: address, ImportState: true, ImportStateVerify: true,
			ImportStateIdFunc: func(s *terraform.State) (string, error) {
				return s.RootModule().Resources[address].Primary.Attributes["uuid"], nil
			},
		})
	}
	steps = append(steps,
		resource.TestStep{Config: renamed, ConfigPlanChecks: resource.ConfigPlanChecks{
			PreApply: []plancheck.PlanCheck{
				plancheck.ExpectResourceAction("buildkite_deployment_artifact.test", plancheck.ResourceActionUpdate),
				plancheck.ExpectResourceAction("buildkite_target.test", plancheck.ResourceActionUpdate),
				plancheck.ExpectResourceAction("buildkite_component.test", plancheck.ResourceActionUpdate),
				plancheck.ExpectResourceAction("buildkite_component_instance.test", plancheck.ResourceActionUpdate),
			}, PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
		}, Check: check(prefix+"-renamed", map[string]string{"region": "eu"}, false)},
		resource.TestStep{Config: cleared, ConfigPlanChecks: empty, Check: check(prefix+"-renamed", map[string]string{}, false)},
		resource.TestStep{Config: cleared, Destroy: true, Check: checkArchived},
		resource.TestStep{Config: cleared, ExpectError: regexp.MustCompile("Unable to create registration")},
		resource.TestStep{Config: restored, ConfigPlanChecks: empty, Check: check(prefix+"-renamed", map[string]string{"restored": "yes"}, true)},
	)
	return resource.TestCase{ProtoV6ProviderFactories: protoV6ProviderFactories(), Steps: steps, CheckDestroy: checkArchived}
}

func TestReleasesRegistrationLifecycle(t *testing.T) {
	server, _ := newFakeReleasesAPI(t)
	client := NewClient(&clientConfig{org: "acme", restURL: server.URL})
	resource.UnitTest(t, releasesLifecycle(t, fakeWorkloadIdentityProvider(server), client))
}

func TestAccReleasesRegistrationLifecycle(t *testing.T) {
	if os.Getenv("BUILDKITE_RELEASES_ACCEPTANCE") != "1" {
		t.Skip("Releases live API untested; set BUILDKITE_RELEASES_ACCEPTANCE=1 for a disposable Releases-enabled organization with by-ID PATCH support")
	}
	testAccPreCheck(t)
	resource.Test(t, releasesLifecycle(t, "", getTestClient()))
}

func TestReleasesCreateConflictsAndRestore(t *testing.T) {
	for _, kind := range []string{"deployment_artifact", "target", "component", "component_instance"} {
		for _, scenario := range []string{"active-default", "archived-default", "active-opt-in", "archived-opt-in", "absent-opt-in", "wrong-binding", "archived-uppercase-opt-in"} {
			if (scenario == "wrong-binding" || scenario == "archived-uppercase-opt-in") && kind != "component" && kind != "component_instance" {
				continue
			}
			t.Run(kind+"/"+scenario, func(t *testing.T) {
				server, api := newFakeReleasesAPI(t)
				artifact := api.seed("artifacts", "artifact", nil)
				target := api.seed("targets", "target", map[string]any{"labels": map[string]string{}})
				component := api.seed("components", "component", map[string]any{"artifact": map[string]string{"id": artifact}})
				route := map[string]string{"deployment_artifact": "artifacts", "target": "targets", "component": "components", "component_instance": "component-instances"}[kind]
				extra, configExtra := map[string]any{}, ""
				switch kind {
				case "target":
					extra["labels"] = map[string]string{"old": "remove"}
					configExtra = `labels = { new = "keep" }`
				case "component":
					extra["artifact"] = map[string]string{"id": artifact}
					configExtra = fmt.Sprintf("artifact_uuid = %q", artifact)
					if scenario == "wrong-binding" {
						extra["artifact"] = map[string]string{"id": uuid.NewString()}
					}
				case "component_instance":
					extra["target_id"], extra["component_id"] = target, component
					configExtra = fmt.Sprintf("target_uuid = %q\ncomponent_uuid = %q", target, component)
					if scenario == "wrong-binding" {
						extra["component_id"] = uuid.NewString()
					}
				}
				if scenario == "archived-uppercase-opt-in" {
					for _, id := range []string{artifact, component, target} {
						configExtra = strings.ReplaceAll(configExtra, id, strings.ToUpper(id))
					}
				}
				archived := strings.HasPrefix(scenario, "archived") || scenario == "wrong-binding"
				if archived {
					extra["archived_at"] = "2026-11-18T00:00:00Z"
				}
				id := ""
				if scenario != "absent-opt-in" {
					id = api.seed(route, "registration", extra)
				}
				optIn := strings.HasSuffix(scenario, "opt-in") || scenario == "wrong-binding"
				config := fakeWorkloadIdentityProvider(server) + fmt.Sprintf(`
resource "buildkite_%s" "test" {
  name = "registration"
  allow_unarchive_on_create = %t
  %s
}`, kind, optIn, configExtra)
				step := resource.TestStep{Config: config}
				if scenario != "archived-opt-in" && scenario != "absent-opt-in" && scenario != "archived-uppercase-opt-in" {
					step.ExpectError = regexp.MustCompile("Unable to create registration|Registration already exists|Archived registration binding mismatch")
				} else {
					step.ConfigPlanChecks = resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}
					step.Check = func(s *terraform.State) error {
						got := s.RootModule().Resources["buildkite_"+kind+".test"].Primary.Attributes["uuid"]
						if id != "" && got != id {
							return fmt.Errorf("restore changed UUID %s to %s", id, got)
						}
						api.mu.Lock()
						defer api.mu.Unlock()
						if api.records[route+"/"+got]["archived_at"] != nil {
							return fmt.Errorf("registration is still archived")
						}
						return nil
					}
				}
				resource.UnitTest(t, resource.TestCase{ProtoV6ProviderFactories: protoV6ProviderFactories(), Steps: []resource.TestStep{step}})
				if id != "" && step.ExpectError != nil {
					api.mu.Lock()
					defer api.mu.Unlock()
					if (api.records[route+"/"+id]["archived_at"] != nil) != archived {
						t.Fatal("failed create changed the registration's archived state")
					}
				}
			})
		}
	}
}

func TestReleasesReadAndArchiveErrors(t *testing.T) {
	for _, test := range []struct {
		name    string
		status  int
		body    string
		absent  bool
		wantErr bool
	}{
		{name: "missing", status: 404, absent: true},
		{name: "archived", status: 200, body: `{"id":"uuid","name":"example","archived_at":"2026-11-18T00:00:00Z"}`, absent: true},
		{name: "forbidden", status: 403, wantErr: true},
		{name: "server error", status: 500, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.URL.Path != "/v2/organizations/acme/artifacts/by-id/uuid" && req.URL.Path != "/v2/organizations/acme/artifacts/by-id/uuid/archive" {
					t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			r := newDeploymentArtifactResource().(*releasesResource)
			r.client = NewClient(&clientConfig{org: "acme", restURL: server.URL, maxRetries: 0})
			ctx := context.Background()
			sch := resourceSchema(ctx, t, r)
			state := tfsdk.State{Schema: sch, Raw: nullObjectWith(ctx, t, sch.Type(), map[string]tftypes.Value{
				"id": tftypes.NewValue(tftypes.String, "graphql-id"), "uuid": tftypes.NewValue(tftypes.String, "uuid"),
				"name": tftypes.NewValue(tftypes.String, "example"), "allow_unarchive_on_create": tftypes.NewValue(tftypes.Bool, false),
			})}
			read := frameworkresource.ReadResponse{State: state}
			r.Read(ctx, frameworkresource.ReadRequest{State: state}, &read)
			if read.Diagnostics.HasError() != test.wantErr || read.State.Raw.IsNull() != test.absent {
				t.Fatalf("read diagnostics %v, state %v", read.Diagnostics, read.State.Raw)
			}
			if test.wantErr && !read.State.Raw.Equal(state.Raw) {
				t.Fatal("failed refresh changed state")
			}
			deleted := frameworkresource.DeleteResponse{State: state}
			r.Delete(ctx, frameworkresource.DeleteRequest{State: state}, &deleted)
			if deleted.Diagnostics.HasError() != test.wantErr {
				t.Fatalf("archive diagnostics %v", deleted.Diagnostics)
			}
		})
	}
}

func TestReleasesImmutableBindings(t *testing.T) {
	for _, binding := range []string{"artifact_uuid", "component_uuid", "target_uuid"} {
		t.Run(binding, func(t *testing.T) {
			server, api := newFakeReleasesAPI(t)
			artifact := api.seed("artifacts", "artifact", nil)
			component := api.seed("components", "component", map[string]any{"artifact": map[string]string{"id": artifact}})
			target := api.seed("targets", "target", map[string]any{"labels": map[string]string{}})
			kind := "component_instance"
			extra := fmt.Sprintf("component_uuid = %q\ntarget_uuid = %q", component, target)
			old := component
			if binding == "target_uuid" {
				old = target
			}
			if binding == "artifact_uuid" {
				kind, old = "component", artifact
				extra = fmt.Sprintf("artifact_uuid = %q", artifact)
			}
			config := fakeWorkloadIdentityProvider(server) + fmt.Sprintf(`
resource "buildkite_%s" "test" {
  name = "registration"
  %s
}`, kind, extra)
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{
					{Config: config},
					{Config: strings.Replace(config, old, uuid.NewString(), 1), PlanOnly: true, ExpectNonEmptyPlan: true,
						ConfigPlanChecks: resource.ConfigPlanChecks{PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectResourceAction("buildkite_"+kind+".test", plancheck.ResourceActionDestroyBeforeCreate)}}},
				},
			})
		})
	}
}

func TestReleasesTargetRestoreDoesNotRestoreInstances(t *testing.T) {
	server, api := newFakeReleasesAPI(t)
	archived := "2026-11-18T00:00:00Z"
	target := api.seed("targets", "target", map[string]any{"labels": map[string]string{}, "archived_at": archived})
	instance := api.seed("component-instances", "unmanaged", map[string]any{"target_id": target, "component_id": uuid.NewString(), "archived_at": archived})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{{Config: fakeWorkloadIdentityProvider(server) + `
resource "buildkite_target" "test" {
  name = "target"
  allow_unarchive_on_create = true
}`, Check: func(s *terraform.State) error {
			api.mu.Lock()
			defer api.mu.Unlock()
			if api.records["targets/"+target]["archived_at"] != nil || api.records["component-instances/"+instance]["archived_at"] != archived {
				return fmt.Errorf("Target restoration cascaded to an unmanaged Instance")
			}
			return nil
		}}},
	})
}
