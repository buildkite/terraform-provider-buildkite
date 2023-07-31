package buildkite

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func testAccClusterQueueConfigBasic(name string) string {
	config := `
	
	resource "buildkite_cluster_queue" "foobar" {
		cluster_id = "Q2x1c3Rlci0tLTMzMDc1ZDhiLTMyMjctNDRkYS05YTk3LTkwN2E2NWZjOGFiNg=="
		description = "Acceptance Test %s"
		key = "foobar"
	}
	`

	return fmt.Sprintf(config, name)
}

// Confirm that we can create a new cluster queue, and then delete it without error
func TestAccClusterQueue_add_remove(t *testing.T) {
	var cq ClusterQueueResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterQueueDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterQueueConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the cluster queue exists in the buildkite API
					testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
					// Confirm the cluster queue has the correct values in Buildkite's system
					testAccCheckClusterQueueRemoteValues(&cq, "Acceptance Test foo", "foobar"),
					// Confirm the cluster queue has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "key", "foobar"),
					resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", "Acceptance Test foo"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_queue.foobar", "id"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_queue.foobar", "uuid"),
				),
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
}

func TestAccClusterQueue_update(t *testing.T) {
	var cq ClusterQueueResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterQueueDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterQueueConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the cluster queue exists in the buildkite API
					testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
					// Confirm the cluster queue has the correct values in Buildkite's system
					testAccCheckClusterQueueRemoteValues(&cq, "Acceptance Test foo", "foobar"),
					// Confirm the cluster queue has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", "Acceptance Test foo"),
				),
			},
			{
				Config: testAccClusterQueueConfigBasic("bar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the cluster queue exists in the buildkite API
					testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
					// Confirm the cluster queue has the correct values in Buildkite's system
					testAccCheckClusterQueueRemoteValues(&cq, "Acceptance Test bar", "foobar"),
					// Confirm the cluster queue has the correct values in terraform state
					resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", "Acceptance Test bar"),
				),
			},
		},
	})
}

func TestAccClusterQueue_import(t *testing.T) {
	var cq ClusterQueueResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterQueueDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterQueueConfigBasic("foo"),
				Check: resource.ComposeAggregateTestCheckFunc(
					// Confirm the cluster queue exists in the buildkite API
					testAccCheckClusterQueueExists("buildkite_cluster_queue.foobar", &cq),
					// Check to confirm the local state is correct before we re-import it
					resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "key", "foobar"),
					resource.TestCheckResourceAttr("buildkite_cluster_queue.foobar", "description", "Acceptance Test foo"),
				),
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
}

func testAccCheckClusterQueueExists(resourceName string, clusterQueueResourceModel *ClusterQueueResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		// Setup variables
		var queueFound bool
		var cursor *string

		for {
			queues, err := getClusterQueues(
				genqlientGraphql,
				getenv("BUILDKITE_ORGANIZATION_SLUG"),
				resourceState.Primary.Attributes["cluster_uuid"],
				cursor,
			)

			if err != nil {
				return fmt.Errorf("Unable to read Cluster Queues: %v", err)
			}

			// Loop over the returned page of cluster queues to see if the queue is found
			for _, queue := range queues.Organization.Cluster.Queues.Edges {
				if queue.Node.Id == resourceState.Primary.ID {
					queueFound = true
					// Update ClusterQueueResourceModel with Node values and append
					updateClusterQueueResource(queue.Node, clusterQueueResourceModel)
					break
				}
			}

			// Stop the do-while for loop if the queue was found or no more pages from the API response
			if queueFound || !queues.Organization.Cluster.Queues.PageInfo.HasNextPage {
				break
			}

			// Update cursor with next page
			cursor = &queues.Organization.Cluster.Queues.PageInfo.EndCursor
		}

		if !queueFound {
			return fmt.Errorf("Unable to find Cluster Queue %s in cluster %s",
				resourceState.Primary.ID,
				resourceState.Primary.Attributes["cluster_uuid"])
		}

		return nil
	}
}

func testAccCheckClusterQueueRemoteValues(cq *ClusterQueueResourceModel, description, key string) resource.TestCheckFunc {
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

func testAccGetImportClusterQueueId(cq *ClusterQueueResourceModel) resource.ImportStateIdFunc {
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

		// Setup variables
		var queueFound bool
		var cursor *string

		for {
			queues, err := getClusterQueues(
				genqlientGraphql,
				getenv("BUILDKITE_ORGANIZATION_SLUG"),
				rs.Primary.Attributes["cluster_uuid"],
				cursor,
			)

			if err != nil {
				return fmt.Errorf("Unable to read Cluster Queues: %v", err)
			}

			// Loop over the returned page of cluster queues to see if the queue is found
			for _, queue := range queues.Organization.Cluster.Queues.Edges {
				if queue.Node.Id == rs.Primary.ID {
					queueFound = true
					break
				}
			}

			// If the cluster queue is found, error - as we expect not to find it
			if queueFound {
				return fmt.Errorf("Cluster queue %s still exists in cluster %s, expected not to find it",
					rs.Primary.ID,
					rs.Primary.Attributes["cluster_uuid"],
				)
			}

			// If there are no more pages from the API response break (Cluster queue is deleted)
			if !queues.Organization.Cluster.Queues.PageInfo.HasNextPage {
				break
			} else {
				// Update cursor with the EndCursor (next page)
				cursor = &queues.Organization.Cluster.Queues.PageInfo.EndCursor
			}
		}

		// If we end up here, the Cluster queue was deleted
		return nil
	}
	return nil
}
