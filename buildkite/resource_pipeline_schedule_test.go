package buildkite

import (
	"context"
	"fmt"
	"testing"

	genqlient "github.com/Khan/genqlient/graphql"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

type fakeGraphQLClient struct {
	handler func(ctx context.Context, req *genqlient.Request, resp *genqlient.Response) error
}

func (f *fakeGraphQLClient) MakeRequest(ctx context.Context, req *genqlient.Request, resp *genqlient.Response) error {
	return f.handler(ctx, req, resp)
}

// TestPipelineScheduleDeleteSurfacesParentPipelineNotFound pins the fix for the customer bug:
// pipelineScheduleDelete returning "No pipeline found" (which it does when the schedule's parent
// pipeline is invisible to the viewer — archived, missing RBAC, token in the wrong org) must
// NOT be reported as a successful destroy. The schedule may still exist upstream.
func TestPipelineScheduleDeleteSurfacesParentPipelineNotFound(t *testing.T) {
	var calls int
	fake := &fakeGraphQLClient{
		handler: func(ctx context.Context, req *genqlient.Request, resp *genqlient.Response) error {
			calls++
			return gqlerror.List{&gqlerror.Error{Message: "No pipeline found"}}
		},
	}
	r := &pipelineSchedule{client: &Client{genqlient: fake}}

	var schemaResp frameworkresource.SchemaResponse
	r.Schema(context.Background(), frameworkresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResp.Diagnostics)
	}

	state := tfsdk.State{Schema: schemaResp.Schema}
	model := pipelineScheduleResourceModel{
		Id:         types.StringValue("UGlwZWxpbmVTY2hlZHVsZS0tLWJhMjNiZWZiLTU1NTctNDI1NC1hZTU1LTc4OGM0Y2I5MjBiOQ=="),
		Uuid:       types.StringValue("ba23befb-5557-4254-ae55-788c4cb920b9"),
		PipelineId: types.StringValue("UGlwZWxpbmUtLS0wMTlmNWRlOC1iNDJhLTQxNDgtYTA5OS03ZWVmM2JjNjNjOTg="),
		Label:      types.StringValue("Nightly build"),
		Cronline:   types.StringValue("@midnight"),
		Branch:     types.StringValue("main"),
		Commit:     types.StringNull(),
		Message:    types.StringNull(),
		Env:        types.MapNull(types.StringType),
		Enabled:    types.BoolValue(true),
	}
	if diags := state.Set(context.Background(), &model); diags.HasError() {
		t.Fatalf("state.Set: %v", diags)
	}

	resp := frameworkresource.DeleteResponse{State: state}
	r.Delete(context.Background(), frameworkresource.DeleteRequest{State: state}, &resp)

	if calls == 0 {
		t.Fatal("Delete did not call the GraphQL client")
	}
	if !resp.Diagnostics.HasError() {
		t.Fatal(`Delete silently swallowed a "No pipeline found" response from pipelineScheduleDelete; the schedule may still exist upstream`)
	}
}

