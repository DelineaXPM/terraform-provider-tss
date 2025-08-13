package delinea

import (
	"context"
	"fmt"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretDeletionResource defines the resource implementation
type TSSSecretDeletionResource struct {
	clientConfig *server.Configuration // Store the provider configuration
}

// SecretDeletionResourceState defines the state structure for the deletion resource
type SecretDeletionResourceState struct {
	SecretID types.Int64  `tfsdk:"secret_id"`
	ID       types.String `tfsdk:"id"`
}

// Metadata provides the resource type name
func (r *TSSSecretDeletionResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "tss_secret_deletion"
}

// Configure initializes the resource with the provider configuration
func (r *TSSSecretDeletionResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	config, ok := req.ProviderData.(*server.Configuration)
	if !ok {
		resp.Diagnostics.AddError("Configuration Error", "Failed to retrieve provider configuration")
		return
	}

	// Store the provider configuration in the resource
	r.clientConfig = config
}

// Schema defines the schema for the resource
func (r *TSSSecretDeletionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A resource to delete secrets by ID without requiring them to be in the Terraform state.",
		Attributes: map[string]schema.Attribute{
			"secret_id": schema.Int64Attribute{
				Required:    true,
				Description: "The ID of the secret to delete.",
			},
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "The ID of the resource. This is set to 'secret_<secret_id>' after deletion.",
			},
		},
	}
}

// Create performs the secret deletion operation
func (r *TSSSecretDeletionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecretDeletionResourceState

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
		resp.Diagnostics.AddError("Configuration Error", fmt.Sprintf("Failed to create server client: %s", err))
		return
	}

	secretID := int(plan.SecretID.ValueInt64())

	// Check if the secret exists
	_, err = client.Secret(secretID)
	if err != nil {
		resp.Diagnostics.AddError("Secret Not Found", fmt.Sprintf("The secret with ID %d does not exist: %s", secretID, err))
		return
	}

	// Delete the secret
	err = client.DeleteSecret(secretID)
	if err != nil {
		resp.Diagnostics.AddError("Secret Deletion Error", fmt.Sprintf("Failed to delete secret with ID %d: %s", secretID, err))
		return
	}

	// Set the ID to a unique value based on the secret ID for Terraform state
	plan.ID = types.StringValue(fmt.Sprintf("secret_%d", secretID))

	// Set the state with the deleted secret information
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read checks if the secret still exists
func (r *TSSSecretDeletionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecretDeletionResourceState

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
		resp.Diagnostics.AddError("Configuration Error", fmt.Sprintf("Failed to create server client: %s", err))
		return
	}

	secretID := int(state.SecretID.ValueInt64())

	// Check if the secret still exists
	_, err = client.Secret(secretID)
	if err != nil {
		// Secret doesn't exist, which is what we want
		diags = resp.State.Set(ctx, state)
		resp.Diagnostics.Append(diags...)
		return
	}

	// Secret still exists, report as removed from state
	resp.Diagnostics.AddWarning(
		"Secret Still Exists",
		fmt.Sprintf("Secret with ID %d still exists even though it was marked for deletion. This might indicate that the deletion failed or was reverted.", secretID),
	)

	// Still keep the state
	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
}

// Update is a no-op since we can't update a deleted resource
func (r *TSSSecretDeletionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SecretDeletionResourceState

	// Read the plan
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set the state with the plan
	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Delete is a no-op because the secret was already deleted during Create
func (r *TSSSecretDeletionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// The actual deletion was done in Create, so this is a no-op
}
