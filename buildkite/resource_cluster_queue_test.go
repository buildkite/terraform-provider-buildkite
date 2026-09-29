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

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
				{
					// <cluster uuid>/<queue key> is also accepted
					ResourceName:      "buildkite_cluster_queue.foobar",
					ImportStateIdFunc: testAccImportIDFromAttributes("buildkite_cluster_queue.foobar", "cluster_uuid", "buildkite_cluster_queue.foobar", "key"),
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

// hostedAgentsValue builds a linux hosted_agents object from the schema's own nested types, so a
// fixture does not have to restate them and drift when the schema changes.
func hostedAgentsValue(ctx context.Context, t *testing.T, schemaType attr.Type, shape string) tftypes.Value {
	t.Helper()

	object, ok := schemaType.TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("Expected the schema to be an object")
	}
	hosted, ok := object.AttributeTypes["hosted_agents"].(tftypes.Object)
	if !ok {
		t.Fatal("Expected hosted_agents to be an object")
	}
	linux, ok := hosted.AttributeTypes["linux"].(tftypes.Object)
	if !ok {
		t.Fatal("Expected hosted_agents.linux to be an object")
	}

	return tftypes.NewValue(hosted, map[string]tftypes.Value{
		"instance_shape": tftypes.NewValue(tftypes.String, shape),
		"mac":            tftypes.NewValue(hosted.AttributeTypes["mac"], nil),
		"linux": tftypes.NewValue(linux, map[string]tftypes.Value{
			"agent_image_ref": tftypes.NewValue(tftypes.String, "an-image-ref"),
		}),
	})
}

// Update pauses before the rest of the update and resumes after it, so a failed apply leaves the
// queue paused either way. Both halves of that need state to agree: a pause that applied has to be
// recorded even though the step after it failed, and a resume that never ran must not be recorded
// just because the plan asked for it. Getting either wrong leaves Terraform describing a dispatch
// state the queue is not in, with nothing in the next plan to say so until state is refreshed.
func TestClusterQueueUpdateLeavesDispatchPausedWhenAStepFails(t *testing.T) {
	t.Parallel()

	// The queue mutation that fails in both cases, after the pause in one and before the resume in
	// the other.
	queueUpdateFailed := stubResponse{status: http.StatusOK, body: `{"errors":[{"message":"queue update exploded"}]}`}

	tests := []struct {
		name  string
		stubs []stubResponse
		// The number of requests the ordering allows. The resume case turns on the resume mutation
		// never being sent, which the count is the only evidence of.
		wantRequests                     int
		priorPause, planPause, wantPause bool
	}{
		{
			// The pause mutation selects only clientMutationId, so there is no dispatchPaused in its
			// response and the assertion can only be satisfied by Update recording the pause itself.
			name: "the pause applies and the queue update then fails",
			stubs: []stubResponse{
				{status: http.StatusOK, body: `{"data":{"clusterQueuePauseDispatch":{"clientMutationId":""}}}`},
				queueUpdateFailed,
			},
			wantRequests: 2,
			priorPause:   false,
			planPause:    true,
			wantPause:    true,
		},
		{
			// Resuming last is what makes this fail closed. The queue keeps running the old
			// configuration, but it is not dispatching jobs against it.
			name:         "the queue update fails before the resume is reached",
			stubs:        []stubResponse{queueUpdateFailed},
			wantRequests: 1,
			priorPause:   true,
			planPause:    false,
			wantPause:    true,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, requests := newRetryStub(t, testCase.stubs...)
			defer server.Close()

			client := newRetryTestClient(t, server.URL, 0, time.Millisecond)
			orgID := "organization-id"
			client.organizationId = &orgID
			cq := &clusterQueueResource{client: client}

			ctx := t.Context()
			sch := resourceSchema(ctx, t, cq)

			shared := map[string]tftypes.Value{
				"id":                   tftypes.NewValue(tftypes.String, "queue-id"),
				"uuid":                 tftypes.NewValue(tftypes.String, "queue-uuid"),
				"cluster_id":           tftypes.NewValue(tftypes.String, "cluster-id"),
				"cluster_uuid":         tftypes.NewValue(tftypes.String, "cluster-uuid"),
				"key":                  tftypes.NewValue(tftypes.String, "a-queue"),
				"retry_agent_affinity": tftypes.NewValue(tftypes.String, RetryAgentAffinityPreferWarmest),
			}
			prior := map[string]tftypes.Value{"dispatch_paused": tftypes.NewValue(tftypes.Bool, testCase.priorPause)}
			planned := map[string]tftypes.Value{
				"dispatch_paused": tftypes.NewValue(tftypes.Bool, testCase.planPause),
				"description":     tftypes.NewValue(tftypes.String, "a new description"),
			}
			maps.Copy(prior, shared)
			maps.Copy(planned, shared)

			priorRaw := nullObjectWith(ctx, t, sch.Type(), prior)
			plannedRaw := nullObjectWith(ctx, t, sch.Type(), planned)
			req := fwresource.UpdateRequest{
				Plan:   tfsdk.Plan{Schema: sch, Raw: plannedRaw},
				State:  tfsdk.State{Schema: sch, Raw: priorRaw},
				Config: tfsdk.Config{Schema: sch, Raw: plannedRaw},
			}
			resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: priorRaw}}

			cq.Update(ctx, req, &resp)

			if got := requests.Load(); got != int64(testCase.wantRequests) {
				t.Fatalf("Made %d requests, want %d", got, testCase.wantRequests)
			}
			if !diagnosticsContain(resp.Diagnostics, "Unable to update Cluster Queue") {
				t.Fatalf("Update() diagnostics = %v, want the queue update failure reported", resp.Diagnostics)
			}

			var persisted clusterQueueResourceModel
			if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
				t.Fatalf("Reading the persisted state = %v", diags)
			}
			if got := persisted.DispatchPaused.ValueBool(); got != testCase.wantPause {
				t.Errorf("Persisted dispatch_paused = %t, want %t: state has to describe the queue the failure left behind", got, testCase.wantPause)
			}
		})
	}
}

