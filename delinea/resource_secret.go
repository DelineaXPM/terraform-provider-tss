package delinea

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/DelineaXPM/delinea-common/api"
	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretResource defines the resource implementation
type TSSSecretResource struct {
	client *server.Server // Shared SDK client built once in Provider.Configure
}

// SecretResourceState defines the state structure for the secret resource
type SecretResourceState struct {
	ID                               types.Int64   `tfsdk:"id"`
	Name                             types.String  `tfsdk:"name"`
	FolderID                         types.String  `tfsdk:"folderid"`
	SiteID                           types.String  `tfsdk:"siteid"`
	SecretTemplateID                 types.String  `tfsdk:"secrettemplateid"`
	Fields                           []SecretField `tfsdk:"fields"`
	SshKeyArgs                       *SshKeyArgs   `tfsdk:"sshkeyargs"`
	Active                           types.Bool    `tfsdk:"active"`
	SecretPolicyID                   types.Int64   `tfsdk:"secretpolicyid"`
	PasswordTypeWebScriptID          types.Int64   `tfsdk:"passwordtypewebscriptid"`
	LauncherConnectAsSecretID        types.Int64   `tfsdk:"launcherconnectassecretid"`
	CheckOutIntervalMinutes          types.Int64   `tfsdk:"checkoutintervalminutes"`
	CheckedOut                       types.Bool    `tfsdk:"checkedout"`
	CheckOutEnabled                  types.Bool    `tfsdk:"checkoutenabled"`
	AutoChangeEnabled                types.Bool    `tfsdk:"autochangenabled"`
	CheckOutChangePasswordEnabled    types.Bool    `tfsdk:"checkoutchangepasswordenabled"`
	DelayIndexing                    types.Bool    `tfsdk:"delayindexing"`
	EnableInheritPermissions         types.Bool    `tfsdk:"enableinheritpermissions"`
	EnableInheritSecretPolicy        types.Bool    `tfsdk:"enableinheritsecretpolicy"`
	ProxyEnabled                     types.Bool    `tfsdk:"proxyenabled"`
	RequiresComment                  types.Bool    `tfsdk:"requirescomment"`
	SessionRecordingEnabled          types.Bool    `tfsdk:"sessionrecordingenabled"`
	WebLauncherRequiresIncognitoMode types.Bool    `tfsdk:"weblauncherrequiresincognitomode"`
}

type SecretField struct {
	FieldName         types.String `tfsdk:"fieldname"`
	ItemValue         types.String `tfsdk:"itemvalue"`
	PasswordValue     types.String `tfsdk:"password_value"`
	PasswordWoVersion types.Int64  `tfsdk:"password_wo_version"`
	Generate          types.Bool   `tfsdk:"generate"`
	ItemID            types.Int64  `tfsdk:"itemid"`
	FieldID           types.Int64  `tfsdk:"fieldid"`
	FileAttachmentID  types.Int64  `tfsdk:"fileattachmentid"`
	Slug              types.String `tfsdk:"slug"`
	FieldDescription  types.String `tfsdk:"fielddescription"`
	Filename          types.String `tfsdk:"filename"`
	IsFile            types.Bool   `tfsdk:"isfile"`
	IsNotes           types.Bool   `tfsdk:"isnotes"`
	IsPassword        types.Bool   `tfsdk:"ispassword"`
	IsList            types.Bool   `tfsdk:"islist"`
	ListType          types.String `tfsdk:"listtype"`
}

type SshKeyArgs struct {
	GeneratePassphrase types.Bool `tfsdk:"generatepassphrase"`
	GenerateSshKeys    types.Bool `tfsdk:"generatesshkeys"`
}

// Metadata provides the resource type name
func (r *TSSSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "tss_resource_secret"
}

// Configure initializes the resource with the provider configuration
func (r *TSSSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.client = clientFromProviderData(req.ProviderData, &resp.Diagnostics)
}

