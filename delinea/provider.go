package delinea

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/DelineaXPM/delinea-common/api"
	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	ServerURL         types.String `tfsdk:"server_url"`
	Username          types.String `tfsdk:"username"`
	Password          types.String `tfsdk:"password"`
	Token             types.String `tfsdk:"token"`
	Domain            types.String `tfsdk:"domain"`
	AllowInsecureHTTP types.Bool   `tfsdk:"allow_insecure_http"`
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
			"allow_insecure_http": schema.BoolAttribute{
				Optional:    true,
				Description: "Permit a plaintext http:// server_url to a non-loopback host. Defaults to false; plaintext HTTP exposes the credential on the wire, so set this only after accepting that risk. May also be supplied via the TSS_ALLOW_INSECURE_HTTP environment variable.",
			},
		},
	}
}

// Configure initializes the provider with the given configuration
func (p *TSSProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config TSSProviderModel

	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError("Configuration Error", "Failed to read provider configuration")
		return
	}

	resolved, errs := resolveProviderConfig(config, os.Getenv)
	for _, e := range errs {
		resp.Diagnostics.AddError("Configuration Error", e)
	}
	if resp.Diagnostics.HasError() {
		return
	}

	serverConfig := server.Configuration{
		ServerURL:         resolved.ServerURL.ValueString(),
		AllowInsecureHTTP: resolved.AllowInsecureHTTP.ValueBool(),
		Credentials: server.UserCredential{
			Username: resolved.Username.ValueString(),
			Password: resolved.Password.ValueString(),
			Token:    resolved.Token.ValueString(),
			Domain:   resolved.Domain.ValueString(),
		},
	}

	// One shared client for every resource and data source: the v3 SDK runs
	// a backend probe per constructed Server, so per-operation construction
	// would pay that probe round trip on every CRUD call.
	client, err := server.New(serverConfig)
	if err != nil {
		resp.Diagnostics.AddError("Configuration Error", "Failed to create Secret Server client: "+err.Error())
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
	resp.EphemeralResourceData = client
}

