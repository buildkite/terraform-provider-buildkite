package buildkite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// newPaginatedPortalsAPI serves total portals from the list endpoint, pageSize at a time, with
// the Link header the REST API uses for pagination. Each page is delayed by pageDelay.
func newPaginatedPortalsAPI(t *testing.T, total, pageSize int, pageDelay time.Duration) *httptest.Server {
	t.Helper()

	const path = "/v2/organizations/acme/portals"

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != path {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}

		select {
		case <-time.After(pageDelay):
		case <-r.Context().Done():
			return
		}

		page := 1
		if raw := r.URL.Query().Get("page"); raw != "" {
			page, _ = strconv.Atoi(raw)
		}
		start := (page - 1) * pageSize
		end := min(start+pageSize, total)

		portals := []portalAPIResponse{}
		for i := start; i < end; i++ {
			portals = append(portals, portalAPIResponse{
				UUID:      fmt.Sprintf("0199e1c2-0000-7000-8000-%012d", i+1),
				Slug:      fmt.Sprintf("portal-%d", i+1),
				Name:      fmt.Sprintf("Portal %d", i+1),
				Query:     "{ viewer { user { name } } }",
				CreatedAt: "2026-01-01T00:00:00.000Z",
			})
		}

		if end < total {
			w.Header().Set("Link", fmt.Sprintf(`<%s%s?page=%d&per_page=%d>; rel="next", <%s%s?page=%d&per_page=%d>; rel="last"`,
				server.URL, path, page+1, pageSize, server.URL, path, (total+pageSize-1)/pageSize, pageSize))
		}
		if err := json.NewEncoder(w).Encode(portals); err != nil {
			t.Errorf("unable to write the response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return server
}

func TestUnitBuildkitePortalsDataSourceReadsEveryPage(t *testing.T) {
	server := newPaginatedPortalsAPI(t, 65, 30, 0)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
				provider "buildkite" {
					organization = "acme"
					api_token    = "fake"
					rest_url     = %q
				}

				data "buildkite_portals" "all" {}
				`, server.URL),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.buildkite_portals.all", "portals.#", "65"),
					resource.TestCheckResourceAttr("data.buildkite_portals.all", "portals.0.slug", "portal-1"),
					resource.TestCheckResourceAttr("data.buildkite_portals.all", "portals.64.slug", "portal-65"),
				),
			},
		},
	})
}

func TestUnitBuildkitePortalsDataSourceSharesReadTimeoutAcrossPages(t *testing.T) {
	// Three pages of 600ms each fit in a 1s budget per page but not in one 1s budget for the
	// whole read.
	server := newPaginatedPortalsAPI(t, 3, 1, 600*time.Millisecond)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
				provider "buildkite" {
					organization = "acme"
					api_token    = "fake"
					rest_url     = %q
					max_retries  = 0
					timeouts     = { read = "1s" }
				}

				data "buildkite_portals" "all" {}
				`, server.URL),
				ExpectError: regexp.MustCompile(`context deadline exceeded`),
			},
		},
	})
}

func TestNextPageLink(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{header: "", want: ""},
		{
			header: `<https://api.buildkite.com/v2/organizations/acme/portals?page=2&per_page=30>; rel="next", <https://api.buildkite.com/v2/organizations/acme/portals?page=3&per_page=30>; rel="last"`,
			want:   "https://api.buildkite.com/v2/organizations/acme/portals?page=2&per_page=30",
		},
		{
			header: `<https://api.buildkite.com/v2/organizations/acme/portals?page=1&per_page=30>; rel="first", <https://api.buildkite.com/v2/organizations/acme/portals?page=2&per_page=30>; rel="prev"`,
			want:   "",
		},
	}

	for _, tt := range tests {
		if got := nextPageLink(tt.header); got != tt.want {
			t.Errorf("nextPageLink(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}