// Create creates the resource
func (r *TSSSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config SecretResourceState

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	mergeWriteOnlyFieldValues(plan.Fields, config.Fields)

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	client := r.client

	// Get the secret data
	newSecret, err := r.getSecretData(ctx, &plan, client)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", fmt.Sprintf("Failed to prepare secret data: %s", err))
		return
	}

	fmt.Printf("[DEBUG] creating secret with name %s", newSecret.Name)

	// Use the client to create the secret
	createdSecret, err := client.CreateSecretContext(ctx, *newSecret)
	if err != nil {
		// A PartialWriteError means Secret Server accepted the create but a
		// later SDK step (refresh, file upload) failed or was cancelled. The
		// secret exists server-side; record its ID so Terraform tracks it as
		// tainted instead of orphaning it and creating a duplicate next apply.
		if pwe, ok := errors.AsType[*server.PartialWriteError](err); ok {
			if pwe.SecretID != 0 {
				recordTaintedSecret(ctx, resp, pwe.SecretID, "Secret Partially Created",
					fmt.Sprintf("Secret %d was created on Secret Server but a later step failed: %s.", pwe.SecretID, err))
				return
			}
			resp.Diagnostics.AddError("Secret Possibly Created",
				fmt.Sprintf("Secret Server may have accepted the create, but the response did not yield a usable secret ID: %s. "+
					"Before re-applying, check Secret Server for a secret named %q and delete it, or the next apply may create a duplicate.", err, newSecret.Name))
			return
		}
		// A rejection the server actually sent back (4xx), or a failure the
		// SDK classifies as configuration/auth/authorization — all of which
		// happen before or instead of a committed write — means the secret
		// was not created. Anything else — timeout, cancellation, connection
		// loss — leaves the outcome unknown: the POST may have committed
		// after the client gave up.
		he, isHTTP := errors.AsType[*server.HTTPError](err)
		if (isHTTP && he.StatusCode >= 400 && he.StatusCode < 500) ||
			errors.Is(err, api.ErrConfig) || errors.Is(err, api.ErrAuth) || errors.Is(err, api.ErrAccessDenied) {
			resp.Diagnostics.AddError("Secret Creation Error", fmt.Sprintf("Failed to create secret: %s", err))
			return
		}
		resp.Diagnostics.AddError("Secret Creation Error",
			fmt.Sprintf("Failed to create secret: %s. The request may still have reached Secret Server; "+
				"before re-applying, check for a secret named %q and delete it, or the next apply may create a duplicate.", err, newSecret.Name))
		return
	}

	fmt.Printf("Secret is Created successfully...!")

	// Refresh state - let Terraform accept the computed values from the server
	newState, notFound, readDiags := r.readSecretByID(ctx, createdSecret.ID, client, plan.SecretTemplateID, plan.Fields)
	resp.Diagnostics.Append(readDiags...)
	if notFound {
		// The create succeeded but the server now reports the secret gone.
		recordTaintedSecret(ctx, resp, createdSecret.ID, "Secret Created, Then Reported Gone",
			fmt.Sprintf("Secret %d was created but Secret Server reports it no longer exists.", createdSecret.ID))
		return
	}
	if resp.Diagnostics.HasError() {
		// The create succeeded; only the refresh failed. Record the ID so
		// Terraform tracks the secret as tainted instead of orphaning it.
		recordTaintedSecret(ctx, resp, createdSecret.ID, "Secret Created, Refresh Failed",
			fmt.Sprintf("Secret %d was created but reading it back failed.", createdSecret.ID))
		return
	}

	// Preserve the SSH key args from the plan since the server doesn't return them
	if plan.SshKeyArgs != nil {
		newState.SshKeyArgs = plan.SshKeyArgs
	}

	// Preserve file attachment information for file fields
	for i, field := range newState.Fields {
		if field.IsFile.ValueBool() {
			// Find the matching field in the plan
			for _, planField := range plan.Fields {
				if planField.FieldName.ValueString() == field.FieldName.ValueString() && planField.IsFile.ValueBool() {
					// Preserve FileAttachmentID and Filename
					newState.Fields[i].FileAttachmentID = planField.FileAttachmentID
					newState.Fields[i].Filename = planField.Filename
					break
				}
			}
		}
	}

	// Set the state
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource
func (r *TSSSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config SecretResourceState

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	diags = req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	mergeWriteOnlyFieldValues(plan.Fields, config.Fields)

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	client := r.client

	// Get the secret data
	// During update, we shouldn't send SSH key generation parameters
	// because the server doesn't support SSH key generation during update
	updatePlan := plan

	// Check if SSH key generation was requested in the original creation
	hasSshKeyArgs := false
	if state.SshKeyArgs != nil &&
		(state.SshKeyArgs.GenerateSshKeys.ValueBool() ||
			state.SshKeyArgs.GeneratePassphrase.ValueBool()) {
		hasSshKeyArgs = true
	}

	// Don't send SSH key args during update - they're only for creation
	updatePlan.SshKeyArgs = nil

	updatedSecret, err := r.getSecretData(ctx, &updatePlan, client)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", fmt.Sprintf("Failed to prepare secret data: %s", err))
		return
	}

	// If we have SSH key fields, preserve the existing values from the current state
	for i, field := range updatedSecret.Fields {
		fieldName := field.FieldName
		if hasSshKeyArgs && (strings.Contains(strings.ToLower(fieldName), "key") ||
			strings.Contains(strings.ToLower(fieldName), "passphrase")) {
			// For secrets with SSH keys, preserve the server-generated values
			for _, stateField := range state.Fields {
				if strings.EqualFold(stateField.FieldName.ValueString(), fieldName) {
					// Check if the plan specifically wants to update this field
					// If not, preserve the existing state value
					fieldFound := false
					for _, planField := range plan.Fields {
						if strings.EqualFold(planField.FieldName.ValueString(), fieldName) {
							fieldFound = true
							if planField.ItemValue.IsNull() || planField.ItemValue.ValueString() == "" {
								// Plan is not updating this field, preserve state
								updatedSecret.Fields[i].ItemValue = stateField.ItemValue.ValueString()
								fmt.Printf("[DEBUG] Preserving SSH field %s value during update\n", fieldName)
							} else {
								// Plan is updating this field, use new value
								fmt.Printf("[DEBUG] Updating SSH field %s with new value\n", fieldName)
							}
							break
						}
					}

					if !fieldFound {
						// Field not found in plan, preserve state value
						updatedSecret.Fields[i].ItemValue = stateField.ItemValue.ValueString()
						fmt.Printf("[DEBUG] Preserving SSH field %s value (not in plan)\n", fieldName)
					}

					// Also preserve the filename for key fields regardless
					if !stateField.Filename.IsNull() && stateField.Filename.ValueString() != "" {
						updatedSecret.Fields[i].Filename = stateField.Filename.ValueString()
						fmt.Printf("[DEBUG] Preserving filename %s for field %s\n",
							stateField.Filename.ValueString(), fieldName)
					}
					break
				}
			}
		}
	}

	// Update the secret
	updatedSecret.ID = int(state.ID.ValueInt64())
	fmt.Printf("[DEBUG] updating secret with id %d", updatedSecret.ID)
	_, err = client.UpdateSecretContext(ctx, *updatedSecret)
	if err != nil {
		if _, ok := errors.AsType[*server.PartialWriteError](err); ok {
			resp.Diagnostics.AddError("Secret Partially Updated",
				fmt.Sprintf("Secret Server accepted the update for secret %d but a later step failed: %s. "+
					"State keeps the pre-update values; the next apply re-runs the update and converges.", updatedSecret.ID, err))
			return
		}
		resp.Diagnostics.AddError("Secret Update Error", fmt.Sprintf("Failed to update secret: %s", err))
		return
	}

	fmt.Printf("Secret is Updated successfully...!")

	//Refresh state
	newState, notFound, readDiags := r.readSecretByID(ctx, updatedSecret.ID, client, plan.SecretTemplateID, plan.Fields)
	if notFound {
		resp.Diagnostics.AddError("Secret Missing After Update",
			fmt.Sprintf("Secret %d was updated but no longer exists on Secret Server. It may have been deleted concurrently; the next plan will show the discrepancy.", updatedSecret.ID))
		return
	}
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve the SSH key args from the plan since the server doesn't return them
	if plan.SshKeyArgs != nil {
		newState.SshKeyArgs = plan.SshKeyArgs
	}

	// Preserve file attachment information for file fields and SSH key fields
	for i, field := range newState.Fields {
		fieldName := field.FieldName.ValueString()
		isSSHKeyField := hasSshKeyArgs && (strings.Contains(strings.ToLower(fieldName), "key") ||
			strings.Contains(strings.ToLower(fieldName), "passphrase"))

		// Handle both regular file fields and SSH key fields
		if field.IsFile.ValueBool() || isSSHKeyField {
			// First check the state (higher priority for existing secrets)
			for _, stateField := range state.Fields {
				if stateField.FieldName.ValueString() == fieldName {
					// Preserve FileAttachmentID and Filename from state
					if !stateField.FileAttachmentID.IsNull() {
						newState.Fields[i].FileAttachmentID = stateField.FileAttachmentID
					}
					if !stateField.Filename.IsNull() && stateField.Filename.ValueString() != "" {
						newState.Fields[i].Filename = stateField.Filename
						fmt.Printf("[DEBUG] Preserved filename %s for field %s from state\n",
							stateField.Filename.ValueString(), fieldName)
					}
					break
				}
			}

			// If filename still empty, check plan
			if newState.Fields[i].Filename.IsNull() || newState.Fields[i].Filename.ValueString() == "" {
				for _, planField := range plan.Fields {
					if planField.FieldName.ValueString() == fieldName {
						if !planField.Filename.IsNull() && planField.Filename.ValueString() != "" {
							newState.Fields[i].Filename = planField.Filename
							fmt.Printf("[DEBUG] Preserved filename %s for field %s from plan\n",
								planField.Filename.ValueString(), fieldName)
						}
						break
					}
				}
			}
		}
	}

	// Set the state
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
}

