package buildkite

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeClusterAgentImageAPI serves the agent-images endpoints for one cluster. Images report BUILDING
// for their first buildingReads reads, then finish building, failing with buildError when it is set.
type fakeClusterAgentImageAPI struct {
	t                 *testing.T
	mu                sync.Mutex
	images            map[string]*clusterAgentImage
	reads             map[string]int
	buildingReads     int
	buildError        string
	inUse             bool
	disappearOnDelete bool
	created           int
}

const fakeClusterAgentImageCluster = "0190a0f0-0000-7000-8000-000000000001"

func newFakeClusterAgentImageAPI(t *testing.T) (*httptest.Server, *fakeClusterAgentImageAPI) {
	t.Helper()

	api := &fakeClusterAgentImageAPI{t: t, images: map[string]*clusterAgentImage{}, reads: map[string]int{}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	return server, api
}

func (a *fakeClusterAgentImageAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	prefix := "/v2/organizations/acme/clusters/" + fakeClusterAgentImageCluster + "/agent-images"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
		return
	}
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, prefix), "/")

	switch {
	case r.Method == http.MethodPost && id == "":
		var body struct{ Name, Dockerfile string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			a.t.Errorf("decoding create body: %v", err)
		}
		for _, image := range a.images {
			if image.Name == body.Name {
				writeFakeJSON(w, http.StatusConflict, map[string]string{"message": "an agent image with that name already exists"})
				return
			}
		}
		a.created++
		image := &clusterAgentImage{ID: fmt.Sprintf("img%d", a.created), Name: body.Name, Dockerfile: body.Dockerfile, Status: "BUILDING"}
		a.images[image.ID] = image
		writeFakeJSON(w, http.StatusCreated, image)
	case r.Method == http.MethodGet && id == "":
		images := make([]*clusterAgentImage, 0, len(a.images))
		for _, image := range a.images {
			images = append(images, image)
		}
		writeFakeJSON(w, http.StatusOK, images)
	case r.Method == http.MethodGet && id != "":
		image, ok := a.images[id]
		if !ok {
			writeFakeJSON(w, http.StatusNotFound, map[string]string{"message": "No agent image found with the given ID."})
			return
		}
		a.reads[id]++
		if image.Status == "BUILDING" && a.reads[id] > a.buildingReads {
			if a.buildError != "" {
				image.Status, image.LastBuildError = "FAILED", a.buildError
			} else {
				image.Status, image.ImageRef = "READY", "nscr.io/acme/"+id+":v0"
			}
		}
		writeFakeJSON(w, http.StatusOK, image)
	case r.Method == http.MethodDelete && id != "":
		if a.disappearOnDelete {
			delete(a.images, id)
		}
		if _, ok := a.images[id]; !ok {
			writeFakeJSON(w, http.StatusNotFound, map[string]string{"message": "No agent image found with the given ID."})
			return
		}
		if a.inUse {
			writeFakeJSON(w, http.StatusConflict, map[string]string{"message": "this agent image is in use by one or more queues; reassign those queues before deleting it"})
			return
		}
		delete(a.images, id)
		w.WriteHeader(http.StatusNoContent)
	default:
		a.t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writeFakeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (a *fakeClusterAgentImageAPI) setBuildError(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.buildError = message
}

func (a *fakeClusterAgentImageAPI) setInUse(inUse bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.inUse = inUse
}

// checkImages asserts the names of the images left in the cluster
func (a *fakeClusterAgentImageAPI) checkImages(want ...string) func(*terraform.State) error {
	return func(*terraform.State) error {
		a.mu.Lock()
		defer a.mu.Unlock()

		var got []string
		for _, image := range a.images {
			got = append(got, image.Name+" "+image.Dockerfile)
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			return fmt.Errorf("images in the cluster = %q, want %q", got, want)
		}
		return nil
	}
}

func fakeClusterAgentImageConfig(server *httptest.Server, dockerfile string) string {
	return fmt.Sprintf(`
	provider "buildkite" {
		organization = "acme"
		api_token    = "fake"
		rest_url     = %q
		graphql_url  = %q
	}

	resource "buildkite_cluster_agent_image" "ruby" {
		cluster_id = %q
		name       = "ruby"
		dockerfile = %q
	}
	`, server.URL, server.URL+"/graphql", fakeClusterAgentImageCluster, dockerfile)
}

func TestUnitBuildkiteClusterAgentImageWaitsForItsBuild(t *testing.T) {
	server, api := newFakeClusterAgentImageAPI(t)
	api.buildingReads = 2

	const name = "buildkite_cluster_agent_image.ruby"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkImages(),
		Steps: []resource.TestStep{
			{
				Config: fakeClusterAgentImageConfig(server, "RUN apt-get install -y ruby\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", "img1"),
					// the create response is still building and has no image ref yet
					resource.TestCheckResourceAttr(name, "status", "READY"),
					resource.TestCheckResourceAttr(name, "image_ref", "nscr.io/acme/img1:v0"),
					resource.TestCheckResourceAttr(name, "dockerfile", "RUN apt-get install -y ruby\n"),
				),
			},
			{
				ResourceName:            name,
				ImportState:             true,
				ImportStateId:           fakeClusterAgentImageCluster + "/img1",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				ResourceName:            name,
				ImportState:             true,
				ImportStateId:           fakeClusterAgentImageCluster + "/ruby",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				// there is no update endpoint, so a new Dockerfile is a new image
				Config: fakeClusterAgentImageConfig(server, "RUN apt-get install -y ruby-full\n"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "image_ref", "nscr.io/acme/img2:v0"),
					api.checkImages("ruby RUN apt-get install -y ruby-full\n"),
				),
			},
		},
	})
}