// resolveProviderConfig fills in unset config fields from environment
// variables and validates the resulting credentials. Documented precedence
// is explicit config > environment variable > unset. Closes gh #108.
//
// Returns the resolved config and a slice of error messages (empty on
// success). Pulled out of Configure for unit-testability — getenv is
// injected so tests don't need to mutate process env.
func resolveProviderConfig(config TSSProviderModel, getenv func(string) string) (TSSProviderModel, []string) {
	// A set-but-unknown value (e.g. derived from a resource not yet applied)
	// must not be silently replaced by an env var — that would connect to
	// whatever server or identity the operator's shell points at. One table
	// drives both the unknown-value guard and the env fallback for the string
	// attributes, so joining the fallback structurally joins the guard;
	// allow_insecure_http is a Bool and is guarded separately below with the
	// same error text via unknownAttrErr.
	unknownAttrErr := func(name, envVar, kind string) string {
		return "The `" + name + "` provider attribute is set to a value that is not known at configure time (for example, derived from a resource that has not been applied yet). Set " + kind + ", use the " + envVar + " environment variable instead, or apply the value's source first."
	}
	stringAttrs := []struct {
		name, envVar string
		value        *types.String
	}{
		{"server_url", "TSS_SERVER_URL", &config.ServerURL},
		{"username", "TSS_USERNAME", &config.Username},
		{"password", "TSS_PASSWORD", &config.Password},
		{"token", "TSS_TOKEN", &config.Token},
		{"domain", "TSS_DOMAIN", &config.Domain},
	}

	var errs []string
	for _, a := range stringAttrs {
		if a.value.IsUnknown() {
			errs = append(errs, unknownAttrErr(a.name, a.envVar, "a static value"))
		}
	}
	if config.AllowInsecureHTTP.IsUnknown() {
		errs = append(errs, unknownAttrErr("allow_insecure_http", "TSS_ALLOW_INSECURE_HTTP", "a static true/false"))
	}
	if len(errs) > 0 {
		return config, errs
	}

	for _, a := range stringAttrs {
		if !a.value.IsNull() && a.value.ValueString() != "" {
			continue
		}
		if v := getenv(a.envVar); v != "" {
			*a.value = types.StringValue(v)
		}
	}

	if config.AllowInsecureHTTP.IsNull() {
		if v := getenv("TSS_ALLOW_INSECURE_HTTP"); v != "" {
			parsed, err := strconv.ParseBool(v)
			if err != nil {
				errs = append(errs, "TSS_ALLOW_INSECURE_HTTP must be a boolean value (true/false/1/0), got: "+v)
			} else {
				config.AllowInsecureHTTP = types.BoolValue(parsed)
			}
		}
	}

	hasServerURL := !config.ServerURL.IsNull() && config.ServerURL.ValueString() != ""
	hasUsername := !config.Username.IsNull() && config.Username.ValueString() != ""
	hasPassword := !config.Password.IsNull() && config.Password.ValueString() != ""
	hasToken := !config.Token.IsNull() && config.Token.ValueString() != ""

	if !hasServerURL {
		errs = append(errs, "Server URL is required: set the `server_url` provider attribute or the TSS_SERVER_URL environment variable.")
	}
	switch {
	case hasToken && (hasUsername || hasPassword):
		errs = append(errs, "Provide either username/password OR token, not both. Set exactly one of: (username + password) or token; values may come from provider config or the corresponding TSS_* environment variables.")
	case hasUsername && !hasPassword:
		errs = append(errs, "Username is set but password is missing. Set the `password` provider attribute or the TSS_PASSWORD environment variable.")
	case !hasUsername && hasPassword:
		errs = append(errs, "Password is set but username is missing. Set the `username` provider attribute or the TSS_USERNAME environment variable.")
	case !hasUsername && !hasToken:
		errs = append(errs, "Credentials missing: set username + password or token. Values may come from provider config or the corresponding TSS_* environment variables (TSS_USERNAME / TSS_PASSWORD / TSS_TOKEN).")
	}

	// Validate the URL at configure time with the same delinea-common
	// validator tss-sdk-go's server.New uses, so plaintext-http and URL-shape
	// failures surface before apply starts creating resources, with the
	// allow_insecure_http attribute named instead of the SDK option. The
	// unknown-value guard above guarantees AllowInsecureHTTP is known here.
	if hasServerURL {
		if _, err := api.NormalizeURL(config.ServerURL.ValueString(), config.AllowInsecureHTTP.ValueBool()); err != nil {
			msg := "Invalid server URL: " + err.Error() + "."
			// Probe whether the opt-in alone would make the URL acceptable,
			// so the hint doesn't depend on the upstream error's prose.
			if !config.AllowInsecureHTTP.ValueBool() {
				if _, retryErr := api.NormalizeURL(config.ServerURL.ValueString(), true); retryErr == nil {
					msg += " In this provider, set `allow_insecure_http = true` (or TSS_ALLOW_INSECURE_HTTP=true) only after accepting that risk."
				}
			}
			errs = append(errs, msg)
		}
	}

	return config, errs
}

// clientFromProviderData extracts the shared SDK client Configure stored on
// the provider. A nil ProviderData (provider not configured yet — the
// framework calls Configure on dependents before that happens) returns nil
// with no diagnostic; an unexpected type is a diagnostic.
func clientFromProviderData(data any, diags *diag.Diagnostics) *server.Server {
	if data == nil {
		return nil
	}
	client, ok := data.(*server.Server)
	if !ok {
		diags.AddError("Configuration Error", fmt.Sprintf("Expected *server.Server in provider data, got %T", data))
		return nil
	}
	return client
}

// requireClient reports whether the shared client is available, adding the
// standard provider-not-configured diagnostic when it is not.
func requireClient(client *server.Server, diags *diag.Diagnostics) bool {
	if client == nil {
		diags.AddError("Provider Not Configured", "The provider must be configured before this operation; the Secret Server client is not available.")
		return false
	}
	return true
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
		func() resource.Resource { return &TSSSecretDeletionResource{} },
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
