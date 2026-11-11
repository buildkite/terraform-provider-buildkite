package buildkite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
)

// fakePortalAPI serves the portal endpoints against portals held in memory. An update only
// changes the keys its request carries, so a field the resource leaves out of the request keeps
// its previous value, the same as the API does.
type fakePortalAPI struct {
	t       *testing.T
	mu      sync.Mutex
	portals map[string]*portalAPIResponse
}

func newFakePortalAPI(t *testing.T) *httptest.Server {
	t.Helper()

	api := &fakePortalAPI{t: t, portals: map[string]*portalAPIResponse{}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	return server
}

func (a *fakePortalAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	const base = "/v2/organizations/acme/portals"

	a.mu.Lock()
	defer a.mu.Unlock()

	if r.URL.Path == base && r.Method == http.MethodPost {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			a.t.Errorf("unable to read the request: %v", err)
			http.Error(w, `{"message":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		slug, _ := body["slug"].(string)
		portal := &portalAPIResponse{
			UUID:      "0199e1c2-0000-7000-8000-000000000001",
			Slug:      slug,
			CreatedAt: "2026-01-01T00:00:00.000Z",
			Token:     ptr("bkpt_fake"),
		}
		a.apply(portal, body)
		a.portals[slug] = portal
		a.write(w, portal)
		return
	}

	slug, found := strings.CutPrefix(r.URL.Path, base+"/")
	portal, exists := a.portals[slug]
	if !found || !exists {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.write(w, a.withoutToken(portal))
	case http.MethodPut:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			a.t.Errorf("unable to read the request: %v", err)
			http.Error(w, `{"message":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		a.apply(portal, body)
		a.write(w, a.withoutToken(portal))
	case http.MethodDelete:
		delete(a.portals, slug)
		w.WriteHeader(http.StatusNoContent)
	default:
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}
}

func (a *fakePortalAPI) apply(portal *portalAPIResponse, body map[string]any) {
	for key, value := range body {
		switch key {
		case "name":
			portal.Name, _ = value.(string)
		case "slug":
			portal.Slug, _ = value.(string)
		case "query":
			portal.Query, _ = value.(string)
		case "user_invokable":
			portal.UserInvokable, _ = value.(bool)
		case "description":
			portal.Description = optionalStringFromJSON(value)
		case "allowed_ip_addresses":
			portal.AllowedIPAddresses = optionalStringFromJSON(value)
		default:
			a.t.Errorf("unexpected key in the request: %s", key)
		}
	}
}

func (a *fakePortalAPI) withoutToken(portal *portalAPIResponse) *portalAPIResponse {
	copied := *portal
	copied.Token = nil
	return &copied
}

func (a *fakePortalAPI) write(w http.ResponseWriter, portal *portalAPIResponse) {
	if err := json.NewEncoder(w).Encode(portal); err != nil {
		a.t.Errorf("unable to write the response: %v", err)
	}
}

func TestUnitBuildkitePortalResourceClearsOptionalFields(t *testing.T) {
	server := newFakePortalAPI(t)

	config := func(attributes string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			organization = "acme"
			api_token    = "fake"
			rest_url     = %q
		}

		resource "buildkite_portal" "test" {
			slug  = "test-portal"
			name  = "Test Portal"
			query = "{ viewer { user { name } } }"
			%s
		}
		`, server.URL, attributes)
	}

	const name = "buildkite_portal.test"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config(`
					description          = "Test portal description"
					allowed_ip_addresses = "192.0.2.0/24"
				`),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "description", "Test portal description"),
					resource.TestCheckResourceAttr(name, "allowed_ip_addresses", "192.0.2.0/24"),
				),
			},
			{
				// removing the optional fields from the configuration clears them on the portal
				Config: config(``),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(name, "description"),
					resource.TestCheckNoResourceAttr(name, "allowed_ip_addresses"),
				),
			},
		},
	})
}