// retry_agent_affinity is a REST call made after the queue mutation, so a failure there arrives with
// the description and the instance shape already applied. Recording those only after the REST call
// returned left state describing a queue that no longer existed in that form.
func TestClusterQueueUpdatePersistsTheMutationWhenTheAffinityUpdateFails(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t,
		// updateClusterQueue applies, with both the new description and the new instance shape.
		stubResponse{status: http.StatusOK, body: `{"data":{"clusterQueueUpdate":{"clusterQueue":{
			"id": "queue-id",
			"uuid": "queue-uuid",
			"key": "a-queue",
			"description": "a new description",
			"cluster": {"id": "cluster-id", "uuid": "cluster-uuid"},
			"hosted": true,
			"hostedAgents": {
				"instanceShape": {"name": "LINUX_AMD64_4X16"},
				"platformSettings": {"linux": {"agentImageRef": "an-image-ref"}}
			},
			"dispatchPaused": false
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
	sch := resourceSchema(ctx, t, cq)

	shared := map[string]tftypes.Value{
		"id":              tftypes.NewValue(tftypes.String, "queue-id"),
		"uuid":            tftypes.NewValue(tftypes.String, "queue-uuid"),
		"cluster_id":      tftypes.NewValue(tftypes.String, "cluster-id"),
		"cluster_uuid":    tftypes.NewValue(tftypes.String, "cluster-uuid"),
		"key":             tftypes.NewValue(tftypes.String, "a-queue"),
		"dispatch_paused": tftypes.NewValue(tftypes.Bool, false),
	}
	prior := map[string]tftypes.Value{
		"description":          tftypes.NewValue(tftypes.String, "an old description"),
		"retry_agent_affinity": tftypes.NewValue(tftypes.String, RetryAgentAffinityPreferWarmest),
		"hosted_agents":        hostedAgentsValue(ctx, t, sch.Type(), LinuxAMD64InstanceSmall),
	}
	planned := map[string]tftypes.Value{
		"description":          tftypes.NewValue(tftypes.String, "a new description"),
		"retry_agent_affinity": tftypes.NewValue(tftypes.String, RetryAgentAffinityPreferDifferent),
		"hosted_agents":        hostedAgentsValue(ctx, t, sch.Type(), LinuxAMD64InstanceMedium),
	}
	maps.Copy(prior, shared)
	maps.Copy(planned, shared)

	priorRaw := nullObjectWith(ctx, t, sch.Type(), prior)
	plannedRaw := nullObjectWith(ctx, t, sch.Type(), planned)
	req := fwresource.UpdateRequest{
		Plan:   tfsdk.Plan{Schema: sch, Raw: plannedRaw},
		State:  tfsdk.State{Schema: sch, Raw: priorRaw},
		Config: tfsdk.Config{Schema: sch, Raw: plannedRaw},
	}
	resp := fwresource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: priorRaw}}

	cq.Update(ctx, req, &resp)

	if got := requests.Load(); got < 2 {
		t.Fatalf("Made %d requests, want the queue update to have applied before the failure", got)
	}
	if !diagnosticsContain(resp.Diagnostics, "Unable to update retry_agent_affinity") {
		t.Fatalf("Update() diagnostics = %v, want the affinity failure reported", resp.Diagnostics)
	}

	var persisted clusterQueueResourceModel
	if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
		t.Fatalf("Reading the persisted state = %v", diags)
	}
	if got := persisted.Description.ValueString(); got != "a new description" {
		t.Errorf("Persisted description = %q, want %q: the mutation applied it before the REST call failed", got, "a new description")
	}
	if persisted.HostedAgents == nil {
		t.Fatal("Persisted hosted_agents = nil, want the shape the mutation applied")
	}
	if got := persisted.HostedAgents.InstanceShape.ValueString(); got != LinuxAMD64InstanceMedium {
		t.Errorf("Persisted instance_shape = %q, want %q: the mutation applied it before the REST call failed", got, LinuxAMD64InstanceMedium)
	}
	if got := persisted.RetryAgentAffinity.ValueString(); got != RetryAgentAffinityPreferWarmest {
		t.Errorf("Persisted retry_agent_affinity = %q, want the prior %q: the PATCH failed, so nothing changed it", got, RetryAgentAffinityPreferWarmest)
	}
}

// Setting retry_agent_affinity is a REST call made after the queue has been created. Create used to
// persist on that failure with a bare resp.State.Set whose diagnostics went nowhere; it now shares
// the deferred write with every other path out. Either way the queue exists, and dropping the write
// would leave it running with nothing in state pointing at it, recoverable only by terraform import.
func TestClusterQueueCreatePersistsTheQueueWhenTheAffinityUpdateFails(t *testing.T) {
	t.Parallel()

	server, requests := newRetryStub(t,
		// createClusterQueue applies, hosted agent settings and all.
		stubResponse{status: http.StatusOK, body: `{"data":{"clusterQueueCreate":{"clusterQueue":{
			"id": "queue-id",
			"uuid": "queue-uuid",
			"key": "a-queue",
			"description": "a description",
			"cluster": {"id": "cluster-id", "uuid": "cluster-uuid"},
			"hosted": true,
			"hostedAgents": {
				"instanceShape": {"name": "LINUX_AMD64_2X4"},
				"platformSettings": {"linux": {"agentImageRef": "an-image-ref"}}
			}
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
	sch := resourceSchema(ctx, t, cq)

	planned := nullObjectWith(ctx, t, sch.Type(), map[string]tftypes.Value{
		"cluster_id":           tftypes.NewValue(tftypes.String, "cluster-id"),
		"key":                  tftypes.NewValue(tftypes.String, "a-queue"),
		"description":          tftypes.NewValue(tftypes.String, "a description"),
		"retry_agent_affinity": tftypes.NewValue(tftypes.String, RetryAgentAffinityPreferDifferent),
		"dispatch_paused":      tftypes.NewValue(tftypes.Bool, false),
		"hosted_agents":        hostedAgentsValue(ctx, t, sch.Type(), LinuxAMD64InstanceSmall),
	})

	req := fwresource.CreateRequest{
		Plan:   tfsdk.Plan{Schema: sch, Raw: planned},
		Config: tfsdk.Config{Schema: sch, Raw: planned},
	}
	resp := fwresource.CreateResponse{State: tfsdk.State{Schema: sch, Raw: tftypes.NewValue(sch.Type().TerraformType(ctx), nil)}}

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
		t.Errorf("Persisted cluster_uuid = %q, want %q: Read and Update reach the REST API through it", persisted.ClusterUuid.ValueString(), "cluster-uuid")
	}
	if persisted.DispatchPaused.ValueBool() {
		t.Error("Persisted dispatch_paused = true, want false: the queue is created with dispatch running")
	}
	// The queue was created with these, and the failed PATCH did not touch either. Recording them
	// only after the PATCH left hosted_agents null, which the RequiresReplaceIf on the attribute
	// reads as the block having been added, so an untainted queue then plans a needless replace.
	if got := persisted.RetryAgentAffinity.ValueString(); got != RetryAgentAffinityPreferWarmest {
		t.Errorf("Persisted retry_agent_affinity = %q, want %q: the PATCH failed, so the queue is still on the API default", got, RetryAgentAffinityPreferWarmest)
	}
	if persisted.HostedAgents == nil {
		t.Fatal("Persisted hosted_agents = nil, want the settings the queue was created with")
	}
	if got := persisted.HostedAgents.InstanceShape.ValueString(); got != LinuxAMD64InstanceSmall {
		t.Errorf("Persisted instance_shape = %q, want %q", got, LinuxAMD64InstanceSmall)
	}
}

// A failed retry_agent_affinity read used to write the schema default into state. That is the one
// value it must not write: a queue configured prefer-warmest whose real setting is prefer-different
// then agrees with its own configuration, so no plan ever shows the drift.
func TestClusterQueueReadClassifiesAFailedAffinityRead(t *testing.T) {
	t.Parallel()

	queueFound := stubResponse{status: http.StatusOK, body: `{"data":{"node":{
		"__typename": "ClusterQueue",
		"id": "queue-id",
		"uuid": "queue-uuid",
		"key": "a-queue",
		"description": "a description",
		"cluster": {"id": "cluster-id", "uuid": "cluster-uuid"},
		"hosted": false,
		"dispatchPaused": false
	}}}`}

	tests := []struct {
		name string
		// The affinity GET's answer, after the queue itself was read successfully.
		affinity stubResponse
		// What the last successful read recorded. Empty means state holds no value for it yet.
		priorAffinity string
		// Empty means the read has to succeed with a warning instead.
		wantError string
	}{
		{
			// A token that reads the queue over GraphQL but lacks the REST cluster scope. The queue
			// is readable, so failing the whole refresh over one attribute is worse than saying so.
			name:          "the affinity read is refused",
			affinity:      stubResponse{status: http.StatusForbidden, body: `{"message":"Forbidden"}`},
			priorAffinity: RetryAgentAffinityPreferDifferent,
		},
		{
			// An import records only the id and the cluster uuid, so there is nothing to keep. The
			// schema default is the one value a refused read must not stand in with, and leaving it
			// null plans that default against a null prior and fails the apply on the same refusal.
			name:      "the affinity read is refused and state holds no previous value",
			affinity:  stubResponse{status: http.StatusForbidden, body: `{"message":"Forbidden"}`},
			wantError: "state holds no previous value",
		},
		{
			name:          "the affinity read fails for any other reason",
			affinity:      stubResponse{status: http.StatusInternalServerError, body: `{"message":"boom"}`},
			priorAffinity: RetryAgentAffinityPreferDifferent,
			wantError:     "Unable to read retry_agent_affinity",
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			server, _ := newRetryStub(t, queueFound, testCase.affinity)
			defer server.Close()

			cq := &clusterQueueResource{client: newRetryTestClient(t, server.URL, 0, time.Millisecond)}

			ctx := t.Context()
			sch := resourceSchema(ctx, t, cq)

			// A null affinity stands in for the state an import leaves behind.
			affinity := tftypes.NewValue(tftypes.String, nil)
			if testCase.priorAffinity != "" {
				affinity = tftypes.NewValue(tftypes.String, testCase.priorAffinity)
			}

			prior := nullObjectWith(ctx, t, sch.Type(), map[string]tftypes.Value{
				"id":           tftypes.NewValue(tftypes.String, "queue-id"),
				"uuid":         tftypes.NewValue(tftypes.String, "queue-uuid"),
				"cluster_id":   tftypes.NewValue(tftypes.String, "cluster-id"),
				"cluster_uuid": tftypes.NewValue(tftypes.String, "cluster-uuid"),
				"key":          tftypes.NewValue(tftypes.String, "a-queue"),
				// What the last successful read recorded, and what a failed read must not overwrite.
				"retry_agent_affinity": affinity,
			})

			req := fwresource.ReadRequest{State: tfsdk.State{Schema: sch, Raw: prior}}
			resp := fwresource.ReadResponse{State: tfsdk.State{Schema: sch, Raw: prior}}

			cq.Read(ctx, req, &resp)

			if testCase.wantError != "" {
				if !diagnosticsContain(resp.Diagnostics, testCase.wantError) {
					t.Fatalf("Read() diagnostics = %v, want %q: the API never answered, so the value is unknown", resp.Diagnostics, testCase.wantError)
				}
				return
			}

			if resp.Diagnostics.HasError() {
				t.Fatalf("Read() diagnostics = %v, want the refusal reported as a warning", resp.Diagnostics)
			}

			var persisted clusterQueueResourceModel
			if diags := resp.State.Get(ctx, &persisted); diags.HasError() {
				t.Fatalf("Reading the persisted state = %v", diags)
			}
			if got := persisted.RetryAgentAffinity.ValueString(); got != testCase.priorAffinity {
				t.Errorf("Persisted retry_agent_affinity = %q, want the last known %q: the read was refused, not answered", got, testCase.priorAffinity)
			}
		})
	}
}
