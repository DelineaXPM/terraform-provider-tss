package delinea

import (
	"context"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretResource defines the resource implementation
type TSSSecretEphemeralResource struct {
	client *server.Server // Shared SDK client built once in Provider.Configure
}

func (r *TSSSecretEphemeralResource) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = "tss_secret"
}

// Define the model for your resource state
type TSSSecretEphemeralResourceModel struct {
	SecretID    types.String `tfsdk:"id"`
	Field       types.String `tfsdk:"field"`
	SecretValue types.String `tfsdk:"value"`
}

func (r *TSSSecretEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
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

func (r *TSSSecretEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	// Create a model to hold the input configuration
	var data TSSSecretEphemeralResourceModel

	// Read the Terraform config data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	// Check for required fields in the model (secret_id and field)
	if data.SecretID.IsNull() || data.Field.IsNull() {
		resp.Diagnostics.AddError("Missing Required Field", "Both secret_id and field are required")
		return
	}

	client := r.client

	// Convert SecretID to integer
	secretID, err := parsePositiveServerInt(data.SecretID.ValueString(), "Secret ID")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", err.Error())
		return
	}

	fieldValue, ok := fetchSecretField(ctx, client, secretID, data.Field.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}

	// Set the secret value in the result
	data.SecretValue = types.StringValue(fieldValue)

	// Save the data into the ephemeral result state. No RenewAt is set:
	// ephemeral renewal cannot update the result, so a renewal cycle would
	// only re-fetch the secret to discard it.
	resp.Diagnostics.Append(resp.Result.Set(ctx, &data)...)
}

func (r *TSSSecretEphemeralResource) Close(ctx context.Context, req ephemeral.CloseRequest, resp *ephemeral.CloseResponse) {

}

func (r *TSSSecretEphemeralResource) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}
