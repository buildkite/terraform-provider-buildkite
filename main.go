package main

import (
	"github.com/buildkite/terraform-provider-buildkite/buildkite"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
)

// Set at compile time from ldflags
var (
	version string
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: func() *schema.Provider {
			return buildkite.Provider(version)
		},
	})
}
