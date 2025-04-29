package delinea

import (
	"context"
	"fmt"
	"strconv"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// TSSSecretDataSource defines the data source implementation
type TSSSecretDataSource struct {
	clientConfig *server.Configuration // Store the provider configuration
}

// Metadata provides the data source type name
func (d *TSSSecretDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "tss_secret"
	tflog.Debug(ctx, "Metadata function called, TypeName set to 'tss_secret'")
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
	tflog.Debug(ctx, "Schema function called, data source schema defined")
}

// Configure initializes the data source with the provider configuration
func (d *TSSSecretDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		// IMPORTANT: This method is called MULTIPLE times. An initial call might not have configured the Provider yet, so we need
		// to handle this gracefully. It will eventually be called with a configured provider.
		return
	}

	config, ok := req.ProviderData.(*server.Configuration)
	if !ok || config == nil {
		resp.Diagnostics.AddError("Configuration Error", "Failed to retrieve provider configuration")
		tflog.Error(ctx, "ProviderData is nil or not of type *server.Configuration")
		return
	}

	tflog.Debug(ctx, "Successfully retrieved provider configuration", map[string]interface{}{
		"server_url": config.ServerURL,
	})
	d.clientConfig = config
	tflog.Debug(ctx, "Provider configuration stored in clientConfig")
}

// Read retrieves the data for the data source
func (d *TSSSecretDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Debug(ctx, "Read function called")

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
		tflog.Error(ctx, "Failed to read configuration from request", map[string]interface{}{
			"diagnostics": resp.Diagnostics,
		})
		return
	}

	// Ensure the client configuration is set
	if d.clientConfig == nil {
		resp.Diagnostics.AddError("Client Error", "The server client is not configured")
		tflog.Error(ctx, "Client configuration is nil")
		return
	}

	// Create the server client
	client, err := server.New(*d.clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Configuration Error", fmt.Sprintf("Failed to create server client: %s", err))
		tflog.Error(ctx, "Failed to create server client", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// Convert SecretID to int
	secretID, err := strconv.Atoi(state.SecretID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", "Secret ID must be an integer")
		tflog.Error(ctx, "Invalid Secret ID", map[string]interface{}{
			"secret_id": state.SecretID.ValueString(),
		})
		return
	}

	// Fetch the secret
	secret, err := client.Secret(secretID)
	if err != nil {
		resp.Diagnostics.AddError("Secret Fetch Error", fmt.Sprintf("Failed to fetch secret: %s", err))
		tflog.Error(ctx, "Failed to fetch secret", map[string]interface{}{
			"secret_id": secretID,
			"error":     err.Error(),
		})
		return
	}

	// Get the field name dynamically
	fieldName := state.Field.ValueString()

	// Extract the secret value
	fieldValue, ok := secret.Field(fieldName)
	if !ok {
		resp.Diagnostics.AddError("Field Not Found", fmt.Sprintf("The secret does not contain the field '%s'", fieldName))
		tflog.Error(ctx, "Field not found in secret", map[string]interface{}{
			"field_name": fieldName,
		})
		return
	}

	// Check if the value should be ephemeral
	if state.Ephemeral.ValueBool() {
		// Do not set the secret value in the state
		resp.Diagnostics.AddWarning("Ephemeral Value", "The secret value is marked as ephemeral and will not be saved in the Terraform state.")
		state.SecretValue = types.StringNull() // Mark the value as null
		tflog.Debug(ctx, "Ephemeral value detected, secret value not saved in state")
	} else {
		// Set the secret value in the state
		state.SecretValue = types.StringValue(fieldValue)
		tflog.Debug(ctx, "Secret value retrieved and saved in state", map[string]interface{}{
			"field_name":  fieldName,
			"field_value": fieldValue,
		})
	}

	// Set the state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		tflog.Error(ctx, "Failed to set state", map[string]interface{}{
			"diagnostics": resp.Diagnostics,
		})
	}
}
