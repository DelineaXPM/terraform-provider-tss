package delinea

import (
	"context"
	"log"
	"os"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
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
	Token     types.String `tfsdk:"token"`
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
				Optional:    true,
				Description: "The Secret Server base URL e.g. https://localhost/SecretServer. May also be supplied via the TSS_SERVER_URL environment variable.",
			},
			"username": schema.StringAttribute{
				Optional:    true,
				Description: "The username of the Secret Server User to connect as. May also be supplied via the TSS_USERNAME environment variable.",
			},
			"password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The password of the Secret Server User. May also be supplied via the TSS_PASSWORD environment variable.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "A token to authenticate the Secret Server User. May also be supplied via the TSS_TOKEN environment variable.",
			},
			"domain": schema.StringAttribute{
				Optional:    true,
				Description: "Domain of the Secret Server user. May also be supplied via the TSS_DOMAIN environment variable.",
			},
		},
	}
}

// Configure initializes the provider with the given configuration
func (p *TSSProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config TSSProviderModel

	log.Printf("Starting Configure method")

	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError("Configuration Error", "Failed to read provider configuration")
		log.Printf("Failed to read provider configuration: %s", resp.Diagnostics)
		return
	}

	resolved, errs := resolveProviderConfig(config, os.Getenv)
	for _, e := range errs {
		resp.Diagnostics.AddError("Configuration Error", e)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Provider configuration values retrieved: server_url=%s username=%s",
		resolved.ServerURL.ValueString(), resolved.Username.ValueString())

	serverConfig := &server.Configuration{
		ServerURL: resolved.ServerURL.ValueString(),
		Credentials: server.UserCredential{
			Username: resolved.Username.ValueString(),
			Password: resolved.Password.ValueString(),
			Token:    resolved.Token.ValueString(),
			Domain:   resolved.Domain.ValueString(),
		},
	}

	resp.DataSourceData = serverConfig
	resp.ResourceData = serverConfig
	resp.EphemeralResourceData = serverConfig
}

// resolveProviderConfig fills in unset config fields from environment
// variables and validates the resulting credentials. Documented precedence
// is explicit config > environment variable > unset. Closes gh #108.
//
// Returns the resolved config and a slice of error messages (empty on
// success). Pulled out of Configure for unit-testability — getenv is
// injected so tests don't need to mutate process env.
func resolveProviderConfig(config TSSProviderModel, getenv func(string) string) (TSSProviderModel, []string) {
	fallback := func(current types.String, envVar string) types.String {
		if !current.IsNull() && current.ValueString() != "" {
			return current
		}
		if v := getenv(envVar); v != "" {
			return types.StringValue(v)
		}
		return current
	}

	config.ServerURL = fallback(config.ServerURL, "TSS_SERVER_URL")
	config.Username = fallback(config.Username, "TSS_USERNAME")
	config.Password = fallback(config.Password, "TSS_PASSWORD")
	config.Token = fallback(config.Token, "TSS_TOKEN")
	config.Domain = fallback(config.Domain, "TSS_DOMAIN")

	var errs []string
	hasServerURL := !config.ServerURL.IsNull() && config.ServerURL.ValueString() != ""
	hasUsername := !config.Username.IsNull() && config.Username.ValueString() != ""
	hasPassword := !config.Password.IsNull() && config.Password.ValueString() != ""
	hasToken := !config.Token.IsNull() && config.Token.ValueString() != ""

	if !hasServerURL {
		errs = append(errs, "Server URL is required: set the `server_url` provider attribute or the TSS_SERVER_URL environment variable.")
	}
	switch {
	case hasUsername && hasToken:
		errs = append(errs, "Provide either username/password OR token, not both. Set exactly one of: (username + password) or token; values may come from provider config or the corresponding TSS_* environment variables.")
	case !hasUsername && !hasToken:
		errs = append(errs, "Credentials missing: set username + password or token. Values may come from provider config or the corresponding TSS_* environment variables (TSS_USERNAME / TSS_PASSWORD / TSS_TOKEN).")
	case hasUsername && !hasPassword:
		errs = append(errs, "Username is set but password is missing. Set the `password` provider attribute or the TSS_PASSWORD environment variable.")
	case !hasUsername && hasPassword:
		errs = append(errs, "Password is set but username is missing. Set the `username` provider attribute or the TSS_USERNAME environment variable.")
	}

	return config, errs
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
