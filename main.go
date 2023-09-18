package main

import (
	"flag"
	"log"

	"github.com/buildkite/terraform-provider-buildkite/buildkite"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
)

// Set at compile time from ldflags
var (
	version string
)

func main() {
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	var serveOpts []tf6server.ServeOpt

	if debug {
		serveOpts = append(serveOpts, tf6server.WithManagedDebug())
	}

	err := tf6server.Serve(
		"registry.terraform.io/buildkite/buildkite",
		providerserver.NewProtocol6(buildkite.New(version)),
		serveOpts...,
	)

	if err != nil {
		log.Fatal(err)
	}
}
