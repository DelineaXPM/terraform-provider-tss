package delinea

import (
	"context"
	"fmt"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretResource defines the resource implementation
type TSSSecretResource struct {
	clientConfig *server.Configuration // Store the provider configuration
}

// Metadata provides the resource type name
func (r *TSSSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "tss_secret"
}

// Schema defines the schema for the resource
func (r *TSSSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:    true,
				Description: "The ID of the secret.",
			},
			"field": schema.StringAttribute{
				Required:    true,
				Description: "The field to manage in the secret.",
			},
			"value": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The value of the field to manage.",
			},
			"ephemeral": schema.BoolAttribute{
				Optional:    true,
				Description: "If true, the secret value will not be saved in the Terraform state file.",
			},
		},
	}
}

// Configure initializes the resource with the provider configuration
func (r *TSSSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Retrieve the provider configuration
	config, ok := req.ProviderData.(*server.Configuration)
	if !ok {
		resp.Diagnostics.AddError("Configuration Error", "Failed to retrieve provider configuration")
		return
	}

	// Store the provider configuration in the resource
	r.clientConfig = config
}

// Create creates the resource
func (r *TSSSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state struct {
		ID        types.Int64  `tfsdk:"id"`
		Name      types.String `tfsdk:"name"`
		Field     types.String `tfsdk:"field"`
		Value     types.String `tfsdk:"value"`
		Ephemeral types.Bool   `tfsdk:"ephemeral"`
	}

	// Read the configuration
	diags := req.Plan.Get(ctx, &state)
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
		resp.Diagnostics.AddError("Configuration Error", fmt.Sprintf("Failed to create server client: %s", err))
		return
	}

	// Construct the secret object
	newSecret := server.Secret{
		Name: state.Name.ValueString(), // Use the "name" field for the secret's name
		Fields: []server.SecretField{
			{
				FieldName: state.Field.ValueString(), // Use the "field" for the field name
				ItemValue: state.Value.ValueString(), // Use the "value" for the field value
			},
		},
	}

	// Use the client to create the secret
	createdSecret, err := client.CreateSecret(newSecret)
	if err != nil {
		resp.Diagnostics.AddError("Secret Creation Error", fmt.Sprintf("Failed to create secret: %s", err))
		return
	}

	// Set the secret ID in the state
	state.ID = types.Int64Value(int64(createdSecret.ID))

	// Check if the value should be ephemeral
	if state.Ephemeral.ValueBool() {
		// Do not save the secret value in the state
		resp.Diagnostics.AddWarning("Ephemeral Value", "The secret value is marked as ephemeral and will not be saved in the Terraform state.")
		state.Value = types.StringNull() // Mark the value as null
	} else {
		// Save the secret value in the state
		state.Value = types.StringValue(state.Value.ValueString())
	}

	// Set the state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Read reads the resource
func (r *TSSSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Implement similar logic for reading the resource
}

// Update updates the resource
func (r *TSSSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Implement similar logic for updating the resource
}

// Delete deletes the resource
func (r *TSSSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Implement logic for deleting the resource
}