// Delete deletes the resource
func (r *TSSSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SecretResourceState

	// Read the state
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	fmt.Printf("[DEBUG] deleting secret with id %d", int(state.ID.ValueInt64()))

	client := r.client

	// Delete the secret
	secretID := int(state.ID.ValueInt64())
	err := client.DeleteSecretContext(ctx, secretID)
	if err != nil {
		// Classic Secret Server answers a DELETE of an already-deleted secret
		// with the same ambiguous access-denied status it uses for a
		// policy-refused DELETE of a live secret (checkout, DoubleLock,
		// revoked permission), so disambiguate with a follow-up GET: a
		// readable secret proves the DELETE was refused, while a GET that
		// also reports it gone makes the idempotent-destroy interpretation
		// safe — e.g. a tainted phantom ID no longer wedges every replace.
		if isSecretGone(err) {
			if _, getErr := client.SecretContext(ctx, secretID); getErr == nil {
				resp.Diagnostics.AddError("Secret Deletion Refused",
					fmt.Sprintf("Secret Server refused to delete secret %d (%s) but the secret still exists — it may be checked out, DoubleLocked, or the account may lack delete permission.", secretID, err))
				return
			} else if isSecretGone(getErr) {
				resp.Diagnostics.AddWarning("Secret Already Absent",
					fmt.Sprintf("Deleting secret %d reported it as already deleted or inaccessible (%s), and it is not readable either; it has been removed from state.", secretID, err))
				return
			}
		}
		resp.Diagnostics.AddError("Secret Deletion Error", fmt.Sprintf("Failed to delete secret: %s", err))
		return
	}

	fmt.Printf("Secret is Deleted successfully...!")
}