func TestUnitBuildkiteClusterAgentImageImportResolution(t *testing.T) {
	const images = `[{"id":"img2","name":"img1"},{"id":"img1","name":"ruby / prod"}]`
	for _, tc := range []struct {
		name       string
		identifier string
		status     int
		wantID     string
		wantError  string
	}{
		{name: "ID takes precedence over name", identifier: "img1", status: http.StatusOK, wantID: "img1"},
		{name: "name containing spaces and slash", identifier: "ruby / prod", status: http.StatusOK, wantID: "img1"},
		{name: "case-sensitive name", identifier: "Ruby / prod", status: http.StatusOK, wantError: "No cluster agent image"},
		{name: "missing image", identifier: "missing", status: http.StatusOK, wantError: "No cluster agent image"},
		{name: "forbidden", identifier: "ruby", status: http.StatusForbidden, wantError: "403"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.Method != http.MethodGet || req.URL.Path != "/v2/organizations/acme/clusters/"+fakeClusterAgentImageCluster+"/agent-images" {
					t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				if tc.status == http.StatusOK {
					_, _ = w.Write([]byte(images))
				} else {
					_, _ = w.Write([]byte(`{"message":"access denied"}`))
				}
			}))
			defer server.Close()
			r := &clusterAgentImageResource{client: NewClient(&clientConfig{org: "acme", restURL: server.URL})}
			ctx := t.Context()
			schema := resourceSchema(ctx, t, r)
			state := tfsdk.State{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), nil)}
			resp := fwresource.ImportStateResponse{State: state}
			r.ImportState(ctx, fwresource.ImportStateRequest{ID: fakeClusterAgentImageCluster + "/" + tc.identifier}, &resp)
			if tc.wantError != "" {
				if !resp.Diagnostics.HasError() || !diagnosticsContain(resp.Diagnostics, tc.wantError) || !resp.State.Raw.Equal(state.Raw) {
					t.Fatalf("want import error %q without state, got %v, state: %s", tc.wantError, resp.Diagnostics, resp.State.Raw)
				}
				return
			}
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var imported clusterAgentImageResourceModel
			if diags := resp.State.Get(ctx, &imported); diags.HasError() {
				t.Fatal(diags)
			}
			if imported.ID.ValueString() != tc.wantID || imported.ClusterID.ValueString() != fakeClusterAgentImageCluster {
				t.Fatalf("imported ID = %q, cluster = %q; want %q, %q", imported.ID.ValueString(), imported.ClusterID.ValueString(), tc.wantID, fakeClusterAgentImageCluster)
			}
		})
	}
}

