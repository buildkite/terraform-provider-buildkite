package main

import (
	"context"
	"flag"
	"log"

	"github.com/buildkite/terraform-provider-buildkite/buildkite"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// Set at compile time from ldflags
var (
	version string
)

func main() {
	ctx := context.Background()

	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/buildkite/buildkite",
		Debug:   debug,
	}

	err := providerserver.Serve(ctx, buildkite.New(version), opts)

	if err != nil {
		log.Fatal(err.Error())
	}
}
