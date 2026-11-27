// Terraform provider for Algizian (https://algizian.com): tenants, custom domains, OIDC clients,
// own provider apps, email sign-in settings and service accounts, through the management API.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/elmerisllc/terraform-provider-algizian/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name algizian

// version is set by the release build: -ldflags "-X main.version=1.2.3".
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers such as delve")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/elmerisllc/algizian",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
