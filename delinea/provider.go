package delinea

import (
	"context"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSProvider defines the provider implementation
type TSSProvider struct{}

// TSSProviderModel defines the provider schema
type TSSProviderModel struct {
	ServerURL types.String `tfsdk:"server_url"`
	Username  types.String `tfsdk:"username"`
	Password  types.String `tfsdk:"password"`
	Domain    types.String `tfsdk:"domain"`
}

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
				Description: "The Secret Server base URL",
			},
			"username": schema.StringAttribute{
				Required:    true,
				Description: "The username for authentication",
			},
			"password": schema.StringAttribute{
				Required:    true,
				Sensitive:   true,
				Description: "The password for authentication",
			},
			"domain": schema.StringAttribute{
				Optional:    true,
				Description: "The domain for authentication",
			},
		},
	}
}

// Configure initializes the provider configuration
func (p *TSSProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config TSSProviderModel

	// Read configuration
	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create server configuration
	serverConfig := server.Configuration{
		ServerURL: config.ServerURL.ValueString(),
		Credentials: server.UserCredential{
			Username: config.Username.ValueString(),
			Password: config.Password.ValueString(),
			Domain:   config.Domain.ValueString(),
		},
	}

	resp.DataSourceData = serverConfig
	resp.ResourceData = serverConfig
}

// Resources returns the resources supported by the provider
func (p *TSSProvider) Resources(ctx context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		func() resource.Resource { return &TSSSecretResource{} },
	}
}

// DataSources returns the data sources supported by the provider
func (p *TSSProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		func() datasource.DataSource { return &TSSSecretDataSource{} },
	}
}

// New returns a new instance of the provider
func New() provider.Provider {
	return &TSSProvider{}
}
