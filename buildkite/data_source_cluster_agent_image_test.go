package buildkite

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestUnitClusterAgentImageDatasource(t *testing.T) {
	const ready = `{"id":"ruby-id","name":"ruby","dockerfile":"RUN ruby --version\n","status":"READY","image_ref":"nscr.io/acme/ruby-id:v3"}`
	for _, tc := range []struct {
		name       string
		selector   string
		body       string
		statusCode int
		wantStatus string
		wantRef    string
		wantError  string
	}{
		{
			name:       "by ID",
			selector:   `id = "ruby-id"`,
			body:       ready,
			wantStatus: "READY",
			wantRef:    "nscr.io/acme/ruby-id:v3",
		},
		{
			name:       "by exact name",
			selector:   `name = "ruby"`,
			body:       `[{"id":"other-id","name":"Ruby","status":"READY","image_ref":"wrong-image"},` + ready + `]`,
			wantStatus: "READY",
			wantRef:    "nscr.io/acme/ruby-id:v3",
		},
		{
			name:       "building image is not polled",
			selector:   `id = "ruby-id"`,
			body:       `{"id":"ruby-id","name":"ruby","dockerfile":"RUN ruby --version\n","status":"BUILDING","image_ref":""}`,
			wantStatus: "BUILDING",
		},
		{
			name:       "failed image is readable",
			selector:   `name = "ruby"`,
			body:       `[{"id":"ruby-id","name":"ruby","dockerfile":"RUN ruby --version\n","status":"FAILED","image_ref":"","last_build_error":"build failed"}]`,
			wantStatus: "FAILED",
		},
		{
			name:       "missing ID",
			selector:   `id = "ruby-id"`,
			body:       `{"message":"No agent image found with the given ID."}`,
			statusCode: http.StatusNotFound,
			wantError:  "No agent image found with the given ID",
		},
		{
			name:      "missing name",
			selector:  `name = "ruby"`,
			body:      `[{"id":"other-id","name":"Ruby"}]`,
			wantError: `Could not find.*ruby`,
		},
		{
			name:      "cluster without images",
			selector:  `name = "ruby"`,
			body:      `[]`,
			wantError: `Could not find.*ruby`,
		},
		{
			name:       "list permission denied",
			selector:   `name = "ruby"`,
			body:       `{"message":"permission to manage this cluster is required"}`,
			statusCode: http.StatusForbidden,
			wantError:  "permission to manage this cluster is required",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path := "/v2/organizations/acme/clusters/" + fakeClusterAgentImageCluster + "/agent-images"
				if tc.selector == `id = "ruby-id"` {
					path += "/ruby-id"
				}
				if r.Method != http.MethodGet || r.URL.Path != path {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if tc.statusCode != 0 {
					w.WriteHeader(tc.statusCode)
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()

			step := resource.TestStep{
				Config: fmt.Sprintf(`
					provider "buildkite" {
						organization = "acme"
						api_token = "test"
						rest_url = %q
						graphql_url = %q
					}
					data "buildkite_cluster_agent_image" "ruby" {
						cluster_id = %q
						%s
					}
				`, server.URL, server.URL+"/graphql", fakeClusterAgentImageCluster, tc.selector),
			}
			if tc.wantError != "" {
				step.ExpectError = regexp.MustCompile(tc.wantError)
			} else {
				const name = "data.buildkite_cluster_agent_image.ruby"
				step.Check = resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", "ruby-id"),
					resource.TestCheckResourceAttr(name, "name", "ruby"),
					resource.TestCheckResourceAttr(name, "cluster_id", fakeClusterAgentImageCluster),
					resource.TestCheckResourceAttr(name, "dockerfile", "RUN ruby --version\n"),
					resource.TestCheckResourceAttr(name, "status", tc.wantStatus),
					resource.TestCheckResourceAttr(name, "image_ref", tc.wantRef),
				)
			}
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps:                    []resource.TestStep{step},
			})
		})
	}
}

func TestUnitClusterAgentImageDatasourceSelectors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selectors string
		wantError string
	}{
		{name: "neither", wantError: "Invalid Attribute Combination"},
		{name: "both", selectors: `id = "ruby-id"
			name = "ruby"`, wantError: "Invalid Attribute Combination"},
		{name: "empty ID", selectors: `id = ""`, wantError: "Invalid Attribute Value Length"},
		{name: "empty name", selectors: `name = ""`, wantError: "Invalid Attribute Value Length"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				Steps: []resource.TestStep{{
					Config: fmt.Sprintf(`
						provider "buildkite" {
							organization = "acme"
							api_token = "test"
						}
						data "buildkite_cluster_agent_image" "ruby" {
							cluster_id = %q
							%s
						}
					`, fakeClusterAgentImageCluster, tc.selectors),
					ExpectError: regexp.MustCompile(tc.wantError),
				}},
			})
		})
	}
}
