package buildkite

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
)

var testAccProviders map[string]*schema.Provider
var testAccProvider *schema.Provider

func init() {
	testAccProvider = Provider()
	testAccProviders = map[string]*schema.Provider{
		"buildkite": testAccProvider,
	}
}

// TestProvider just does basic validation to ensure the schema is defined and supporting functions exist
func TestProvider(t *testing.T) {
	if err := Provider().InternalValidate(); err != nil {
		t.Fatalf("err: %s", err)
	}
}

func TestProvider_impl(t *testing.T) {
	var _ *schema.Provider = Provider()
}

func testAccPreCheck(t *testing.T) {
	if v := os.Getenv("BUILDKITE_ORGANIZATION"); v == "" {
		t.Fatal("BUILDKITE_ORGANIZATION must be set for acceptance tests")
	}
	if v := os.Getenv("BUILDKITE_API_TOKEN"); v == "" {
		t.Fatal("BUILDKITE_API_TOKEN must be set for acceptance tests")
	}
}

// testAccCheckExampleResourceDestroy verifies the Widget
// has been destroyed
func testAccCheckExampleResourceDestroy(s *terraform.State) error {
	// TODO manually check that all resources created during acceptance tests have been cleaned up
	return nil
}
