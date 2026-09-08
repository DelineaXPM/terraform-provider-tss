package delinea

import (
	"context"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretResource defines the resource implementation
type TSSSecretsEphemeralResource struct {
	client *server.Server // Shared SDK client built once in Provider.Configure
}

func (r *TSSSecretsEphemeralResource) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = "tss_secrets"
}

// Define the model for your resource state
type TSSSecretsEphemeralResourceModel struct {
	IDs     []types.Int64 `tfsdk:"ids"`
	Field   types.String  `tfsdk:"field"`
	Secrets []SecretModel `tfsdk:"secrets"`
}

func (r *TSSSecretsEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"ids": schema.ListAttribute{
				ElementType: types.Int64Type,
				Required:    true,
				Description: "A list of IDs of the secrets",
			},
			"field": schema.StringAttribute{
				Required:    true,
				Description: "The field to extract from the secrets",
			},
			"secrets": schema.ListNestedAttribute{
				Computed:    true,
				Description: "A list of secrets with their field values",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Computed:    true,
							Description: "The ID of the secret",
						},
						"value": schema.StringAttribute{
							Computed:    true,
							Sensitive:   true,
							Description: "The ephemeral value of the field of the secret",
						},
					},
				},
			},
		},
	}
}

func (r *TSSSecretsEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	// Create a model to hold the input configuration
	var data TSSSecretsEphemeralResourceModel

	// Read the Terraform config data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	// Check for required fields in the model (secret_ids and field)
	if len(data.IDs) == 0 || data.Field.IsNull() {
		resp.Diagnostics.AddError("Missing Required Field", "Both secret_ids and field are required")
		return
	}

	client := r.client

	results, ok := fetchSecretModels(ctx, client, data.IDs, data.Field.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}

	// Set the secret value in the result
	data.Secrets = results

	// Save the data into the ephemeral result state. No RenewAt is set:
	// ephemeral renewal cannot update the result, so a renewal cycle would
	// only re-fetch every secret (attachment downloads included) to discard
	// it.
	resp.Diagnostics.Append(resp.Result.Set(ctx, &data)...)
}

func (r *TSSSecretsEphemeralResource) Close(ctx context.Context, req ephemeral.CloseRequest, resp *ephemeral.CloseResponse) {

}

func (r *TSSSecretsEphemeralResource) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}
