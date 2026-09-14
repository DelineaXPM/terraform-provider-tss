package delinea

import (
	"context"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretsDataSource defines the data source implementation
type TSSSecretsDataSource struct {
	client *server.Server // Shared SDK client built once in Provider.Configure
}

// Metadata provides the data source type name
func (d *TSSSecretsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "tss_secrets"
}

// Schema defines the schema for the data source
func (d *TSSSecretsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
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
							Description: "The value of the field of the secret",
						},
					},
				},
			},
		},
	}
}

// Configure initializes the data source with the provider configuration
func (d *TSSSecretsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	d.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

func (d *TSSSecretsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state struct {
		IDs     []types.Int64 `tfsdk:"ids"`
		Field   types.String  `tfsdk:"field"`
		Secrets []SecretModel `tfsdk:"secrets"`
	}

	// Read the configuration
	diags := req.Config.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !requireClient(d.client, &resp.Diagnostics) {
		return
	}

	secretsClient := d.client

	results, ok := fetchSecretModels(ctx, secretsClient, state.IDs, state.Field.ValueString(), &resp.Diagnostics)
	if !ok {
		return
	}

	// Set the state
	state.Secrets = results
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}
