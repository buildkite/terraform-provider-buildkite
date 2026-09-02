package buildkite

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteClusterQueueResource(t *testing.T) {
	configBasic := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}

		resource "buildkite_cluster" "cluster_test" {
			name = "Test cluster %s"
			description = "Acceptance testing cluster"
		}

		resource "buildkite_cluster_queue" "foobar" {
			cluster_id = buildkite_cluster.cluster_test.id
			key = "queue-%s"
			description = "Acceptance test %s"
		}
		`, fields[0], fields[1], fields[2])
	}

	configBasicDispatch := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}
		resource "buildkite_cluster" "cluster_test" {
			name = "Test cluster %s"
			description = "Acceptance testing cluster"
		}
		resource "buildkite_cluster_queue" "foobar" {
			cluster_id = buildkite_cluster.cluster_test.id
			key = "queue-%s"
			description = "Acceptance test %s"
			dispatch_paused = "%s"
		}
		`, fields[0], fields[1], fields[2], fields[3])
	}

	configRetryAffinity := func(fields ...string) string {
		return fmt.Sprintf(`
		provider "buildkite" {
			timeouts = {
				create = "60s"
				read = "60s"
				update = "60s"
				delete = "60s"
			}
		}
		resource "buildkite_cluster" "cluster_test" {
			name = "Test cluster %s"
			description = "Acceptance testing cluster"
		}
		resource "buildkite_cluster_queue" "foobar" {
			cluster_id = buildkite_cluster.cluster_test.id
			key = "queue-%s"
			description = "Acceptance test %s"
			retry_agent_affinity = "%s"
		}
		`, fields[0], fields[1], fields[2], fields[3])
	}

	configHostedMac := func(fields ...string) string {
		return fmt.Sprintf(`
    provider "buildkite" {
        timeouts = {
            create = "60s"
            read = "60s"
            update = "60s"
            delete = "60s"
        }
    }

    resource "buildkite_cluster" "cluster_test" {
        name = "Test cluster %s"
        description = "Acceptance testing cluster"
    }

    resource "buildkite_cluster_queue" "foobar" {
        cluster_id = buildkite_cluster.cluster_test.id
        key = "queue-%s"
        description = "Acceptance test %s"

        hosted_agents = {
            mac = {
                xcode_version = "14.3.1"
            }
            instance_shape = "MACOS_ARM64_M4_6X28"
        }
    }
    `, fields[0], fields[1], fields[2])
	}

	configHostedLinux := func(fields ...string) string {
		return fmt.Sprintf(`
    provider "buildkite" {
        timeouts = {
            create = "60s"
            read = "60s"
            update = "60s"
            delete = "60s"
        }
    }

    resource "buildkite_cluster" "cluster_test" {
        name = "Test cluster %s"
        description = "Acceptance testing cluster"
    }

    resource "buildkite_cluster_queue" "foobar" {
        cluster_id = buildkite_cluster.cluster_test.id
        key = "queue-%s"
        description = "Acceptance test %s"

        hosted_agents = {
            linux = {
                agent_image_ref = "buildkite/agent:latest"
            }
            instance_shape = "LINUX_AMD64_2X4"
        }
    }
    `, fields[0], fields[1], fields[2])
	}

	configInvalidMacShape := func(fields ...string) string {
		return fmt.Sprintf(`
    provider "buildkite" {
        timeouts = {
            create = "60s"
            read = "60s"
            update = "60s"
            delete = "60s"
        }
    }

    resource "buildkite_cluster" "cluster_test" {
        name = "Test cluster %s"
        description = "Acceptance testing cluster"
    }

    resource "buildkite_cluster_queue" "foobar" {
        cluster_id = buildkite_cluster.cluster_test.id
        key = "queue-%s"
        description = "Acceptance test %s"

        hosted_agents = {
            mac = {
                xcode_version = "14.3.1"
            }
            instance_shape = "LINUX_AMD64_2X4"
        }
    }
    `, fields[0], fields[1], fields[2])
	}

	configInvalidLinuxShape := func(fields ...string) string {
		return fmt.Sprintf(`
    provider "buildkite" {
        timeouts = {
            create = "60s"
            read = "60s"
            update = "60s"
            delete = "60s"
        }
    }

    resource "buildkite_cluster" "cluster_test" {
        name = "Test cluster %s"
        description = "Acceptance testing cluster"
    }

    resource "buildkite_cluster_queue" "foobar" {
        cluster_id = buildkite_cluster.cluster_test.id
        key = "queue-%s"
        description = "Acceptance test %s"

        hosted_agents = {
            linux = {
                agent_image_ref = "buildkite/agent:latest"
            }
            instance_shape = "MACOS_ARM64_M4_6X28"
        }
    }
    `, fields[0], fields[1], fields[2])
	}

	configBothPlatforms := func(fields ...string) string {
		return fmt.Sprintf(`
    provider "buildkite" {
        timeouts = {
            create = "60s"
            read = "60s"
            update = "60s"
            delete = "60s"
        }
    }

    resource "buildkite_cluster" "cluster_test" {
        name = "Test cluster %s"
        description = "Acceptance testing cluster"
    }

    resource "buildkite_cluster_queue" "foobar" {
        cluster_id = buildkite_cluster.cluster_test.id
        key = "queue-%s"
        description = "Acceptance test %s"

        hosted_agents = {
            mac = {
                xcode_version = "14.3.1"
            }
            linux = {
                agent_image_ref = "buildkite/agent:latest"
            }
            instance_shape = "MACOS_ARM64_M4_6X28"
        }
    }
    `, fields[0], fields[1], fields[2])
	}

	t.Run("creates a cluster queue", func(t *testing.T) {
		var cq clusterQueueResourceModel
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the cluster queue exists in the buildkite API
			testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
			// Confirm the cluster queue has the correct values in Buildkite's system
			testAccCheckClusterQueueRemoteValues(&cq, fmt.Sprintf("Acceptance test %s", queueDesc), fmt.Sprintf("queue-%s", queueKey)),
			// Confirm the cluster queue has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "key", fmt.Sprintf("queue-%s", queueKey)),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", fmt.Sprintf("Acceptance test %s", queueDesc)),
			resource.TestCheckResourceAttrSet("buildkite_cluster_queue.foobar", "id"),
			resource.TestCheckResourceAttrSet("buildkite_cluster_queue.foobar", "uuid"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, queueKey, queueDesc),
					Check:  check,
				},
				{
					RefreshState: true,
					PlanOnly:     true,
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet("buildkite_cluster_queue.foobar", "key"),
						resource.TestCheckResourceAttrSet("buildkite_cluster_queue.foobar", "description"),
					),
				},
			},
		})
	})

	t.Run("updates a cluster queue", func(t *testing.T) {
		var cq clusterQueueResourceModel
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)
		updatedQueueDesc := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the cluster queue exists in the buildkite API
			testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
			// Confirm the cluster queue has the correct values in Buildkite's system
			testAccCheckClusterQueueRemoteValues(&cq, fmt.Sprintf("Acceptance test %s", queueDesc), fmt.Sprintf("queue-%s", queueKey)),
			// Confirm the cluster queue has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "key", fmt.Sprintf("queue-%s", queueKey)),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", fmt.Sprintf("Acceptance test %s", queueDesc)),
		)

		checkUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm the cluster queue exists in the buildkite API
			testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
			// Confirm the cluster queue has the correct values in Buildkite's system
			testAccCheckClusterQueueRemoteValues(&cq, fmt.Sprintf("Acceptance test %s", updatedQueueDesc), fmt.Sprintf("queue-%s", queueKey)),
			// Confirm the cluster queue has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "key", fmt.Sprintf("queue-%s", queueKey)),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", fmt.Sprintf("Acceptance test %s", updatedQueueDesc)),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, queueKey, queueDesc),
					Check:  check,
				},
				{
					Config: configBasic(clusterName, queueKey, updatedQueueDesc),
					Check:  checkUpdated,
				},
			},
		})
	})

	t.Run("pause dispatch on a cluster queue", func(t *testing.T) {
		var cq clusterQueueResourceModel
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)
		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the cluster queue exists in the buildkite API
			testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
			// Confirm the cluster queue has the correct values in Buildkite's system
			testAccCheckClusterQueueRemoteValues(&cq, fmt.Sprintf("Acceptance test %s", queueDesc), fmt.Sprintf("queue-%s", queueKey)),
			// Confirm the cluster queue has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "key", fmt.Sprintf("queue-%s", queueKey)),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", fmt.Sprintf("Acceptance test %s", queueDesc)),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "dispatch_paused", "false"),
		)
		checkUpdated := resource.ComposeAggregateTestCheckFunc(
			// Confirm the cluster queue exists in the buildkite API
			testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
			// Confirm the cluster queue has the correct values in Buildkite's system
			testAccCheckClusterQueueRemoteValues(&cq, fmt.Sprintf("Acceptance test %s", queueDesc), fmt.Sprintf("queue-%s", queueKey)),
			// Confirm the cluster queue has the correct values in terraform state
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "key", fmt.Sprintf("queue-%s", queueKey)),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", fmt.Sprintf("Acceptance test %s", queueDesc)),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "dispatch_paused", "true"),
		)
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasicDispatch(clusterName, queueKey, queueDesc, "false"),
					Check:  check,
				},
				{
					Config: configBasicDispatch(clusterName, queueKey, queueDesc, "true"),
					Check:  checkUpdated,
				},
			},
		})
	})

	t.Run("imports a cluster queue", func(t *testing.T) {
		var cq clusterQueueResourceModel
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			// Confirm the cluster queue exists in the buildkite API
			testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
			// Check to confirm the local state is correct before we re-import it
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "key", fmt.Sprintf("queue-%s", queueKey)),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", fmt.Sprintf("Acceptance test %s", queueDesc)),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, queueKey, queueDesc),
					Check:  check,
				},
				{
					// re-import the resource (using the graphql token of the existing resource) and confirm they match
					ResourceName:      "buildkite_cluster_queue.foobar",
					ImportStateIdFunc: testAccGetImportClusterQueueId(&cq),
					ImportState:       true,
					ImportStateVerify: true,
				},
			},
		})
	})

	t.Run("preserves the API-selected macOS version during unrelated updates", func(t *testing.T) {
		var cq clusterQueueResourceModel
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)
		updatedQueueDesc := acctest.RandString(10)
		var macosVersion string

		checkCreated := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "hosted_agents.instance_shape", "MACOS_ARM64_M4_6X28"),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "hosted_agents.mac.xcode_version", "14.3.1"),
			resource.TestCheckResourceAttrWith("buildkite_cluster_queue.foobar", "hosted_agents.mac.macos_version", func(value string) error {
				macosVersion = value
				return nil
			}),
		)

		checkUpdated := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", fmt.Sprintf("Acceptance test %s", updatedQueueDesc)),
			resource.TestCheckResourceAttrWith("buildkite_cluster_queue.foobar", "hosted_agents.mac.macos_version", func(value string) error {
				if value != macosVersion {
					return fmt.Errorf("macOS version changed from %q to %q after an unrelated update", macosVersion, value)
				}

				return nil
			}),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config: configHostedMac(clusterName, queueKey, queueDesc),
					Check:  checkCreated,
				},
				{
					Config: configHostedMac(clusterName, queueKey, updatedQueueDesc),
					Check:  checkUpdated,
				},
			},
		})
	})

	t.Run("creates a hosted linux queue", func(t *testing.T) {
		var cq clusterQueueResourceModel
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)

		check := resource.ComposeAggregateTestCheckFunc(
			testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "hosted_agents.linux.agent_image_ref", "buildkite/agent:latest"),
			resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "hosted_agents.instance_shape", "LINUX_AMD64_2X4"),
		)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config: configHostedLinux(clusterName, queueKey, queueDesc),
					Check:  check,
				},
			},
		})
	})

	t.Run("fails with invalid mac instance shape", func(t *testing.T) {
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configInvalidMacShape(clusterName, queueKey, queueDesc),
					ExpectError: regexp.MustCompile("Invalid instance shape for Mac platform"),
				},
			},
		})
	})

	t.Run("fails with invalid linux instance shape", func(t *testing.T) {
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configInvalidLinuxShape(clusterName, queueKey, queueDesc),
					ExpectError: regexp.MustCompile("Invalid instance shape for Linux platform"),
				},
			},
		})
	})

	t.Run("fails with both platforms specified", func(t *testing.T) {
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config:      configBothPlatforms(clusterName, queueKey, queueDesc),
					ExpectError: regexp.MustCompile(`Invalid platform configuration`),
				},
			},
		})
	})

	t.Run("creates a cluster queue with retry_agent_affinity", func(t *testing.T) {
		var cq clusterQueueResourceModel
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config: configRetryAffinity(clusterName, queueKey, queueDesc, "prefer-different"),
					Check: resource.ComposeAggregateTestCheckFunc(
						testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
						resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "retry_agent_affinity", "prefer-different"),
					),
				},
			},
		})
	})

	t.Run("defaults retry_agent_affinity to prefer-warmest when omitted", func(t *testing.T) {
		var cq clusterQueueResourceModel
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config: configBasic(clusterName, queueKey, queueDesc),
					Check: resource.ComposeAggregateTestCheckFunc(
						testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
						resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "retry_agent_affinity", "prefer-warmest"),
					),
				},
			},
		})
	})

	t.Run("updates retry_agent_affinity value", func(t *testing.T) {
		var cq clusterQueueResourceModel
		clusterName := acctest.RandString(10)
		queueKey := acctest.RandString(10)
		queueDesc := acctest.RandString(10)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterQueueDestroy,
			Steps: []resource.TestStep{
				{
					Config: configRetryAffinity(clusterName, queueKey, queueDesc, "prefer-warmest"),
					Check: resource.ComposeAggregateTestCheckFunc(
						testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
						resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "retry_agent_affinity", "prefer-warmest"),
					),
				},
				{
					Config: configRetryAffinity(clusterName, queueKey, queueDesc, "prefer-different"),
					Check: resource.ComposeAggregateTestCheckFunc(
						testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
						resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "retry_agent_affinity", "prefer-different"),
					),
				},
			},
		})
	})
}

