package buildkite

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccBuildkiteClusterDatasource(t *testing.T) {
	t.Run("timeout reading cluster", func(t *testing.T) {
		t.Skip()
		clusterName := acctest.RandString(12)

		resource.ParallelTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						provider "buildkite" {
							timeouts = {
								read = "0s"
							}
						}
						resource "buildkite_cluster" "cluster" {
							name = "%s"
						}
						data "buildkite_cluster" "default" {
							name = buildkite_cluster.cluster.name
						}`, clusterName),
					ExpectError: regexp.MustCompile(`timeout while waiting for state to become 'success'`),
				},
			},
		})
	})

	t.Run("can find a cluster", func(t *testing.T) {
		clusterName := acctest.RandString(12)
		resource.ParallelTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: fmt.Sprintf(`
						resource "buildkite_cluster" "cluster" {
							name = "%s"
							color = "#f1efff"
						}
						data "buildkite_cluster" "cluster" {
								name = buildkite_cluster.cluster.name
						}`, clusterName),
					Check: resource.ComposeTestCheckFunc(
						resource.TestCheckResourceAttrPair("data.buildkite_cluster.cluster", "id", "buildkite_cluster.cluster", "id"),
						resource.TestCheckResourceAttr("data.buildkite_cluster.cluster", "color", "#f1efff"),
					),
				},
			},
		})
	})

	t.Run("errors if cannot find cluster", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: `data "buildkite_cluster" "default" {
								name = "doesn't exist"
							}`,
					ExpectError: regexp.MustCompile("Unable to find Cluster"),
				},
			},
		})
	})
}
