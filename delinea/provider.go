package delinea

import (
	"context"
	"log"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Define the provider structure
type TSSProvider struct{}

// Define the provider schema model
type TSSProviderModel struct {
	ServerURL types.String `tfsdk:"server_url"`
	Username  types.String `tfsdk:"username"`
	Password  types.String `tfsdk:"password"`
	Domain    types.String `tfsdk:"domain"`
}

// Ensure the provider implements the ProviderWithEphemeralResources interface
var _ provider.ProviderWithEphemeralResources = (*TSSProvider)(nil)

// Metadata returns the provider type name
func (p *TSSProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "tss"
}

// Schema defines the provider-level schema
func (p *TSSProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"server_url": schema.StringAttribute{
				Required:    true,
				Description: "The Secret Server base URL e.g. https://localhost/SecretServer",
			},
			"username": schema.StringAttribute{
				Required:    true,
				Description: "The username of the Secret Server User to connect as",
			},
			"password": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "The password of the Secret Server User",
			},
			"domain": schema.StringAttribute{
				Optional:    true,
				Description: "Domain of the Secret Server user",
			},
		},
	}
}

// Configure initializes the provider with the given configuration
func (p *TSSProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config TSSProviderModel

	// Log the start of the Configure method
	log.Printf("Starting Configure method")

	// Read configuration values into the config struct
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError("Configuration Error", "Failed to read provider configuration")
		log.Printf("Failed to read provider configuration", map[string]interface{}{
			"diagnostics": resp.Diagnostics,
		})
		return
	}

	// Log the configuration values
	log.Printf("Provider configuration values retrieved", map[string]interface{}{
		"server_url": config.ServerURL.ValueString(),
		"username":   config.Username.ValueString(),
	})

	// Create the server configuration
	serverConfig := &server.Configuration{
		ServerURL: config.ServerURL.ValueString(),
		Credentials: server.UserCredential{
			Username: config.Username.ValueString(),
			Password: config.Password.ValueString(),
			Domain:   config.Domain.ValueString(),
		},
	}

	// Pass the server configuration to resources and data sources
	if serverConfig == nil {
		log.Printf("Server configuration is nil")
		resp.Diagnostics.AddError("Configuration Error", "Server configuration is nil")
		return
	}
	resp.DataSourceData = serverConfig
	resp.ResourceData = serverConfig
	resp.EphemeralResourceData = serverConfig
}

// DataSources returns the data sources supported by the provider
func (p *TSSProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		func() datasource.DataSource { return &TSSSecretDataSource{} },
		func() datasource.DataSource { return &TSSSecretsDataSource{} },
	}
}

// Resources returns the resources supported by the provider
func (p *TSSProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		func() resource.Resource { return &TSSSecretResource{} },
		func() resource.Resource {
			return &TSSSecretDeletionResource{}
		},
		//For the DEBUG environment, uncomment this line to unit test whether the secret value is being fetched successfully.
		//func() resource.Resource { return &PrintSecretResource{} },
	}
}

func (p *TSSProvider) EphemeralResources(_ context.Context) []func() ephemeral.EphemeralResource {
	return []func() ephemeral.EphemeralResource{
		func() ephemeral.EphemeralResource {
			return &TSSSecretEphemeralResource{}
		},
		func() ephemeral.EphemeralResource {
			return &TSSSecretsEphemeralResource{}
		},
	}
}

// New returns a new instance of the provider
func New() provider.Provider {
	return &TSSProvider{}
}