func testAccCheckClusterQueueExists(resourceName string, clusterQueueResourceModel *clusterQueueResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		ctx := context.Background()
		err := retry.RetryContext(ctx, DefaultTimeout, func() *retry.RetryError {
			r, err := getClusterQueueByNode(ctx, genqlientGraphql, resourceState.Primary.ID)
			if err != nil {
				return retryContextError(err)
			}

			// Check if the node exists and is a ClusterQueue
			if r.Node == nil {
				return retry.NonRetryableError(fmt.Errorf("Cluster queue not found with ID: %s", resourceState.Primary.ID))
			}

			clusterQueue, ok := r.Node.(*getClusterQueueByNodeNodeClusterQueue)
			if !ok {
				return retry.NonRetryableError(fmt.Errorf("Invalid node type returned"))
			}

			// Update ClusterQueueResourceModel with Node values
			updateClusterQueueResourceFromNode(*clusterQueue, clusterQueueResourceModel)
			return nil
		})
		if err != nil {
			return fmt.Errorf("Error fetching Cluster queue from graphql API: %v", err)
		}

		// If clusterQueueResourceModel isnt set
		if clusterQueueResourceModel.Id.ValueString() == "" {
			return fmt.Errorf("No Cluster queue found with graphql id: %s", resourceState.Primary.ID)
		}

		return nil
	}
}

