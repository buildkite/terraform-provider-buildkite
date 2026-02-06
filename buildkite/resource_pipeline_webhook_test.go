package buildkite

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkitePipelineWebhook(t *testing.T) {
	repo := os.Getenv("GITHUB_TEST_REPO")

	configBasic := func(name string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "cluster" {
				name = "%s_cluster"
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "%s"
				cluster_id = buildkite_cluster.cluster.id
			}

			resource "buildkite_pipeline_webhook" "webhook" {
				pipeline_id    = buildkite_pipeline.pipeline.id
				repository_url = buildkite_pipeline.pipeline.repository
			}
		`, name, name, repo)
	}

	configPipelineOnly := func(name string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "cluster" {
				name = "%s_cluster"
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "%s"
				cluster_id = buildkite_cluster.cluster.id
			}
		`, name, name, repo)
	}

	t.Run("pipeline webhook can be created and imported", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineWebhookDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "id"),
						resource.TestCheckResourceAttr("buildkite_pipeline_webhook.webhook", "repository_url", repo),
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "webhook_url"),
						resource.TestCheckResourceAttrPair(
							"buildkite_pipeline_webhook.webhook", "pipeline_id",
							"buildkite_pipeline.pipeline", "id",
						),
					),
				},
				{
					ResourceName:      "buildkite_pipeline_webhook.webhook",
					ImportState:             true,
					ImportStateIdFunc:       getPipelineIdForImport("buildkite_pipeline.pipeline"),
					ImportStateVerify:       true,
					ImportStateVerifyIgnore: []string{"repository_url"},
				},
			},
		})
	})

	t.Run("pipeline webhook is recreated if removed externally", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineWebhookDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(pipelineName),
					Check: func(s *terraform.State) error {
						pipelineRes := s.RootModule().Resources["buildkite_pipeline.pipeline"]
						_, err := deletePipelineWebhook(context.Background(),
							genqlientGraphql,
							pipelineRes.Primary.ID)
						return err
					},
					ExpectNonEmptyPlan: true,
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline_webhook.webhook", plancheck.ResourceActionCreate),
						},
					},
				},
			},
		})
	})

	t.Run("pipeline webhook is deleted when resource is removed", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineWebhookDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(pipelineName),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "id"),
					),
				},
				{
					Config: configPipelineOnly(pipelineName),
					Check: func(s *terraform.State) error {
						pipelineRes := s.RootModule().Resources["buildkite_pipeline.pipeline"]
						resp, err := getPipelineWebhook(context.Background(), genqlientGraphql, pipelineRes.Primary.ID)
						if err != nil {
							return err
						}
						if pipeline, ok := resp.GetNode().(*getPipelineWebhookNodePipeline); ok && pipeline != nil {
							info, _ := extractWebhookFromPipeline(pipeline)
							if info != nil {
								return fmt.Errorf("webhook still exists after resource removal")
							}
						}
						return nil
					},
				},
			},
		})
	})
}

func TestAccBuildkitePipelineWebhook_ImportWithNoWebhook(t *testing.T) {
	repo := os.Getenv("GITHUB_TEST_REPO")

	configPipelineOnly := func(name string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "cluster" {
				name = "%s_cluster"
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "%s"
				cluster_id = buildkite_cluster.cluster.id
			}
		`, name, name, repo)
	}

	configWithWebhook := func(name string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "cluster" {
				name = "%s_cluster"
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "%s"
				cluster_id = buildkite_cluster.cluster.id
			}

			resource "buildkite_pipeline_webhook" "webhook" {
				pipeline_id    = buildkite_pipeline.pipeline.id
				repository_url = buildkite_pipeline.pipeline.repository
			}
		`, name, name, repo)
	}

	t.Run("import fails when pipeline has no webhook configured", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				// Step 1: Create pipeline without webhook
				{
					Config: configPipelineOnly(pipelineName),
				},
				// Step 2: Try to import a webhook that doesn't exist - should fail
				{
					Config:            configWithWebhook(pipelineName),
					ResourceName:      "buildkite_pipeline_webhook.webhook",
					ImportState:       true,
					ImportStateIdFunc: getPipelineIdForImport("buildkite_pipeline.pipeline"),
					ImportStateVerify: false,
					ExpectError:       regexp.MustCompile(`Cannot import non-existent remote object`),
				},
			},
		})
	})
}

