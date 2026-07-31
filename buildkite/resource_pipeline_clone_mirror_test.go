package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestPipelineResourceCloneMirrorURLSchema(t *testing.T) {
	var response frameworkresource.SchemaResponse
	(&pipelineResource{}).Schema(context.Background(), frameworkresource.SchemaRequest{}, &response)

	attribute, ok := response.Schema.Attributes["clone_mirror_url"]
	if !ok {
		t.Fatal("expected clone_mirror_url in pipeline resource schema")
	}

	stringAttribute, ok := attribute.(schema.StringAttribute)
	if !ok {
		t.Fatalf("expected clone_mirror_url to be a string attribute, got %T", attribute)
	}
	if !stringAttribute.Optional {
		t.Fatal("expected clone_mirror_url to be optional")
	}
}

func TestSetPipelineModelCloneMirrorURL(t *testing.T) {
	cloneMirrorURL := "https://cache.example.com/repository.git"
	data := PipelineFields{
		CloneMirrorUrl: &cloneMirrorURL,
	}
	var model pipelineResourceModel

	setPipelineModel(&model, &data)

	if !model.CloneMirrorUrl.Equal(types.StringValue(cloneMirrorURL)) {
		t.Fatalf("expected clone mirror URL %q, got %s", cloneMirrorURL, model.CloneMirrorUrl)
	}

	data.CloneMirrorUrl = nil
	setPipelineModel(&model, &data)
	if !model.CloneMirrorUrl.IsNull() {
		t.Fatalf("expected a null clone mirror URL, got %s", model.CloneMirrorUrl)
	}
}

func TestPipelineUpdateInputClearsCloneMirrorURLWithNull(t *testing.T) {
	payload, err := json.Marshal(PipelineUpdateInput{})
	if err != nil {
		t.Fatalf("failed to marshal pipeline update input: %v", err)
	}

	if !strings.Contains(string(payload), `"cloneMirrorUrl":null`) {
		t.Fatalf("expected cloneMirrorUrl null in update payload, got %s", payload)
	}
}

func TestAccBuildkitePipelineCloneMirrorURL(t *testing.T) {
	cloneMirrorURL := os.Getenv("BUILDKITE_TEST_CLONE_MIRROR_URL")
	if cloneMirrorURL == "" {
		t.Skip("BUILDKITE_TEST_CLONE_MIRROR_URL must be set for clone mirror acceptance tests")
	}

	pipelineName := acctest.RandString(12)
	config := func(includeCloneMirror bool) string {
		cloneMirror := ""
		if includeCloneMirror {
			cloneMirror = fmt.Sprintf("clone_mirror_url = %q", cloneMirrorURL)
		}

		return fmt.Sprintf(`
			resource "buildkite_pipeline" "pipeline" {
				name = %q
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
				%s
			}

			data "buildkite_pipeline" "pipeline" {
				slug = buildkite_pipeline.pipeline.slug
			}
		`, pipelineName, cloneMirror)
	}
	checkRemoteCloneMirror := func(expected *string) resource.TestCheckFunc {
		return func(_ *terraform.State) error {
			slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), pipelineName)
			response, err := getPipeline(context.Background(), genqlientGraphql, slug)
			if err != nil {
				return err
			}

			actual := response.Pipeline.CloneMirrorUrl
			if expected == nil && actual == nil {
				return nil
			}
			if expected == nil || actual == nil || *expected != *actual {
				return fmt.Errorf("expected clone mirror URL %v, got %v", expected, actual)
			}
			return nil
		}
	}

	resource.ParallelTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckPipelineDestroyFunc,
		Steps: []resource.TestStep{
			{
				Config: config(true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("buildkite_pipeline.pipeline", "clone_mirror_url", cloneMirrorURL),
					resource.TestCheckResourceAttr("data.buildkite_pipeline.pipeline", "clone_mirror_url", cloneMirrorURL),
					checkRemoteCloneMirror(&cloneMirrorURL),
				),
			},
			{
				ResourceName:      "buildkite_pipeline.pipeline",
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				Config: config(false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("buildkite_pipeline.pipeline", "clone_mirror_url"),
					resource.TestCheckNoResourceAttr("data.buildkite_pipeline.pipeline", "clone_mirror_url"),
					checkRemoteCloneMirror(nil),
				),
			},
		},
	})
}
