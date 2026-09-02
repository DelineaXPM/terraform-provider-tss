package delinea

import (
	"context"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretDataSource defines the data source implementation
type TSSSecretDataSource struct {
	client *server.Server // Shared SDK client built once in Provider.Configure
}

// Metadata provides the data source type name
func (d *TSSSecretDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "tss_secret"
}

// Schema defines the schema for the data source
func (d *TSSSecretDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the secret to retrieve.",
			},
			"field": schema.StringAttribute{
				Required:    true,
				Description: "The field to extract from the secret.",
			},
			"value": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "The value of the requested field from the secret.",
			},
		},
	}
}

// Configure initializes the data source with the provider configuration
func (d *TSSSecretDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

// Read retrieves the data for the data source
func (d *TSSSecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Define the state structure
	var state struct {
		SecretID    types.String `tfsdk:"id"`
		Field       types.String `tfsdk:"field"`
		SecretValue types.String `tfsdk:"value"`
	}

	// Read the configuration from the request
	diags := req.Config.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !requireClient(d.client, &resp.Diagnostics) {
		return
	}

	client := d.client

	// Convert SecretID to int
	secretID, err := parsePositiveServerInt(state.SecretID.ValueString(), "Secret ID")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", err.Error())
		return
	}

	fieldValue, ok := fetchSecretField(ctx, client, secretID, state.Field.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}

	// Set the secret value in the state
	state.SecretValue = types.StringValue(fieldValue)

	// Set the state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}