// Schema defines the schema for the resource
func (r *TSSSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version: 1,
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the secret.",
			},
			"folderid": schema.StringAttribute{ // Changed to string for backward compatibility
				Required:    true,
				Description: "The folder ID of the secret.",
			},
			"siteid": schema.StringAttribute{ // Changed to string for backward compatibility
				Required:    true,
				Description: "The site ID where the secret will be created.",
			},
			"secrettemplateid": schema.StringAttribute{ // Changed to string for backward compatibility
				Required:    true,
				Description: "The template ID in which the secret will be created.",
			},
			"secretpolicyid": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "The ID of the secret policy.",
			},
			"passwordtypewebscriptid": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "The ID of the password type web script.",
			},
			"launcherconnectassecretid": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "The ID of the launcher connect-as secret.",
			},
			"checkoutintervalminutes": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "The checkout interval in minutes.",
			},
			"active": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the secret is active.",
			},
			"checkedout": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the secret is checked out.",
			},
			"checkoutenabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether checkout is enabled for the secret.",
			},
			"autochangenabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether auto-change is enabled for the secret.",
			},

			"id": schema.Int64Attribute{
				Computed:    true,
				Description: "The ID of the secret.",
			},

			"checkoutchangepasswordenabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether checkout change password is enabled.",
			},
			"delayindexing": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether delay indexing is enabled.",
			},
			"enableinheritpermissions": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether inherit permissions is enabled.",
			},
			"enableinheritsecretpolicy": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether inherit secret policy is enabled.",
			},
			"proxyenabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether proxy is enabled.",
			},
			"requirescomment": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether a comment is required.",
			},
			"sessionrecordingenabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether session recording is enabled.",
			},
			"weblauncherrequiresincognitomode": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Whether the web launcher requires incognito mode.",
			},
		},
		Blocks: map[string]schema.Block{
			"fields": schema.ListNestedBlock{
				Description: "List of fields for the secret.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"fieldname": schema.StringAttribute{
							Optional: true,
						},
						"itemvalue": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Sensitive:   true,
							Description: "The value of the field. For SSH key generation, this will be computed by the server. Prefer password_value for password fields so the value does not land in state.",
							PlanModifiers: []planmodifier.String{
								sshKeyFieldPlanModifier{},
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"password_value": schema.StringAttribute{
							Optional:    true,
							WriteOnly:   true,
							Sensitive:   true,
							Description: "Write-only password value for password fields. Never persisted in Terraform state (framework-enforced). Pair with password_wo_version to trigger rotation.",
						},
						"password_wo_version": schema.Int64Attribute{
							Optional:    true,
							Description: "Rotation trigger for password_value or generate. Bump this integer to signal Terraform that the password_value has changed and should be re-sent to TSS, or that a new password should be generated when generate=true; any new value is fine, only the change matters.",
						},
						"generate": schema.BoolAttribute{
							Optional:    true,
							Description: "Request server-side password generation from the template's password-requirement policy (closes gh #110). Only honored on fields the template marks as password fields. Mutually exclusive with password_value and itemvalue. Pair with password_wo_version to rotate the generated password.",
						},
						"itemid": schema.Int64Attribute{
							Computed:    true,
							Description: "Server-assigned database ID of this field-value record. Populated after apply; do not set in config.",
						},
						"fieldid": schema.Int64Attribute{
							Computed:    true,
							Description: "Secret Server template field ID; stable per template, shared across every secret that uses the template. Populated after apply; do not set in config.",
						},
						"fileattachmentid": schema.Int64Attribute{
							Optional:    true,
							Computed:    true,
							Description: "File attachment ID. Only meaningful for file-type fields; genuinely user-settable there as an alternative to uploading a new file.",
						},
						"slug": schema.StringAttribute{
							Computed:    true,
							Description: "Field's URL slug, assigned by the template. Populated after apply; do not set in config.",
						},
						"fielddescription": schema.StringAttribute{
							Computed:    true,
							Description: "Field description from the template. Populated after apply; do not set in config.",
						},
						"filename": schema.StringAttribute{
							Optional: true,
							Computed: true,
						},
						"isfile": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"isnotes": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"ispassword": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"islist": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"listtype": schema.StringAttribute{
							Optional: true,
							Computed: true,
						},
					},
				},
			},
			"sshkeyargs": schema.SingleNestedBlock{
				Description: "SSH key generation arguments.",
				Attributes: map[string]schema.Attribute{
					"generatepassphrase": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Whether to generate a passphrase for the SSH key.",
					},
					"generatesshkeys": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Description: "Whether to generate SSH keys.",
					},
				},
			},
		},
	}
}