func testAccCheckClusterQueueRemoteValues(cq *clusterQueueResourceModel, description, key string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		if cq.Key.ValueString() != key {
			return fmt.Errorf("Remote Cluster queue key (%s) doesn't match expected value (%s)", cq.Key, key)
		}

		if cq.Description.ValueString() != description {
			return fmt.Errorf("Remote Cluster queue description (%s) doesn't match expected value (%s)", cq.Description, description)
		}

		return nil
	}
}

func testAccGetImportClusterQueueId(cq *clusterQueueResourceModel) resource.ImportStateIdFunc {
	return func(s *terraform.State) (string, error) {
		// Obtain trimmed cluster ID and cluster UUID
		clusterUuid := strings.Trim(cq.Id.ValueString(), "\"")
		clusterQueueID := strings.Trim(cq.ClusterUuid.ValueString(), "\"")
		// Set ID for import
		id := fmt.Sprintf("%s,%s", clusterUuid, clusterQueueID)
		return id, nil
	}
}

func testAccCheckClusterQueueDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_queue" {
			continue
		}
	}
	return nil
}

// Pausing dispatch is a mutation of its own, applied before the queue mutation that follows it. If
// the later one fails and Update returns without recording state, Terraform believes dispatch is
// still running on a queue that is actually paused, and nothing in the next plan says otherwise
// until state is refreshed.
func TestClusterQueueUpdatePersistsThePauseWhenALaterStepFails(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t,
		// pauseDispatchClusterQueue applies.
		stubResponse{status: http.StatusOK, body: `{"data":{"clusterQueuePauseDispatch":{"clusterQueue":{
			"id": "queue-id", "uuid": "queue-uuid", "key": "a-queue", "dispatchPaused": true
		}}}}`},
		// updateClusterQueue does not.
		stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"queue update exploded"}]}`},
	)
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
	orgID := "organization-id"
	client.organizationId = &orgID
	cq := &clusterQueueResource{client: client}

	ctx := t.Context()
	var schemaResp fwresource.SchemaResponse
	cq.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
	}
	schema := schemaResp.Schema

	shared := map[string]tftypes.Value{
		"id":                   tftypes.NewValue(tftypes.String, "queue-id"),
		"uuid":                 tftypes.NewValue(tftypes.String, "queue-uuid"),
		"cluster_id":           tftypes.NewValue(tftypes.String, "cluster-id"),
		"cluster_uuid":         tftypes.NewValue(tftypes.String, "cluster-uuid"),
		"key":                  tftypes.NewValue(tftypes.String, "a-queue"),
		"retry_agent_affinity": tftypes.NewValue(tftypes.String, RetryAgentAffinityPreferWarmest),
	}
	prior := map[string]tftypes.Value{"dispatch_paused": tftypes.NewValue(tftypes.Bool, false)}
	planned := map[string]tftypes.Value{
		"dispatch_paused": tftypes.NewValue(tftypes.Bool, true),
		"description":     tftypes.NewValue(tftypes.String, "a new description"),
	}
	maps.Copy(prior, shared)
	maps.Copy(planned, shared)

	priorRaw := nullObjectWith(ctx, t, schema.Type(), prior)
	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
		State:  tfsdk.State{Schema: schema, Raw: priorRaw},
		Config: tfsdk.Config{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: schema, Raw: priorRaw}}

	cq.Update(ctx, req, &resp)

	if got := requests.Load(); got < 2 {
		t.Fatalf("Made %d requests, want the pause to have applied before the failure", got)
	}
	if !diagnosticsContain(resp.Diagnostics, "Unable to update Cluster Queue") {
		t.Fatalf("Update() diagnostics = %v, want the queue update failure reported", resp.Diagnostics)
	}

	var persisted clusterQueueResourceModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if !persisted.DispatchPaused.ValueBool() {
		t.Error("Persisted dispatch_paused = false, want true: the pause applied, so state has to say so")
	}
}

// The mirror of the pause case above. Resuming dispatch is its own mutation, applied before the
// queue mutation that follows it, so a failure there must not leave state calling the queue paused
// when dispatch is running again.
func TestClusterQueueUpdatePersistsTheResumeWhenALaterStepFails(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t,
		// resumeDispatchClusterQueue applies.
		stubResponse{status: http.StatusOK, body: `{"data":{"clusterQueueResumeDispatch":{"clusterQueue":{
			"id": "queue-id", "uuid": "queue-uuid", "key": "a-queue", "dispatchPaused": false
		}}}}`},
		// updateClusterQueue does not.
		stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"queue update exploded"}]}`},
	)
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
	orgID := "organization-id"
	client.organizationId = &orgID
	cq := &clusterQueueResource{client: client}

	ctx := t.Context()
	var schemaResp fwresource.SchemaResponse
	cq.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
	}
	schema := schemaResp.Schema

	shared := map[string]tftypes.Value{
		"id":                   tftypes.NewValue(tftypes.String, "queue-id"),
		"uuid":                 tftypes.NewValue(tftypes.String, "queue-uuid"),
		"cluster_id":           tftypes.NewValue(tftypes.String, "cluster-id"),
		"cluster_uuid":         tftypes.NewValue(tftypes.String, "cluster-uuid"),
		"key":                  tftypes.NewValue(tftypes.String, "a-queue"),
		"retry_agent_affinity": tftypes.NewValue(tftypes.String, RetryAgentAffinityPreferWarmest),
	}
	prior := map[string]tftypes.Value{"dispatch_paused": tftypes.NewValue(tftypes.Bool, true)}
	planned := map[string]tftypes.Value{
		"dispatch_paused": tftypes.NewValue(tftypes.Bool, false),
		"description":     tftypes.NewValue(tftypes.String, "a new description"),
	}
	maps.Copy(prior, shared)
	maps.Copy(planned, shared)

	priorRaw := nullObjectWith(ctx, t, schema.Type(), prior)
	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
		State:  tfsdk.State{Schema: schema, Raw: priorRaw},
		Config: tfsdk.Config{Schema: schema, Raw: nullObjectWith(ctx, t, schema.Type(), planned)},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: schema, Raw: priorRaw}}

	cq.Update(ctx, req, &resp)

	if got := requests.Load(); got < 2 {
		t.Fatalf("Made %d requests, want the resume to have applied before the failure", got)
	}
	if !diagnosticsContain(resp.Diagnostics, "Unable to update Cluster Queue") {
		t.Fatalf("Update() diagnostics = %v, want the queue update failure reported", resp.Diagnostics)
	}

	var persisted clusterQueueResourceModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if persisted.DispatchPaused.ValueBool() {
		t.Error("Persisted dispatch_paused = true, want false: the resume applied, so state has to say so")
	}
}

