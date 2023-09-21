package buildkite

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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
						// load the cluster from the api and ensure the correct queue is default
						return nil
					},
				},
			},
		})
	})

	t.Run("fails to add default if one exists already", func(t *testing.T) {
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
							key = "default"
							cluster_id = buildkite_cluster.cluster.id
						}
						resource "buildkite_cluster_default_queue" "cluster" {
							cluster_id = buildkite_cluster.cluster.id
							queue_id = buildkite_cluster_queue.cluster.id
						}
						resource "buildkite_cluster_queue" "extra" {
							key = "extra"
							cluster_id = buildkite_cluster.cluster.id
						}
						resource "buildkite_cluster_default_queue" "extra" {
							cluster_id = buildkite_cluster.cluster.id
							queue_id = buildkite_cluster_queue.extra.id
						}
					`, clusterName),
					ConfigPlanChecks: resource.ConfigPlanChecks{
						PostApplyPostRefresh: []plancheck.PlanCheck{
							plancheck.ExpectResourceAction("buildkite_cluster_default_queue.extra", plancheck.ResourceActionCreate),
							plancheck.ExpectResourceAction("buildkite_cluster_default_queue.cluster", plancheck.ResourceActionNoop),
						},
					},
					ExpectError: regexp.MustCompile("Cluster already has a default"),
				},
			},
		})
	})
}