func TestUnitBuildkiteClusterAgentImageReplacesAFailedBuild(t *testing.T) {
	server, api := newFakeClusterAgentImageAPI(t)
	api.buildError = "dockerfile step 3 exited with status 1"

	const name = "buildkite_cluster_agent_image.ruby"
	config := fakeClusterAgentImageConfig(server, "RUN apt-get install -y ruby\n")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkImages(),
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`dockerfile\s+step 3 exited with status 1`),
			},
			{
				// the failed image was kept in state, so it is replaced rather than left holding its name
				PreConfig: func() { api.setBuildError("") },
				Config:    config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", "img2"),
					resource.TestCheckResourceAttr(name, "status", "READY"),
					api.checkImages("ruby RUN apt-get install -y ruby\n"),
				),
			},
		},
	})
}

func TestUnitBuildkiteClusterAgentImageReplacesATimedOutBuild(t *testing.T) {
	server, api := newFakeClusterAgentImageAPI(t)
	api.buildingReads = 1_000_000

	const name = "buildkite_cluster_agent_image.ruby"
	config := strings.Replace(fakeClusterAgentImageConfig(server, "RUN true\n"),
		"dockerfile =", "timeouts { create = \"2s\" }\n dockerfile =", 1)

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkImages(),
		Steps: []resource.TestStep{
			{
				Config:      config,
				ExpectError: regexp.MustCompile(`Cluster agent image img1 did not build`),
			},
			{
				PreConfig: func() {
					api.mu.Lock()
					defer api.mu.Unlock()
					api.buildingReads = 0
				},
				Config: strings.Replace(config, `create = "2s"`, `create = "30s"`, 1),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction(name, plancheck.ResourceActionDestroyBeforeCreate)},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "id", "img2"),
					resource.TestCheckResourceAttr(name, "status", "READY"),
					api.checkImages("ruby RUN true\n"),
				),
			},
		},
	})
}

func TestUnitBuildkiteClusterAgentImageRejectsNonPositiveBuildTimeout(t *testing.T) {
	for _, timeout := range []string{"0s", "-1h"} {
		t.Run(timeout, func(t *testing.T) {
			server, api := newFakeClusterAgentImageAPI(t)
			config := strings.Replace(fakeClusterAgentImageConfig(server, "RUN true\n"),
				"dockerfile =", fmt.Sprintf("timeouts { create = %q }\n dockerfile =", timeout), 1)

			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: protoV6ProviderFactories(),
				CheckDestroy:             api.checkImages(),
				Steps: []resource.TestStep{{
					Config:      config,
					ExpectError: regexp.MustCompile(`create timeout must be greater than zero`),
				}},
			})

			api.mu.Lock()
			defer api.mu.Unlock()
			if api.created != 0 {
				t.Errorf("created %d images with an invalid build timeout, want none", api.created)
			}
		})
	}
}

func TestUnitBuildkiteClusterAgentImageDeletesAnAlreadyAbsentImage(t *testing.T) {
	server, api := newFakeClusterAgentImageAPI(t)
	// The image is present during refresh, but disappears before DELETE can find it.
	api.disappearOnDelete = true
	config := fakeClusterAgentImageConfig(server, "RUN true\n")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkImages(),
		Steps: []resource.TestStep{
			{Config: config},
			{Config: config, Destroy: true},
		},
	})
}

