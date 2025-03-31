package delinea

import (
	"context"
	"fmt"
	"strconv"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretsDataSource defines the data source implementation
type TSSSecretsDataSource struct {
	clientConfig *server.Configuration // Store the provider configuration
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
							Description: "The ephemeral value of the field of the secret",
						},
					},
				},
			},
		},
	}
}

// Configure initializes the data source with the provider configuration
func (d *TSSSecretsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	// Retrieve the provider configuration
	config, ok := req.ProviderData.(server.Configuration)
	if !ok {
		resp.Diagnostics.AddError("Configuration Error", "Failed to retrieve provider configuration")
		return
	}

	// Store the provider configuration in the data source
	d.clientConfig = &config
}

// Read handles reading the data source state
func (d *TSSSecretsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state struct {
		IDs     []types.Int64 `tfsdk:"ids"`
		Field   types.String  `tfsdk:"field"`
		Secrets []struct {
			ID    types.Int64  `tfsdk:"id"`
			Value types.String `tfsdk:"value"`
		} `tfsdk:"secrets"`
	}

	// Read the configuration
	diags := req.Config.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Ensure the client configuration is set
	if d.clientConfig == nil {
		resp.Diagnostics.AddError("Client Error", "The server client is not configured")
		return
	}

	// Create the server client
	secretsClient, err := server.New(*d.clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Configuration Error", fmt.Sprintf("Failed to create server client: %s", err))
		return
	}

	// Fetch secrets
	var results []struct {
		ID    types.Int64  `tfsdk:"id"`
		Value types.String `tfsdk:"value"`
	}

	for _, id := range state.IDs {
		secretID := int(id.ValueInt64())

		// Fetch the secret
		secret, err := secretsClient.Secret(secretID)
		if err != nil {
			resp.Diagnostics.AddWarning("Secret Fetch Warning", fmt.Sprintf("Failed to fetch secret with ID %d: %s", secretID, err))
			continue // Skip this ID and continue with the rest
		}

		// Convert field key to int if necessary
		fieldKey, err := strconv.Atoi(state.Field.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid Field Key", fmt.Sprintf("The field key '%s' is not a valid integer: %s", state.Field.ValueString(), err))
			continue
		}

		// Extract the field value
		fieldValue, ok := secret.Field[fieldKey]
		if !ok {
			resp.Diagnostics.AddWarning("Field Not Found", fmt.Sprintf("The secret with ID %d does not contain the field '%d'", secretID, fieldKey))
			continue // Skip this secret if the field is not found
		}

		// Add the secret to the results
		results = append(results, struct {
			ID    types.Int64  `tfsdk:"id"`
			Value types.String `tfsdk:"value"`
		}{
			ID:    types.Int64Value(int64(secretID)),
			Value: types.StringValue(fieldValue),
		})
	}

	// Set the state
	state.Secrets = results
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}
