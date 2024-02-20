package main

import (
	"github.com/DelineaXPM/terraform-provider-tss/v2/delinea"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
)

func main() {
	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: func() *schema.Provider {
			return delinea.Provider()
		},
	})
}
