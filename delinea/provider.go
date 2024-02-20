package delinea

import (
	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func providerConfig(d *schema.ResourceData) (interface{}, error) {
	return server.Configuration{
		ServerURL: d.Get("server_url").(string),
		Credentials: server.UserCredential{
			Username: d.Get("username").(string),
			Password: d.Get("password").(string),
			Domain:   d.Get("domain").(string),
		},
	}, nil
}

// Provider is a Terraform DataSource
func Provider() *schema.Provider {
	return &schema.Provider{
		DataSourcesMap: map[string]*schema.Resource{
			"tss_secret": dataSourceSecret(),
		},
		Schema: map[string]*schema.Schema{
			"server_url": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("TSS_SERVER_URL", nil),
				Description: "The Secret Server base URL e.g. https://localhost/SecretServer",
			},
			"username": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("TSS_USERNAME", nil),
				Description: "The username of the Secret Server User to connect as",
			},
			"password": {
				Type:        schema.TypeString,
				Required:    true,
				DefaultFunc: schema.EnvDefaultFunc("TSS_PASSWORD", nil),
				Description: "The password of the Secret Server User",
			},
			"domain": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("TSS_DOMAIN", nil),
				Description: "Domain of the Server Server user",
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"tss_resource_secret": resourceSecret(),
		},
		ConfigureFunc: providerConfig,
	}
}
