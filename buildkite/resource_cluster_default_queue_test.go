package buildkite

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccBuildkiteClusterDefaultQueueResource(t *testing.T) {
	t.Parallel()

	t.Run("attach a default queue to a cluster", func(t *testing.T) {
		clusterName := acctest.RandString(5)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterDestroy,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster" {
							name = "%s"
						}
						resource "buildkite_cluster_queue" "cluster" {
							key = "default"
							cluster_id = buildkite_cluster.cluster.id
						}
						resource "buildkite_cluster_default_queue" "cluster" {
							cluster_id = buildkite_cluster.cluster.id
							queue_id = buildkite_cluster_queue.cluster.id
						}
					`, clusterName),
					Check: func(s *terraform.State) error {
						cluster := s.RootModule().Resources["buildkite_cluster.cluster"].Primary
						queue := s.RootModule().Resources["buildkite_cluster_queue.cluster"].Primary
						// load the cluster from the api and ensure the correct queue is default
						r, err := getNode(context.Background(), genqlientGraphql, cluster.ID)
						if clusterNode, ok := r.GetNode().(*getNodeNodeCluster); ok {
							if clusterNode.DefaultQueue.Id != queue.ID {
								return errors.New("Default queue does not match")
							}
						}
						return err
					},
				},
			},
		})
	})

	t.Run("change default queue", func(t *testing.T) {
		clusterName := acctest.RandString(5)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			CheckDestroy:             testAccCheckClusterDestroy,
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster" {
							name = "%s"
						}
						resource "buildkite_cluster_queue" "cluster" {
							key = "default"
							cluster_id = buildkite_cluster.cluster.id
						}
						resource "buildkite_cluster_default_queue" "cluster" {
							cluster_id = buildkite_cluster.cluster.id
							queue_id = buildkite_cluster_queue.cluster.id
						}
					`, clusterName),
				},
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster" {
							name = "%s"
						}
						resource "buildkite_cluster_queue" "cluster" {
							key = "new"
							cluster_id = buildkite_cluster.cluster.id
						}
						resource "buildkite_cluster_default_queue" "cluster" {
							cluster_id = buildkite_cluster.cluster.id
							queue_id = buildkite_cluster_queue.cluster.id
						}
					`, clusterName),
					Check: func(s *terraform.State) error {
						cluster := s.RootModule().Resources["buildkite_cluster.cluster"].Primary
						queue := s.RootModule().Resources["buildkite_cluster_queue.cluster"].Primary
						// load the cluster from the api and ensure the correct queue is default
						r, err := getNode(context.Background(), genqlientGraphql, cluster.ID)
						if clusterNode, ok := r.GetNode().(*getNodeNodeCluster); ok {
							if clusterNode.DefaultQueue.Id != queue.ID {
								return errors.New("Default queue does not match")
							}
						}
						return err
					},
				},
			},
		})
	})
}
