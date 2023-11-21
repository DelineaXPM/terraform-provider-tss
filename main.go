package main

import (
	"github.com/DelineaXPM/terraform-provider-tss/v2/delinea"
	"github.com/hashicorp/terraform-plugin-sdk/plugin"
	"github.com/hashicorp/terraform-plugin-sdk/terraform"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: func() terraform.ResourceProvider {
			return delinea.Provider()
		},
	})
}
