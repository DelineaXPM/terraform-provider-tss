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
			"ephemeral": schema.BoolAttribute{
				Optional:    true,
				Description: "If true, the secret value will not be saved in the Terraform state file.",
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
	// Retrieve the provider configuration
	config, ok := req.ProviderData.(*server.Configuration)
	if !ok {
		resp.Diagnostics.AddError("Configuration Error", "Failed to retrieve provider configuration")
		return
	}

	// Store the provider configuration in the data source
	d.clientConfig = config
}

// Read retrieves the data for the data source
func (d *TSSSecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	// Define the state structure
	var state struct {
		SecretID    types.String `tfsdk:"secret_id"`
		Field       types.String `tfsdk:"field"`
		Ephemeral   types.Bool   `tfsdk:"ephemeral"`
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

	// Fetch the secret
	secret, err := client.Secret(secretID)
	if err != nil {
		resp.Diagnostics.AddError("Secret Fetch Error", fmt.Sprintf("Failed to fetch secret: %s", err))
		return
	}

	// Get the field name dynamically
	fieldName := state.Field.ValueString()

	// Extract the secret value
	fieldValue, ok := secret.Field(fieldName)
	if !ok {
		resp.Diagnostics.AddError("Field Not Found", fmt.Sprintf("The secret does not contain the field '%s'", fieldName))
		return
	}

	// Check if the value should be ephemeral
	if state.Ephemeral.ValueBool() {
		// Do not set the secret value in the state
		resp.Diagnostics.AddWarning("Ephemeral Value", "The secret value is marked as ephemeral and will not be saved in the Terraform state.")
		state.SecretValue = types.StringNull() // Mark the value as null
	} else {
		// Set the secret value in the state
		state.SecretValue = types.StringValue(fieldValue)
	}

	// Set the state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}
