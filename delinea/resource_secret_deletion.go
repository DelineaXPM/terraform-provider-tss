package delinea

import (
	"context"
	"fmt"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretDeletionResource defines the resource implementation
type TSSSecretDeletionResource struct {
	client *server.Server // Shared SDK client built once in Provider.Configure
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
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

// Schema defines the schema for the resource
func (r *TSSSecretDeletionResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A one-shot resource that deletes a secret by ID without requiring it to be in Terraform state. A completed operation remains in state and does not delete a later-restored secret again.",
		Attributes: map[string]schema.Attribute{
			"secret_id": schema.Int64Attribute{
				Required:    true,
				Description: "The ID of the secret to delete. Changing it replaces the operation and deletes the newly selected secret.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
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

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	client := r.client

	secretID, err := toPositiveServerInt(plan.SecretID.ValueInt64(), "secret_id")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", err.Error())
		return
	}

	// Delete without a pre-check GET: DeleteSecretContext reports its own
	// failure, and an identity with delete rights but a view restriction
	// would fail a read-based existence check on a secret it can delete.
	err = client.DeleteSecretContext(ctx, secretID)
	if err != nil {
		if isAuthenticationFailure(err) {
			resp.Diagnostics.AddError("Secret Authentication or Authorization Failed", authenticationFailureDetail("delete", secretID, err))
			return
		}
		switch classifyDeleteFailure(ctx, client, secretID, err) {
		case deleteRefused:
			resp.Diagnostics.AddError("Secret Deletion Refused", deleteRefusedDetail(secretID, err))
		case deleteAmbiguous:
			resp.Diagnostics.AddError("Secret Deletion Unverified", deleteAmbiguousDetail(secretID, err, false))
		default:
			resp.Diagnostics.AddError("Secret Deletion Error", fmt.Sprintf("Failed to delete secret with ID %d: %s", secretID, err))
		}
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

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	client := r.client

	secretID, err := toPositiveServerInt(state.SecretID.ValueInt64(), "secret_id")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", err.Error())
		return
	}

	// The resource records a completed one-shot operation. Expected absent and
	// recycled responses retain that record; a visibly active secret is warned
	// about without scheduling another destructive action.
	secret, err := client.SecretContext(ctx, secretID)
	if err != nil {
		if isSecretGone(err) {
			diags = resp.State.Set(ctx, state)
			resp.Diagnostics.Append(diags...)
			return
		}
		if isAuthenticationFailure(err) {
			resp.Diagnostics.AddError("Secret Authentication or Authorization Failed", authenticationFailureDetail("verify deletion of", secretID, err))
			return
		}
		resp.Diagnostics.AddError("Secret Lookup Error",
			fmt.Sprintf("Failed to verify deletion of secret %d: %s", secretID, err))
		return
	}
	if !secret.Active {
		diags = resp.State.Set(ctx, state)
		resp.Diagnostics.Append(diags...)
		return
	}

	resp.Diagnostics.AddWarning(
		"Secret Still Exists",
		fmt.Sprintf("Secret with ID %d is active even though its one-shot deletion operation completed. Terraform will retain the operation record and will not delete the secret again automatically; remove the resource from state and re-apply only if another deletion is intended.", secretID),
	)
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