func TestUnitBuildkiteClusterAgentImageRetainsAnImageInUse(t *testing.T) {
	server, api := newFakeClusterAgentImageAPI(t)
	config := fakeClusterAgentImageConfig(server, "RUN true\n")

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkImages(),
		Steps: []resource.TestStep{
			{Config: config},
			{
				PreConfig:   func() { api.setInUse(true) },
				Config:      config,
				Destroy:     true,
				ExpectError: regexp.MustCompile(`reassign those queues before deleting it`),
			},
			{
				PreConfig: func() { api.setInUse(false) },
				Config:    config,
				Check:     resource.TestCheckResourceAttr("buildkite_cluster_agent_image.ruby", "id", "img1"),
			},
			{Config: config, Destroy: true},
		},
	})
}

func TestAccBuildkiteClusterAgentImage(t *testing.T) {
	clusterName := testAccResourceName()
	imageName := testAccResourceName()

	config := fmt.Sprintf(`
	resource "buildkite_cluster" "hosted" {
		name = "%s"
	}

	# a cluster can hold images once it has a hosted queue
	resource "buildkite_cluster_queue" "default" {
		cluster_id = buildkite_cluster.hosted.id
		key        = "${buildkite_cluster.hosted.name}-default"
		hosted_agents = {
			instance_shape = "LINUX_AMD64_2X4"
		}
	}

	resource "buildkite_cluster_agent_image" "image" {
		cluster_id = buildkite_cluster.hosted.uuid
		name       = "%s"
		dockerfile = "RUN echo hello > /tmp/hello\n"
		depends_on = [buildkite_cluster_queue.default]
	}

	data "buildkite_cluster_agent_image" "by_id" {
		cluster_id = buildkite_cluster.hosted.uuid
		id         = buildkite_cluster_agent_image.image.id
	}

	data "buildkite_cluster_agent_image" "by_name" {
		cluster_id = buildkite_cluster.hosted.uuid
		name       = buildkite_cluster_agent_image.image.name
	}

	resource "buildkite_cluster_queue" "custom" {
		cluster_id = buildkite_cluster.hosted.id
		key        = "${buildkite_cluster.hosted.name}-custom"
		hosted_agents = {
			instance_shape = "LINUX_AMD64_2X4"
			linux = {
				agent_image_ref = buildkite_cluster_agent_image.image.image_ref
			}
		}
	}
	`, clusterName, imageName)

	const name = "buildkite_cluster_agent_image.image"

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: config,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(name, "status", "READY"),
					resource.TestCheckResourceAttr(name, "dockerfile", "RUN echo hello > /tmp/hello\n"),
					resource.TestCheckResourceAttrPair("buildkite_cluster_queue.custom", "hosted_agents.linux.agent_image_ref", name, "image_ref"),
					resource.TestCheckResourceAttrPair("data.buildkite_cluster_agent_image.by_id", "id", name, "id"),
					resource.TestCheckResourceAttrPair("data.buildkite_cluster_agent_image.by_id", "name", name, "name"),
					resource.TestCheckResourceAttrPair("data.buildkite_cluster_agent_image.by_id", "image_ref", name, "image_ref"),
					resource.TestCheckResourceAttr("data.buildkite_cluster_agent_image.by_id", "status", "READY"),
					resource.TestCheckResourceAttr("data.buildkite_cluster_agent_image.by_id", "dockerfile", "RUN echo hello > /tmp/hello\n"),
					resource.TestCheckResourceAttrPair("data.buildkite_cluster_agent_image.by_name", "id", name, "id"),
					resource.TestCheckResourceAttrPair("data.buildkite_cluster_agent_image.by_name", "image_ref", name, "image_ref"),
					resource.TestCheckResourceAttr("data.buildkite_cluster_agent_image.by_name", "status", "READY"),
					resource.TestCheckResourceAttr("data.buildkite_cluster_agent_image.by_name", "dockerfile", "RUN echo hello > /tmp/hello\n"),
				),
			},
			{
				ResourceName: name,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					image := s.RootModule().Resources[name].Primary
					return image.Attributes["cluster_id"] + "/" + image.ID, nil
				},
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				ResourceName: name,
				ImportState:  true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					image := s.RootModule().Resources[name].Primary
					return image.Attributes["cluster_id"] + "/" + image.Attributes["name"], nil
				},
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
		},
	})
}
