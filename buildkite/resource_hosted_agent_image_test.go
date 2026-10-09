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

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeHostedAgentImageAPI serves the agent-images endpoints for one cluster. Images report BUILDING
// for their first buildingReads reads, then finish building, failing with buildError when it is set.
type fakeHostedAgentImageAPI struct {
	t             *testing.T
	mu            sync.Mutex
	images        map[string]*hostedAgentImage
	reads         map[string]int
	buildingReads int
	buildError    string
	created       int
}

const fakeHostedAgentImageCluster = "0190a0f0-0000-7000-8000-000000000001"

func newFakeHostedAgentImageAPI(t *testing.T) (*httptest.Server, *fakeHostedAgentImageAPI) {
	t.Helper()

	api := &fakeHostedAgentImageAPI{t: t, images: map[string]*hostedAgentImage{}, reads: map[string]int{}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	return server, api
}

func (a *fakeHostedAgentImageAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()

	prefix := "/v2/organizations/acme/clusters/" + fakeHostedAgentImageCluster + "/agent-images"
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
		image := &hostedAgentImage{ID: fmt.Sprintf("img%d", a.created), Name: body.Name, Dockerfile: body.Dockerfile, Status: "BUILDING"}
		a.images[image.ID] = image
		writeFakeJSON(w, http.StatusCreated, image)
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
		if _, ok := a.images[id]; !ok {
			writeFakeJSON(w, http.StatusNotFound, map[string]string{"message": "No agent image found with the given ID."})
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

func (a *fakeHostedAgentImageAPI) setBuildError(message string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.buildError = message
}

// checkImages asserts the names of the images left in the cluster
func (a *fakeHostedAgentImageAPI) checkImages(want ...string) func(*terraform.State) error {
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

func fakeHostedAgentImageConfig(server *httptest.Server, dockerfile string) string {
	return fmt.Sprintf(`
	provider "buildkite" {
		organization = "acme"
		api_token    = "fake"
		rest_url     = %q
		graphql_url  = %q
	}

	resource "buildkite_hosted_agent_image" "ruby" {
		cluster_id = %q
		name       = "ruby"
		dockerfile = %q
	}
	`, server.URL, server.URL+"/graphql", fakeHostedAgentImageCluster, dockerfile)
}

func TestUnitBuildkiteHostedAgentImageWaitsForItsBuild(t *testing.T) {
	server, api := newFakeHostedAgentImageAPI(t)
	api.buildingReads = 2

	const name = "buildkite_hosted_agent_image.ruby"

	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             api.checkImages(),
		Steps: []resource.TestStep{
			{
				Config: fakeHostedAgentImageConfig(server, "RUN apt-get install -y ruby\n"),
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
				ImportStateId:           fakeHostedAgentImageCluster + "/img1",
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"timeouts"},
			},
			{
				// there is no update endpoint, so a new Dockerfile is a new image
				Config: fakeHostedAgentImageConfig(server, "RUN apt-get install -y ruby-full\n"),
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

func TestUnitBuildkiteHostedAgentImageReplacesAFailedBuild(t *testing.T) {
	server, api := newFakeHostedAgentImageAPI(t)
	api.buildError = "dockerfile step 3 exited with status 1"

	const name = "buildkite_hosted_agent_image.ruby"
	config := fakeHostedAgentImageConfig(server, "RUN apt-get install -y ruby\n")

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

func TestAccBuildkiteHostedAgentImage(t *testing.T) {
	clusterName := acctest.RandString(12)
	imageName := acctest.RandString(12)

	config := fmt.Sprintf(`
	resource "buildkite_cluster" "hosted" {
		name = "%s"
	}

	# a cluster can hold images once it has a hosted queue
	resource "buildkite_cluster_queue" "default" {
		cluster_id = buildkite_cluster.hosted.id
		key        = "default"
		hosted_agents = {
			instance_shape = "LINUX_AMD64_2X4"
		}
	}

	resource "buildkite_hosted_agent_image" "image" {
		cluster_id = buildkite_cluster.hosted.uuid
		name       = "%s"
		dockerfile = "RUN echo hello > /tmp/hello\n"
		depends_on = [buildkite_cluster_queue.default]
	}

	resource "buildkite_cluster_queue" "custom" {
		cluster_id = buildkite_cluster.hosted.id
		key        = "custom"
		hosted_agents = {
			instance_shape = "LINUX_AMD64_2X4"
			linux = {
				agent_image_ref = buildkite_hosted_agent_image.image.image_ref
			}
		}
	}
	`, clusterName, imageName)

	const name = "buildkite_hosted_agent_image.image"

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
		},
	})
}
