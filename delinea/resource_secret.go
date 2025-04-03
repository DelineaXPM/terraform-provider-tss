package delinea

import (
	"context"
	"fmt"
	"strconv"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretResource defines the resource implementation
type TSSSecretResource struct {
	clientConfig *server.Configuration // Store the server configuration
}

// Metadata provides the resource type name
func (r *TSSSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "tss_secret"
}

// Schema defines the schema for the resource
func (r *TSSSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"secret_id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the secret",
			},
			"password": schema.StringAttribute{
				Computed:    true,
				Sensitive:   true,
				Description: "The ephemeral password of the secret",
			},
		},
	}
}

// Configure initializes the resource with the provider configuration
func (r *TSSSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	providerConfig, ok := req.ProviderData.(server.Configuration)
	if !ok {
		resp.Diagnostics.AddError("Configuration Error", "Failed to retrieve provider configuration")
		return
	}

	// Store the server configuration in the resource
	r.clientConfig = &providerConfig
}

// Create handles the creation of the resource
func (r *TSSSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan struct {
		SecretID  types.String `tfsdk:"secret_id"`
		Ephemeral types.Bool   `tfsdk:"ephemeral"`
	}

	// Read the plan
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Ensure the client configuration is set
	if r.clientConfig == nil {
		resp.Diagnostics.AddError("Client Error", "The server client is not configured")
		return
	}

	// Create the server client
	client, err := server.New(*r.clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Client Creation Error", fmt.Sprintf("Failed to create server client: %s", err))
		return
	}

	// Convert SecretID to int
	secretID, err := strconv.Atoi(plan.SecretID.ValueString())
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

	// Convert the field key to an integer if necessary
	passwordKey := "password" // Define the key for the password field
	fieldKey, err := strconv.Atoi(passwordKey)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Field Key", fmt.Sprintf("The field key '%s' is not a valid integer: %s", passwordKey, err))
		return
	}

	// Extract the password
	// Extract the password
	password, ok := secret.Field("value")
	if !ok {
		resp.Diagnostics.AddError("Field Not Found", fmt.Sprintf("The secret does not contain the field '%d'", fieldKey))
		return
	}

	// Handle ephemeral behavior
	if plan.Ephemeral.ValueBool() {
		// Dynamically set the ephemeral password without persisting it
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("password"), types.StringValue(password))...)
	} else {
		// Persist the password in the state
		resp.State.Set(ctx, &struct {
			SecretID types.String `tfsdk:"secret_id"`
			Password types.String `tfsdk:"password"`
		}{
			SecretID: plan.SecretID,
			Password: types.StringValue(password),
		})
	}
}

// Read handles reading the resource state
func (r *TSSSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state struct {
		SecretID  types.String `tfsdk:"secret_id"`
		Field     types.String `tfsdk:"field"`
		Ephemeral types.Bool   `tfsdk:"ephemeral"`
	}

	// Read the state
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Ensure the client configuration is set
	if r.clientConfig == nil {
		resp.Diagnostics.AddError("Client Error", "The server client is not configured")
		return
	}

	// Create the server client
	client, err := server.New(*r.clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Client Creation Error", fmt.Sprintf("Failed to create server client: %s", err))
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

	// Extract the password
	// Ensure the field name exists before accessing it
	if state.Field.IsNull() || state.Field.IsUnknown() {
		resp.Diagnostics.AddError("Missing Field Name", "The 'field' attribute is required but was not found in the state.")
		return
	}

	// Get the field name dynamically (same as old implementation)
	fieldName := state.Field.ValueString()

	// Retrieve the secret field dynamically
	value, ok := secret.Field(fieldName)
	if !ok {
		resp.Diagnostics.AddError("Field Not Found", fmt.Sprintf("The secret does not contain the field '%s'", fieldName))
		return
	}

	// Handle ephemeral behavior
	if state.Ephemeral.ValueBool() {
		// Dynamically set the ephemeral field without persisting it
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("value"), types.StringValue(value))...)
	} else {
		// Persist the value in the state
		resp.State.Set(ctx, &struct {
			SecretID types.String `tfsdk:"secret_id"`
			Field    types.String `tfsdk:"field"`
			Value    types.String `tfsdk:"value"`
		}{
			SecretID: state.SecretID,
			Field:    state.Field,
			Value:    types.StringValue(value),
		})
	}
}

// Update handles updating the resource
func (r *TSSSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// No update logic needed for this resource
}

// Delete handles deleting the resource
func (r *TSSSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No action needed for ephemeral values
}