func TestAccBuildkitePipelineSchedule(t *testing.T) {
	config := func(name, cronline, label, env string, enabled bool) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://github.com/buildkite/terraform-provider-buildkite.git"
			}

			resource "buildkite_pipeline_schedule" "pipeline" {
				pipeline_id = buildkite_pipeline.pipeline.id
				branch = "main"
				cronline = "%s"
				label = "%s"
				env = {
					%s
				}
				enabled = %v
			}
		`, name, cronline, label, env, enabled)
	}

	loadPipeline := func(name string, pipeline *getPipelinePipeline) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			slug := fmt.Sprintf("%s/%s", getenv("BUILDKITE_ORGANIZATION_SLUG"), name)
			resp, err := getPipeline(context.Background(), genqlientGraphql, slug)
			*pipeline = resp.Pipeline
			return err
		}
	}
	loadPipelineSchedule := func(schedule *PipelineScheduleValues) resource.TestCheckFunc {
		return func(s *terraform.State) error {
			scheduleRes := s.RootModule().Resources["buildkite_pipeline_schedule.pipeline"]
			resp, err := getPipelineSchedule(context.Background(), genqlientGraphql, scheduleRes.Primary.ID)
			if err != nil {
				return err
			}
			if node, ok := resp.Node.(*getPipelineScheduleNodePipelineSchedule); ok {
				*schedule = node.PipelineScheduleValues
				return nil
			}
			return fmt.Errorf("pipeline schedule node is invalid")
		}
	}

	t.Run("pipeline schedule can be created", func(t *testing.T) {
		var pipeline getPipelinePipeline
		var schedule PipelineScheduleValues
		pipelineName := acctest.RandString(12)
		label := acctest.RandString(12)
		cronline := "0 * * * *"

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineScheduleDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(pipelineName, cronline, label, "FOO = \"BAR=2f\"", true),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Schedules need a pipeline
						loadPipeline(pipelineName, &pipeline),
						// Confirm the schedule exists in the buildkite API
						loadPipelineSchedule(&schedule),
						// Confirm the schedule has the correct values in Buildkite's system
						func(s *terraform.State) error {
							if pipeline.Id != schedule.Pipeline.Id {
								return fmt.Errorf("remote pipeline schedule pipeline ID (%s) doesn't match expected value (%s)", schedule.Pipeline.Id, pipeline.Id)
							}
							l := *schedule.Label
							if l != label {
								return fmt.Errorf("remote pipeline schedule label (%s) doesn't match expected value (%s)", l, label)
							}
							if *schedule.Cronline != cronline {
								return fmt.Errorf("remote pipeline schedule cronline (%s) doesn't match expected value (%s)", *schedule.Cronline, cronline)
							}
							return nil
						},
						// Confirm the pipeline has the correct values in terraform state
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "label", label),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "cronline", cronline),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "branch", "main"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "env.FOO", "BAR=2f"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "enabled", "true"),
					),
				},
				{
					ResourceName:      "buildkite_pipeline_schedule.pipeline",
					ImportState:       true,
					ImportStateId:     schedule.Id,
					ImportStateVerify: true,
				},
				{
					// <pipeline slug>/<schedule uuid> is also accepted
					ResourceName:      "buildkite_pipeline_schedule.pipeline",
					ImportState:       true,
					ImportStateIdFunc: testAccImportIDFromAttributes("buildkite_pipeline.pipeline", "slug", "buildkite_pipeline_schedule.pipeline", "uuid"),
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("pipeline schedule can be updated", func(t *testing.T) {
		var pipeline getPipelinePipeline
		var schedule PipelineScheduleValues
		pipelineName := acctest.RandString(12)
		label := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineScheduleDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(pipelineName, "0 * * * *", label, "FOO = \"bar\"", true),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Schedules need a pipeline
						loadPipeline(pipelineName, &pipeline),
						// Confirm the schedule exists in the buildkite API
						loadPipelineSchedule(&schedule),
						// Confirm the schedule has the correct values in Buildkite's system
						func(s *terraform.State) error {
							if pipeline.Id != schedule.Pipeline.Id {
								return fmt.Errorf("remote pipeline schedule pipeline ID (%s) doesn't match expected value (%s)", schedule.Pipeline.Id, pipeline.Id)
							}
							if *schedule.Label != label {
								return fmt.Errorf("remote pipeline schedule label (%s) doesn't match expected value (%s)", *schedule.Label, label)
							}
							if *schedule.Cronline != "0 * * * *" {
								return fmt.Errorf("remote pipeline schedule cronline (%s) doesn't match expected value (%s)", *schedule.Cronline, "0 * * * *")
							}
							return nil
						},
						// Confirm the pipeline has the correct values in terraform state
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "label", label),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "cronline", "0 * * * *"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "branch", "main"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "env.FOO", "bar"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "enabled", "true"),
					),
				},
				{
					Config: config(pipelineName, "0 1 * * *", label, "FOO = \"bar\"", false),
					Check: resource.ComposeAggregateTestCheckFunc(
						// Confirm the schedule exists in the buildkite API
						loadPipelineSchedule(&schedule),
						// Confirm the schedule has the correct values in Buildkite's system
						func(s *terraform.State) error {
							if pipeline.Id != schedule.Pipeline.Id {
								return fmt.Errorf("remote pipeline schedule pipeline ID (%s) doesn't match expected value (%s)", schedule.Pipeline.Id, pipeline.Id)
							}
							if *schedule.Label != label {
								return fmt.Errorf("remote pipeline schedule label (%s) doesn't match expected value (%s)", *schedule.Label, label)
							}
							if *schedule.Cronline != "0 1 * * *" {
								return fmt.Errorf("remote pipeline schedule cronline (%s) doesn't match expected value (%s)", *schedule.Cronline, "0 1 * * *")
							}
							return nil
						},
						// Confirm the pipeline has the correct values in terraform state
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "label", label),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "cronline", "0 1 * * *"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "branch", "main"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "env.FOO", "bar"),
						resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "enabled", "false"),
					),
				},
			},
		})
	})

	t.Run("pipeline schedule env transitions between empty and populated", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		label := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineScheduleDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(pipelineName, "0 * * * *", label, "", true),
					Check:  resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "env.%", "0"),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					},
				},
				{
					Config: config(pipelineName, "0 * * * *", label, "FOO = \"bar\"", true),
					Check:  resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "env.FOO", "bar"),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					},
				},
				{
					Config: config(pipelineName, "0 * * * *", label, "", true),
					Check:  resource.TestCheckResourceAttr("buildkite_pipeline_schedule.pipeline", "env.%", "0"),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
					},
				},
			},
		})
	})

	t.Run("pipeline schedule is recreated if removed", func(t *testing.T) {
		pipelineName := acctest.RandString(12)
		label := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineScheduleDestroy,
			Steps: []resource.TestStep{
				{
					Config: config(pipelineName, "0 * * * *", label, "", true),
					Check: func(s *terraform.State) error {
						ps := s.RootModule().Resources["buildkite_pipeline_schedule.pipeline"]
						_, err := deletePipelineSchedule(context.Background(),
							genqlientGraphql,
							ps.Primary.ID)
						return err
					},
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline_schedule.pipeline", plancheck.ResourceActionCreate),
						},
					},
				},
			},
		})
	})
}

func TestUpdatePipelineScheduleNodeEnvDiscriminator(t *testing.T) {
	ctx := context.Background()
	label, cronline, branch, commit, message := "lbl", "0 * * * *", "main", "HEAD", ""
	emptyMap, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{})
	populatedMap, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"FOO": "bar"})
	fooEqualsBar := "FOO=bar"

	tests := []struct {
		name     string
		priorEnv types.Map
		apiEnv   []*string
		want     func(t *testing.T, got types.Map)
	}{
		{
			name:     "empty map preserved when API returns no env vars",
			priorEnv: emptyMap,
			apiEnv:   nil,
			want: func(t *testing.T, got types.Map) {
				if got.IsNull() || len(got.Elements()) != 0 {
					t.Errorf("expected non-null empty map, got %v", got)
				}
			},
		},
		{
			name:     "null preserved when API returns no env vars",
			priorEnv: types.MapNull(types.StringType),
			apiEnv:   nil,
			want: func(t *testing.T, got types.Map) {
				if !got.IsNull() {
					t.Errorf("expected null, got %v", got)
				}
			},
		},
		{
			name:     "populated state overwritten with null on out-of-band env deletion",
			priorEnv: populatedMap,
			apiEnv:   nil,
			want: func(t *testing.T, got types.Map) {
				if !got.IsNull() {
					t.Errorf("expected null to surface drift, got %v", got)
				}
			},
		},
		{
			name:     "API env vars overwrite null state",
			priorEnv: types.MapNull(types.StringType),
			apiEnv:   []*string{&fooEqualsBar},
			want: func(t *testing.T, got types.Map) {
				if _, ok := got.Elements()["FOO"]; !ok {
					t.Errorf("expected FOO in env, got %v", got)
				}
			},
		},
		{
			name:     "API env vars overwrite empty map state",
			priorEnv: emptyMap,
			apiEnv:   []*string{&fooEqualsBar},
			want: func(t *testing.T, got types.Map) {
				if _, ok := got.Elements()["FOO"]; !ok {
					t.Errorf("expected FOO in env, got %v", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &pipelineScheduleResourceModel{Env: tt.priorEnv}
			node := getPipelineScheduleNodePipelineSchedule{
				PipelineScheduleValues: PipelineScheduleValues{
					Id:       "schedule-id",
					Uuid:     "schedule-uuid",
					Label:    &label,
					Cronline: &cronline,
					Message:  &message,
					Commit:   &commit,
					Branch:   &branch,
					Env:      tt.apiEnv,
					Enabled:  true,
					Pipeline: PipelineScheduleValuesPipeline{Id: "pipeline-id"},
				},
			}
			updatePipelineScheduleNode(ctx, state, node)
			tt.want(t, state.Env)
		})
	}
}

// Testcase destroyer function
func testAccCheckPipelineScheduleDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline_schedule" {
			continue
		}

		resp, err := getPipelineSchedule(context.Background(), genqlientGraphql, rs.Primary.ID)
		if err != nil {
			if isResourceNotFoundError(err) {
				continue
			}
			return err
		}

		if node, ok := resp.GetNode().(*getPipelineScheduleNodePipelineSchedule); ok && node != nil {
			return fmt.Errorf("pipeline schedule still exists upstream: %s", node.Id)
		}
	}
	return nil
}
