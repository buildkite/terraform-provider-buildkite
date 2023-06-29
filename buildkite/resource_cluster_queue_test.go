package buildkite

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

func testAccClusterQueueConfigBasic(name string) string {
	config := `
	
	resource "buildkite_cluster" "clusterfoo" {
		description = "Acceptance Test Cluster"
	}
	
	resource "buildkite_cluster_queue" "queuebar" {
		cluster_id = buildkite_cluster.clusterfoo.id
		description = "Acceptance Test %s"
		key: "queuebar"
	}
	`

	return fmt.Sprintf(config, name)
}

// Confirm that we can create a new Cluster Queue, and then delete it without error
func TestAccClusterQueue_add_remove(t *testing.T) {
	t.Parallel()
	var cq ClusterQueueResourceModel

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: protoV6ProviderFactories(),
		CheckDestroy:             testAccCheckClusterQueueDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccClusterQueueConfigBasic("queuebar"),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckClusterQueueRemoteValues(&cq, "Acceptance Test queuebar"),
				),
			},
			{
				RefreshState: true,
				PlanOnly:     true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("buildkite_cluster_queue.queuebar", "key"),
					resource.TestCheckResourceAttrSet("buildkite_cluster_queue.queuebar", "description"),
				),
			},
		},
	})
}

func testAccCheckClusterQueueRemoteValues(cq *ClusterQueueResourceModel, description string) resource.TestCheckFunc {
	return func(s *terraform.State) error {

		if string(cq.Description.ValueString()) != description {
			return fmt.Errorf("Unexpected description: %s", cq.Description)
		}
		return nil
	}
}

func testAccCheckClusterQueueDestroy(s *terraform.State) error {
	return nil
}