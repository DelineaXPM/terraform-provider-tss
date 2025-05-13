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

// TSSSecretDataSource defines the data source implementation
type TSSSecretDataSource struct {
	clientConfig *server.Configuration // Store the provider configuration
}

// Metadata provides the data source type name
func (d *TSSSecretDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "tss_secret"
}

// Schema defines the schema for the data source
func (d *TSSSecretDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"secret_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the secret to retrieve.",
			},
			"field": schema.StringAttribute{
				Required:    true,
				Description: "The field to extract from the secret.",
			},
			"secret_value": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "The value of the requested field from the secret.",
			},
		},
	}
}

// Configure initializes the data source with the provider configuration
func (d *TSSSecretDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		// IMPORTANT: This method is called MULTIPLE times. An initial call might not have configured the Provider yet, so we need
		// to handle this gracefully. It will eventually be called with a configured provider.
		return
	}

	// Log the received ProviderData
	fmt.Printf("DEBUG: ProviderData received in Configure: %+v\n", req.ProviderData)

	config, ok := req.ProviderData.(*server.Configuration)
	if !ok || config == nil {
		resp.Diagnostics.AddError("Configuration Error", "Failed to retrieve provider configuration")
		return
	}

	// Log the successfully retrieved configuration
	fmt.Printf("DEBUG: Successfully retrieved provider configuration: %+v\n", config)

	d.clientConfig = config
}

// Read retrieves the data for the data source
func (d *TSSSecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Define the state structure
	var state struct {
		SecretID    types.String `tfsdk:"secret_id"`
		Field       types.String `tfsdk:"field"`
		SecretValue types.String `tfsdk:"secret_value"`
	}

	// Read the configuration from the request
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
	client, err := server.New(*d.clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Configuration Error", fmt.Sprintf("Failed to create server client: %s", err))
		return
	}

	// Convert SecretID to int
	secretID, err := strconv.Atoi(state.SecretID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", "Secret ID must be an integer")
		return
	}

	fmt.Printf("[DEBUG] getting secret with id %d", secretID)

	// Fetch the secret
	secret, err := client.Secret(secretID)
	if err != nil {
		resp.Diagnostics.AddError("Secret Fetch Error", fmt.Sprintf("Failed to fetch secret: %s", err))
		return
	}

	// Get the field name dynamically
	fieldName := state.Field.ValueString()

	fmt.Printf("[DEBUG] using '%s' field of secret with id %d", fieldName, secretID)

	// Extract the secret value
	fieldValue, ok := secret.Field(fieldName)
	if !ok {
		resp.Diagnostics.AddError("Field Not Found", fmt.Sprintf("The secret does not contain the field '%s'", fieldName))
		return
	}

	// Set the secret value in the state
	state.SecretValue = types.StringValue(fieldValue)

	// Set the state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}