// Setting retry_agent_affinity is a REST call made after the queue has been created. Create used to
// persist on that failure with a bare resp.State.Set whose diagnostics went nowhere; it now shares
// the deferred write with every other path out. Either way the queue exists, and dropping the write
// would leave it running with nothing in state pointing at it, recoverable only by terraform import.
func TestClusterQueueCreatePersistsTheQueueWhenTheAffinityUpdateFails(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t,
		// createClusterQueue applies.
		stubResponse{status: http.StatusOK, body: `{"data":{"clusterQueueCreate":{"clusterQueue":{
			"id": "queue-id",
			"uuid": "queue-uuid",
			"key": "a-queue",
			"description": "a description",
			"cluster": {"id": "cluster-id", "uuid": "cluster-uuid"},
			"hosted": false
		}}}}`},
		// The retry_agent_affinity PATCH does not.
		stubResponse{status: http.StatusInternalServerError, body: `{"message":"affinity update exploded"}`},
	)
	defer server.Close()

	client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
	orgID := "organization-id"
	client.organizationId = &orgID
	cq := &clusterQueueResource{client: client}

	ctx := t.Context()
	var schemaResp fwresource.SchemaResponse
	cq.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("Schema() diagnostics = %v", schemaResp.Diagnostics)
	}
	schema := schemaResp.Schema

	planned := nullObjectWith(ctx, t, schema.Type(), map[string]tftypes.Value{
		"cluster_id":           tftypes.NewValue(tftypes.String, "cluster-id"),
		"key":                  tftypes.NewValue(tftypes.String, "a-queue"),
		"description":          tftypes.NewValue(tftypes.String, "a description"),
		"retry_agent_affinity": tftypes.NewValue(tftypes.String, RetryAgentAffinityPreferDifferent),
		"dispatch_paused":      tftypes.NewValue(tftypes.Bool, false),
	})

	req := fwresource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: schema, Raw: planned},
		Config: tfsdk.Config{Schema: schema, Raw: planned},
	}
	resp := fwresource.CreateResponse{State: tfsdk.State{Schema: schema, Raw: tftypes.NewValue(schema.Type().TerraformType(ctx), nil)}}

	cq.Create(ctx, req, &resp)

	if got := requests.Load(); got < 2 {
		t.Fatalf("Made %d requests, want the queue to have been created before the failure", got)
	}
	if !diagnosticsContain(resp.Diagnostics, "Unable to set retry_agent_affinity") {
		t.Fatalf("Create() diagnostics = %v, want the affinity failure reported", resp.Diagnostics)
	}

	var persisted clusterQueueResourceModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if persisted.Id.ValueString() != "queue-id" {
		t.Errorf("Persisted id = %q, want %q: the queue was created, so state has to point at it", persisted.Id.ValueString(), "queue-id")
	}
	if persisted.Uuid.ValueString() != "queue-uuid" {
		t.Errorf("Persisted uuid = %q, want %q", persisted.Uuid.ValueString(), "queue-uuid")
	}
	if persisted.ClusterUuid.ValueString() != "cluster-uuid" {
		t.Errorf("Persisted cluster_uuid = %q, want %q: Delete needs it to reach the queue", persisted.ClusterUuid.ValueString(), "cluster-uuid")
	}
	if persisted.DispatchPaused.ValueBool() {
		t.Error("Persisted dispatch_paused = true, want false: the queue is created with dispatch running")
	}
}