func TestAccBuildkitePipelineWebhook_UnsupportedProvider(t *testing.T) {
	configUnsupportedProvider := func(name string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "cluster" {
				name = "%s_cluster"
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "https://gitlab.com/buildkite/test-repo.git"
				cluster_id = buildkite_cluster.cluster.id
			}

			resource "buildkite_pipeline_webhook" "webhook" {
				pipeline_id    = buildkite_pipeline.pipeline.id
				repository_url = buildkite_pipeline.pipeline.repository
			}
		`, name, name)
	}

	t.Run("pipeline webhook fails for unsupported provider", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config:      configUnsupportedProvider(pipelineName),
					ExpectError: regexp.MustCompile(`Auto-creating webhooks is not\s+supported for your repository provider`),
				},
			},
		})
	})
}

func getPipelineIdForImport(resourceName string) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		res := s.RootModule().Resources[resourceName]
		if res == nil {
			return "", fmt.Errorf("resource %s not found", resourceName)
		}
		return res.Primary.ID, nil
	}
}

func TestExtractWebhookFromPipeline_UnsupportedProvider(t *testing.T) {
	tests := []struct {
		name       string
		provider   getPipelineWebhookNodePipelineRepositoryProvider
		wantErr    bool
		wantErrMsg string
	}{
		{
			name:       "GitLab provider returns error",
			provider:   &getPipelineWebhookNodePipelineRepositoryProviderRepositoryProviderGitlab{Typename: "RepositoryProviderGitlab"},
			wantErr:    true,
			wantErrMsg: `unsupported repository provider: webhooks are not supported for repository provider GitLab`,
		},
		{
			name:       "Bitbucket provider returns error",
			provider:   &getPipelineWebhookNodePipelineRepositoryProviderRepositoryProviderBitbucket{Typename: "RepositoryProviderBitbucket"},
			wantErr:    true,
			wantErrMsg: `unsupported repository provider: webhooks are not supported for repository provider Bitbucket`,
		},
		{
			name:       "Unknown provider returns error",
			provider:   &getPipelineWebhookNodePipelineRepositoryProviderRepositoryProviderUnknown{Typename: "RepositoryProviderUnknown"},
			wantErr:    true,
			wantErrMsg: `unsupported repository provider: webhooks are not supported for repository provider Unknown`,
		},
		{
			name:       "nil provider returns error with unknown type",
			provider:   nil,
			wantErr:    true,
			wantErrMsg: `unsupported repository provider: webhooks are not supported for repository provider unknown`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := &getPipelineWebhookNodePipeline{
				Repository: getPipelineWebhookNodePipelineRepository{
					Url:      "https://example.com/repo.git",
					Provider: tt.provider,
				},
			}

			info, err := extractWebhookFromPipeline(pipeline)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if !errors.Is(err, ErrProviderUnknown) {
					t.Errorf("expected error to wrap ErrProviderUnknown but got %v", err)
				}
				if err.Error() != tt.wantErrMsg {
					t.Errorf("expected error %q but got %q", tt.wantErrMsg, err.Error())
				}
				if info != nil {
					t.Errorf("expected nil info but got %+v", info)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestExtractWebhookFromPipeline_NoWebhook(t *testing.T) {
	t.Run("GitHub provider with no webhook", func(t *testing.T) {
		pipeline := &getPipelineWebhookNodePipeline{
			Repository: getPipelineWebhookNodePipelineRepository{
				Url: "https://github.com/example/repo.git",
				Provider: &getPipelineWebhookNodePipelineRepositoryProviderRepositoryProviderGithub{
					Typename: "RepositoryProviderGithub",
					Webhook:  getPipelineWebhookNodePipelineRepositoryProviderRepositoryProviderGithubWebhook{},
				},
			},
		}

		info, err := extractWebhookFromPipeline(pipeline)

		if err != ErrNoWebhook {
			t.Errorf("expected ErrNoWebhook but got %v", err)
		}
		if info != nil {
			t.Errorf("expected nil info but got %+v", info)
		}
	})

	t.Run("GitHub Enterprise provider with no webhook", func(t *testing.T) {
		pipeline := &getPipelineWebhookNodePipeline{
			Repository: getPipelineWebhookNodePipelineRepository{
				Url: "https://github.example.com/org/repo.git",
				Provider: &getPipelineWebhookNodePipelineRepositoryProviderRepositoryProviderGithubEnterprise{
					Typename: "RepositoryProviderGithubEnterprise",
					Webhook:  getPipelineWebhookNodePipelineRepositoryProviderRepositoryProviderGithubEnterpriseWebhook{},
				},
			},
		}

		info, err := extractWebhookFromPipeline(pipeline)

		if err != ErrNoWebhook {
			t.Errorf("expected ErrNoWebhook but got %v", err)
		}
		if info != nil {
			t.Errorf("expected nil info but got %+v", info)
		}
	})
}

func TestAccBuildkitePipelineWebhook_ProviderChange(t *testing.T) {
	repo := os.Getenv("GITHUB_TEST_REPO")

	configWithWebhook := func(name, repository string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "cluster" {
				name = "%s_cluster"
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "%s"
				cluster_id = buildkite_cluster.cluster.id
			}

			resource "buildkite_pipeline_webhook" "webhook" {
				pipeline_id    = buildkite_pipeline.pipeline.id
				repository_url = buildkite_pipeline.pipeline.repository
			}
		`, name, name, repository)
	}

	configPipelineOnly := func(name, repository string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "cluster" {
				name = "%s_cluster"
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "%s"
				cluster_id = buildkite_cluster.cluster.id
			}
		`, name, name, repository)
	}

	t.Run("webhook is removed from state when provider changes to unsupported type", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: configWithWebhook(pipelineName, repo),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "id"),
					),
				},
				{
					Config: configPipelineOnly(pipelineName, "https://gitlab.com/buildkite/test-repo.git"),
					Check: func(s *terraform.State) error {
						if _, ok := s.RootModule().Resources["buildkite_pipeline_webhook.webhook"]; ok {
							return fmt.Errorf("webhook resource should have been removed from state")
						}
						return nil
					},
				},
			},
		})
	})
}

func TestAccBuildkitePipelineWebhook_RepositoryChange(t *testing.T) {
	repo := os.Getenv("GITHUB_TEST_REPO")
	repoAlt := os.Getenv("GITHUB_TEST_REPO_ALT")
	if repoAlt == "" {
		t.Skip("GITHUB_TEST_REPO_ALT must be set to a different GitHub repo connected via the GitHub App")
	}

	configWithWebhook := func(name, repository string) string {
		return fmt.Sprintf(`
			provider "buildkite" {
				timeouts = {
					create = "60s"
					read = "60s"
					update = "60s"
					delete = "60s"
				}
			}

			resource "buildkite_cluster" "cluster" {
				name = "%s_cluster"
			}

			resource "buildkite_pipeline" "pipeline" {
				name = "%s"
				repository = "%s"
				cluster_id = buildkite_cluster.cluster.id
			}

			resource "buildkite_pipeline_webhook" "webhook" {
				pipeline_id    = buildkite_pipeline.pipeline.id
				repository_url = buildkite_pipeline.pipeline.repository
			}
		`, name, name, repository)
	}

	t.Run("webhook is replaced when pipeline repository changes", func(t *testing.T) {
		pipelineName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckPipelineWebhookDestroy,
			Steps: []resource.TestStep{
				{
					Config: configWithWebhook(pipelineName, repo),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "id"),
						resource.TestCheckResourceAttr("buildkite_pipeline_webhook.webhook", "repository_url", repo),
					),
				},
				{
					Config: configWithWebhook(pipelineName, repoAlt),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("buildkite_pipeline_webhook.webhook", "id"),
						resource.TestCheckResourceAttr("buildkite_pipeline_webhook.webhook", "repository_url", repoAlt),
					),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PreApply: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_pipeline_webhook.webhook", plancheck.ResourceActionDestroyBeforeCreate),
						},
					},
				},
			},
		})
	})
}

func testAccCheckPipelineWebhookDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_pipeline_webhook" {
			continue
		}

		resp, err := getPipelineWebhook(context.Background(), genqlientGraphql, rs.Primary.Attributes["pipeline_id"])
		if err != nil {
			return err
		}
		if pipeline, ok := resp.GetNode().(*getPipelineWebhookNodePipeline); ok && pipeline != nil {
			info, _ := extractWebhookFromPipeline(pipeline)
			if info != nil {
				return fmt.Errorf("pipeline webhook still exists")
			}
		}
	}
	return nil
}