func (r *TSSSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SecretResourceState

	// Read the state
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	fmt.Printf("[DEBUG] getting secret with id %d", int(state.ID.ValueInt64()))

	client := r.client

	// Retrieve the secret
	newState, notFound, readDiags := r.readSecretByID(ctx, int(state.ID.ValueInt64()), client, state.SecretTemplateID, state.Fields)
	if notFound {
		// The secret was deleted outside Terraform; drop it from state so the
		// next plan recreates it instead of wedging every refresh.
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve the SSH key args from the current state since the server doesn't return them
	if state.SshKeyArgs != nil {
		newState.SshKeyArgs = state.SshKeyArgs
	}

	// Determine if this secret was created with SSH key generation
	hasSshKeyArgs := false
	if state.SshKeyArgs != nil &&
		(state.SshKeyArgs.GenerateSshKeys.ValueBool() ||
			state.SshKeyArgs.GeneratePassphrase.ValueBool()) {
		hasSshKeyArgs = true
	}

	// Preserve file attachment information for file fields and SSH key fields
	for i, field := range newState.Fields {
		fieldName := field.FieldName.ValueString()
		isSSHKeyField := hasSshKeyArgs && (strings.Contains(strings.ToLower(fieldName), "key") ||
			strings.Contains(strings.ToLower(fieldName), "passphrase"))

		if field.IsFile.ValueBool() || isSSHKeyField {
			// Find the matching field in the old state
			for _, oldField := range state.Fields {
				if oldField.FieldName.ValueString() == fieldName {
					// Preserve FileAttachmentID and Filename
					if !oldField.FileAttachmentID.IsNull() {
						newState.Fields[i].FileAttachmentID = oldField.FileAttachmentID
					}
					if !oldField.Filename.IsNull() && oldField.Filename.ValueString() != "" {
						newState.Fields[i].Filename = oldField.Filename
						fmt.Printf("[DEBUG] Read: Preserved filename %s for field %s\n",
							oldField.Filename.ValueString(), fieldName)
					}
					break
				}
			}
		}
	}

	// Set the state
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
}

// recordTaintedSecret persists just the secret's ID so Terraform tracks a
// created-but-unconfirmed secret as tainted instead of orphaning it, then
// reports the failure with the standard replace-on-next-apply guidance.
func recordTaintedSecret(ctx context.Context, resp *resource.CreateResponse, id int, summary, detail string) {
	partialState := SecretResourceState{ID: types.Int64Value(int64(id))}
	resp.Diagnostics.Append(resp.State.Set(ctx, &partialState)...)
	resp.Diagnostics.AddError(summary,
		detail+" The secret ID has been recorded in state; Terraform will replace the resource on the next apply.")
}

// readSecretByID fetches a secret and converts it to resource state. notFound
// is true only for a definitive 404 on a secret whose template carries no
// file fields: the SDK's attachment sub-requests surface their own 404s
// indistinguishably from the primary GET, and the secret's real field set —
// which the template describes, unlike the config-aligned reference — decides
// whether those sub-requests run. Classic Secret Server reports deleted
// secrets as access denied (never 404); that case surfaces as an error with
// recovery guidance.
func (r *TSSSecretResource) readSecretByID(ctx context.Context, id int, client *server.Server, templateID types.String, reference []SecretField) (state *SecretResourceState, notFound bool, diags diag.Diagnostics) {
	// Retrieve the secret using the provided client
	secret, err := client.SecretContext(ctx, id)
	if err != nil {
		if isSecretNotFound(err) {
			if tid, convErr := stringToInt(templateID); convErr == nil {
				if template, tErr := client.SecretTemplateContext(ctx, tid); tErr == nil && !templateHasFileField(template) {
					return nil, true, nil
				}
			}
		}
		detail := fmt.Sprintf("Failed to retrieve secret: %s.", err)
		if isSecretGone(err) {
			detail += fmt.Sprintf(" Secret Server reports deleted secrets as access denied, so if secret %d was deleted outside Terraform, remove it with `terraform state rm` and re-apply.", id)
		}
		return nil, false, diag.Diagnostics{
			diag.NewErrorDiagnostic("Secret Retrieval Error", detail),
		}
	}

	state, err = flattenSecret(secret)
	if err != nil {
		return nil, false, diag.Diagnostics{
			diag.NewErrorDiagnostic("State Error", fmt.Sprintf("Failed to flatten secret: %s", err)),
		}
	}

	state.Fields = alignFieldsToReference(state.Fields, reference)

	return state, false, nil
}

// templateHasFileField reports whether the template defines any file
// attachment field.
func templateHasFileField(template *server.SecretTemplate) bool {
	for _, f := range template.Fields {
		if f.IsFile {
			return true
		}
	}
	return false
}

// mergeWriteOnlyFieldValues copies write-only attribute values (password_value)
// from the config-derived slice into the plan-derived slice, matching by
// fieldname. WriteOnly attributes are absent from Plan and State by framework
// design and are only available via req.Config; Create/Update merge them back
// in before building the TSS API request.
func mergeWriteOnlyFieldValues(plan, config []SecretField) {
	for i, p := range plan {
		for _, c := range config {
			if strings.EqualFold(p.FieldName.ValueString(), c.FieldName.ValueString()) {
				if !c.PasswordValue.IsNull() {
					plan[i].PasswordValue = c.PasswordValue
				}
				break
			}
		}
	}
}

// alignFieldsToReference returns fields whose names appear in reference, in reference
// order. Used so post-apply state mirrors the fields the user specified in config;
// without this, TSS templates that define more fields than the user listed would
// trigger Terraform's "Provider produced inconsistent result after apply" error.
// A nil reference disables filtering (returns fields unchanged).
//
// For each matched field, user-set attributes that the TSS API does not round-trip
// are copied from the reference into the aligned result: password_wo_version is
// the rotation trigger for the write-only password_value attribute, and generate
// is the request-server-side-generation flag — both live only on the Terraform
// side and must be preserved from plan (on Create/Update) or prior state (on Read).
func alignFieldsToReference(fields []SecretField, reference []SecretField) []SecretField {
	if reference == nil {
		return fields
	}
	byName := make(map[string]SecretField, len(fields))
	for _, f := range fields {
		byName[strings.ToLower(f.FieldName.ValueString())] = f
	}
	aligned := make([]SecretField, 0, len(reference))
	for _, r := range reference {
		if f, ok := byName[strings.ToLower(r.FieldName.ValueString())]; ok {
			f.PasswordWoVersion = r.PasswordWoVersion
			f.Generate = r.Generate
			aligned = append(aligned, f)
		}
	}
	return aligned
}

func (r *TSSSecretResource) getSecretData(ctx context.Context, state *SecretResourceState, client *server.Server) (*server.Secret, error) {
	// Convert string attributes to integers
	folderID, err := stringToInt(state.FolderID)
	if err != nil {
		return nil, fmt.Errorf("invalid Folder ID: %w", err)
	}
	siteID, err := stringToInt(state.SiteID)
	if err != nil {
		return nil, fmt.Errorf("invalid Site ID: %w", err)
	}
	templateID, err := stringToInt(state.SecretTemplateID)
	if err != nil {
		return nil, fmt.Errorf("invalid Template ID: %w", err)
	}

	// Fetch the secret template
	template, err := client.SecretTemplateContext(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve secret template: %w", err)
	}

	// Construct the fields dynamically
	var fields []server.SecretField
	for _, field := range state.Fields {
		templateField := server.SecretTemplateField{}
		fieldName := field.FieldName.ValueString()

		// Match the field name with the template fields
		matched := false
		for _, record := range template.Fields {
			if strings.EqualFold(record.Name, fieldName) || strings.EqualFold(record.FieldSlugName, fieldName) {
				templateField = record
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("field %q is not defined on template %q (id %d); check the fieldname against the template's field names or slugs", fieldName, template.Name, templateID)
		}

		hasGenerate := !field.Generate.IsNull() && field.Generate.ValueBool()
		hasPasswordValue := !field.PasswordValue.IsNull() && field.PasswordValue.ValueString() != ""
		hasItemValue := !field.ItemValue.IsNull() && field.ItemValue.ValueString() != ""

		if hasGenerate {
			if !templateField.IsPassword {
				return nil, fmt.Errorf("field %q has generate=true but is not a password field on template %q; only fields with isPassword=true support template-policy generation", fieldName, template.Name)
			}
			if hasPasswordValue {
				return nil, fmt.Errorf("field %q has both generate=true and password_value set; they are mutually exclusive", fieldName)
			}
			if hasItemValue {
				return nil, fmt.Errorf("field %q has both generate=true and itemvalue set; they are mutually exclusive on a password field", fieldName)
			}
		}

		var itemValue string
		switch {
		case hasGenerate:
			pw, err := client.GeneratePasswordContext(ctx, templateField.FieldSlugName, template)
			if err != nil {
				return nil, fmt.Errorf("failed to generate password for field %q from template policy: %w", fieldName, err)
			}
			itemValue = pw
		case hasPasswordValue:
			itemValue = field.PasswordValue.ValueString()
		case hasItemValue:
			itemValue = field.ItemValue.ValueString()
		case templateField.IsPassword:
			// Null/empty plan for a password field on Update: omit entirely so TSS preserves
			// the existing server-side value rather than blanking it.
			continue
		default:
			itemValue = ""
			fmt.Printf("[DEBUG] Field with null/empty value detected: %s\n", fieldName)
		}

		secretField := server.SecretField{
			FieldDescription: templateField.Description,
			FieldID:          templateField.SecretTemplateFieldID,
			FieldName:        templateField.Name,
			IsFile:           templateField.IsFile,
			IsNotes:          templateField.IsNotes,
			IsPassword:       templateField.IsPassword,
			ItemValue:        itemValue,
			Slug:             templateField.FieldSlugName,
		}

		// For file attachments, preserve the FileAttachmentID and Filename
		if !field.IsFile.IsNull() && field.IsFile.ValueBool() {
			secretField.FileAttachmentID = int(field.FileAttachmentID.ValueInt64())
			secretField.Filename = field.Filename.ValueString()
		}

		fields = append(fields, secretField)
	}

	// Populate the secret object
	secret := &server.Secret{
		Name:             state.Name.ValueString(),
		FolderID:         folderID,
		SiteID:           siteID,
		SecretTemplateID: templateID,
		Fields:           fields,
		Active:           state.Active.ValueBool(),
	}

	// Handle SSH key args if provided - only during create operations
	// (We ensure this is nil during updates in the Update method)
	if state.SshKeyArgs != nil {
		secret.SshKeyArgs = &server.SshKeyArgs{
			GeneratePassphrase: state.SshKeyArgs.GeneratePassphrase.ValueBool(),
			GenerateSshKeys:    state.SshKeyArgs.GenerateSshKeys.ValueBool(),
		}
	}

	// Handle optional attributes
	if !state.SecretPolicyID.IsNull() {
		secret.SecretPolicyID = int(state.SecretPolicyID.ValueInt64())
	}
	if !state.PasswordTypeWebScriptID.IsNull() {
		secret.PasswordTypeWebScriptID = int(state.PasswordTypeWebScriptID.ValueInt64())
	}
	if !state.LauncherConnectAsSecretID.IsNull() {
		secret.LauncherConnectAsSecretID = int(state.LauncherConnectAsSecretID.ValueInt64())
	}
	if !state.CheckOutIntervalMinutes.IsNull() {
		secret.CheckOutIntervalMinutes = int(state.CheckOutIntervalMinutes.ValueInt64())
	}
	if !state.CheckedOut.IsNull() {
		secret.CheckedOut = state.CheckedOut.ValueBool()
	}
	if !state.CheckOutEnabled.IsNull() {
		secret.CheckOutEnabled = state.CheckOutEnabled.ValueBool()
	}
	if !state.AutoChangeEnabled.IsNull() {
		secret.AutoChangeEnabled = state.AutoChangeEnabled.ValueBool()
	}
	if !state.CheckOutChangePasswordEnabled.IsNull() {
		secret.CheckOutChangePasswordEnabled = state.CheckOutChangePasswordEnabled.ValueBool()
	}
	if !state.DelayIndexing.IsNull() {
		secret.DelayIndexing = state.DelayIndexing.ValueBool()
	}
	if !state.EnableInheritPermissions.IsNull() {
		secret.EnableInheritPermissions = state.EnableInheritPermissions.ValueBool()
	}
	if !state.EnableInheritSecretPolicy.IsNull() {
		secret.EnableInheritSecretPolicy = state.EnableInheritSecretPolicy.ValueBool()
	}
	if !state.ProxyEnabled.IsNull() {
		secret.ProxyEnabled = state.ProxyEnabled.ValueBool()
	}
	if !state.RequiresComment.IsNull() {
		secret.RequiresComment = state.RequiresComment.ValueBool()
	}
	if !state.SessionRecordingEnabled.IsNull() {
		secret.SessionRecordingEnabled = state.SessionRecordingEnabled.ValueBool()
	}
	if !state.WebLauncherRequiresIncognitoMode.IsNull() {
		secret.WebLauncherRequiresIncognitoMode = state.WebLauncherRequiresIncognitoMode.ValueBool()
	}

	return secret, nil
}

func flattenSecret(secret *server.Secret) (*SecretResourceState, error) {
	var fields []SecretField

	for _, f := range secret.Fields {
		var itemValue types.String
		if f.IsPassword {
			// Never write the server's password value into state. Use an empty
			// string rather than null so sshKeyFieldPlanModifier's null→""
			// normalization on a user's un-configured itemvalue doesn't perpetually
			// diff against null-in-state.
			itemValue = types.StringValue("")
		} else {
			itemValue = types.StringValue(f.ItemValue)
		}

		if !f.IsPassword && f.ItemValue == "" {
			fmt.Printf("[DEBUG] Flatten: Field '%s' has empty value\n", f.FieldName)
		}

		field := SecretField{
			FieldName:         types.StringValue(f.FieldName),
			ItemValue:         itemValue,
			PasswordValue:     types.StringNull(),
			PasswordWoVersion: types.Int64Null(),
			ItemID:            types.Int64Value(int64(f.ItemID)),
			FieldID:           types.Int64Value(int64(f.FieldID)),
			FileAttachmentID:  types.Int64Value(int64(f.FileAttachmentID)),
			Slug:              types.StringValue(f.Slug),
			FieldDescription:  types.StringValue(f.FieldDescription),
			Filename:          types.StringValue(f.Filename),
			IsFile:            types.BoolValue(f.IsFile),
			IsNotes:           types.BoolValue(f.IsNotes),
			IsPassword:        types.BoolValue(f.IsPassword),
		}

		// Handle file fields and potential SSH key fields
		if f.IsFile {
			field.FileAttachmentID = types.Int64Value(int64(f.FileAttachmentID))
			if f.Filename != "" {
				field.Filename = types.StringValue(f.Filename)
			}
		}

		// Special handling for SSH key fields - ensure they have filename if provided by server
		isSSHKeyField := strings.Contains(strings.ToLower(f.FieldName), "key") ||
			strings.Contains(strings.ToLower(f.FieldName), "passphrase")

		if isSSHKeyField && f.Filename != "" {
			field.Filename = types.StringValue(f.Filename)
			fmt.Printf("[DEBUG] Flatten: Found SSH key field %s with filename %s\n", f.FieldName, f.Filename)
		}

		fields = append(fields, field)
	}

	state := &SecretResourceState{
		Name:             types.StringValue(secret.Name),
		ID:               types.Int64Value(int64(secret.ID)),
		FolderID:         types.StringValue(strconv.Itoa(secret.FolderID)),
		SiteID:           types.StringValue(strconv.Itoa(secret.SiteID)),
		SecretTemplateID: types.StringValue(strconv.Itoa(secret.SecretTemplateID)),
		Fields:           fields,
		Active:           types.BoolValue(secret.Active),
	}

	// Handle SSH key args if present
	if secret.SshKeyArgs != nil {
		state.SshKeyArgs = &SshKeyArgs{
			GeneratePassphrase: types.BoolValue(secret.SshKeyArgs.GeneratePassphrase),
			GenerateSshKeys:    types.BoolValue(secret.SshKeyArgs.GenerateSshKeys),
		}
	}

	// Optional fields
	if secret.SecretPolicyID != 0 {
		state.SecretPolicyID = types.Int64Value(int64(secret.SecretPolicyID))
	}
	if secret.PasswordTypeWebScriptID != 0 {
		state.PasswordTypeWebScriptID = types.Int64Value(int64(secret.PasswordTypeWebScriptID))
	}
	if secret.LauncherConnectAsSecretID != 0 {
		state.LauncherConnectAsSecretID = types.Int64Value(int64(secret.LauncherConnectAsSecretID))
	}
	if secret.CheckOutIntervalMinutes != 0 {
		state.CheckOutIntervalMinutes = types.Int64Value(int64(secret.CheckOutIntervalMinutes))
	}
	state.CheckedOut = types.BoolValue(secret.CheckedOut)
	state.CheckOutEnabled = types.BoolValue(secret.CheckOutEnabled)
	state.AutoChangeEnabled = types.BoolValue(secret.AutoChangeEnabled)
	state.CheckOutChangePasswordEnabled = types.BoolValue(secret.CheckOutChangePasswordEnabled)
	state.DelayIndexing = types.BoolValue(secret.DelayIndexing)
	state.EnableInheritPermissions = types.BoolValue(secret.EnableInheritPermissions)
	state.EnableInheritSecretPolicy = types.BoolValue(secret.EnableInheritSecretPolicy)
	state.ProxyEnabled = types.BoolValue(secret.ProxyEnabled)
	state.RequiresComment = types.BoolValue(secret.RequiresComment)
	state.SessionRecordingEnabled = types.BoolValue(secret.SessionRecordingEnabled)
	state.WebLauncherRequiresIncognitoMode = types.BoolValue(secret.WebLauncherRequiresIncognitoMode)

	return state, nil
}

// Helper function to convert string to int
func stringToInt(value types.String) (int, error) {
	if value.IsNull() {
		return 0, nil
	}
	return strconv.Atoi(value.ValueString())
}

// sshKeyFieldPlanModifier is a custom plan modifier for SSH key fields
type sshKeyFieldPlanModifier struct{}

func (m sshKeyFieldPlanModifier) Description(ctx context.Context) string {
	return "If SSH key generation is enabled and the value is empty, mark as unknown so it can be computed."
}

func (m sshKeyFieldPlanModifier) MarkdownDescription(ctx context.Context) string {
	return "If SSH key generation is enabled and the value is empty, mark as unknown so it can be computed."
}

func (m sshKeyFieldPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Log the plan values for debugging
	fmt.Printf("[DEBUG] PlanModifyString field")

	// If user explicitly set a value (including empty string) in the config, respect it
	if !req.ConfigValue.IsNull() {
		fmt.Printf("[DEBUG] Using explicit config value\n")
		resp.PlanValue = req.ConfigValue
		return
	}

	// For creation with potentially computed values
	if req.State.Raw.IsNull() && (req.PlanValue.IsNull() || req.PlanValue.ValueString() == "") {
		// Determine if this value should be computed by SSH key generation
		if shouldComputeSshKeyValue(req) {
			fmt.Printf("[DEBUG] Marking value as computed for potential SSH key field\n")
			resp.PlanValue = types.StringUnknown()
			return
		}
	}

	// For null values in the plan, convert to empty string for consistency
	if req.PlanValue.IsNull() {
		fmt.Printf("[DEBUG] Converting null plan value to empty string\n")
		resp.PlanValue = types.StringValue("")
		return
	}

	// Otherwise, use the planned value as is
	resp.PlanValue = req.PlanValue
}

// Helper function to determine if a field value should be computed by SSH key generation
func shouldComputeSshKeyValue(req planmodifier.StringRequest) bool {
	// Only mark values as computed during creation for SSH key fields when SSH key generation is enabled

	// Check if this is a create operation (state is null)
	if !req.State.Raw.IsNull() {
		// This is an update, not a creation, so don't compute
		return false
	}

	// Check if the user explicitly set an empty string in the config
	// If they did, we should respect that and not compute a value
	if req.ConfigValue.IsNull() == false && req.ConfigValue.ValueString() == "" {
		// User explicitly set an empty string, preserve it
		fmt.Printf("[DEBUG] User explicitly set empty string in config, preserving\n")
		return false
	}

	// If we've reached here, it's a create operation and the field might need to be computed

	// Check if the path contains a field reference
	pathSteps := req.Path.Steps()
	if len(pathSteps) < 3 {
		return false
	}

	// Check if this is the "itemvalue" attribute within a "fields" block
	if pathSteps[0].String() != "fields" || pathSteps[len(pathSteps)-1].String() != "itemvalue" {
		return false
	}

	// At this point, we would ideally check:
	// 1. If this field is an SSH key field (by name)
	// 2. If SSH key generation is enabled in the plan
	//
	// However, without easy access to the field name here,
	// and since we don't have access to other parts of the plan,
	// we'll assume any null/empty field during create could be computed

	// For create operations with empty values that haven't been explicitly set,
	// mark as computed
	return req.PlanValue.ValueString() == ""
}
