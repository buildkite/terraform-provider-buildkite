package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func testAccClusterQueueConfigBasic(name string) string {
	config := `
	
	resource "buildkite_cluster_queue" "foobar" {
		cluster_id = "Q2x1c3Rlci0tLTFkNmIxOTg5LTJmYjctNDRlMC04MWYyLTAxYjIxNzQ4MTVkMg=="
		description = "Acceptance Test %s"
		key = "foobar"
	}
	`

	return fmt.Sprintf(config, name)
}

// Confirm that we can create a new cluster queue, and then delete it without error
func TestAccClusterQueue_add_remove(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func testAccCheckClusterQueueExists(resourceName string, clusterQueueResourceModel *ClusterQueueResourceModel) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		resourceState, ok := s.RootModule().Resources[resourceName]

		if !ok {
			return fmt.Errorf("Not found in state: %s", resourceName)
		}

		if resourceState.Primary.ID == "" {
			return fmt.Errorf("No ID is set in state")
		}

		// Obtain queues of the queue's cluster from its cluster UUID
		queues, err := getClusterQueues(
			genqlientGraphql,
			getenv("BUILDKITE_ORGANIZATION_SLUG"),
			resourceState.Primary.Attributes["cluster_uuid"],
		)

		// If cluster queues were not able to be fetched by Genqlient
		if err != nil {
			return fmt.Errorf("Error fetching Cluster queues from graphql API: %v", err)
		}

		// Obtain the ClusterQueueResourceModel from the queues slice
		for i := range queues.Organization.Cluster.Queues.Edges {
			if queues.Organization.Cluster.Queues.Edges[i].Node.Id == resourceState.Primary.ID {
				// Update ClusterQueueResourceModel with Node values and append
				updateClusterQueueResource(queues.Organization.Cluster.Queues.Edges[i].Node, clusterQueueResourceModel)
				break
			}
		}

		// If clusterQueueResourceModel isnt set from the queues slice
		if clusterQueueResourceModel.Id.ValueString() == "" {
			return fmt.Errorf("No Cluster queue found with graphql id: %s", resourceState.Primary.ID)
		}

		return nil
	}
}

func testAccCheckClusterQueueRemoteValues(cq *ClusterQueueResourceModel, description string, key string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if cq.Key.ValueString() != key {
			return fmt.Errorf("Remote Cluster queue key (%s) doesn't match expected value (%s)", cq.Description, description)
		}

		if cq.Description.ValueString() != description {
			return fmt.Errorf("Remote Cluster queue description (%s) doesn't match expected value (%s)", cq.Description, description)
		}

		return nil
	}
}

func testAccCheckClusterQueueDestroy(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "buildkite_cluster_queue" {
			continue
		}

		// Obtain queues of the queue's cluster from its cluster UUID
		queues, err := getClusterQueues(
			genqlientGraphql,
			getenv("BUILDKITE_ORGANIZATION_SLUG"),
			rs.Primary.Attributes["cluster_uuid"],
		)

		// If cluster queues were not able to be fetched by Genqlient
		if err != nil {
			return fmt.Errorf("Error fetching Cluster queues from graphql API: %v", err)
		}

		for i := range queues.Organization.Cluster.Queues.Edges {
			//If the cluster queue's ID matches any of the queues in the cluster, it hasnt been deleted
			if queues.Organization.Cluster.Queues.Edges[i].Node.Id == rs.Primary.ID {
				return fmt.Errorf("Cluster queue still exists in cluster, expected not to find it")
			}
		}

		return nil
	}
	return nil
}
