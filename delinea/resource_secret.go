package delinea

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretResource defines the resource implementation
type TSSSecretResource struct {
	client *server.Server // Shared SDK client built once in Provider.Configure
}

var _ resource.ResourceWithModifyPlan = (*TSSSecretResource)(nil)

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

func (r *TSSSecretResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var plan, state, config SecretResourceState
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || len(plan.Fields) != len(config.Fields) || !plan.SecretTemplateID.Equal(state.SecretTemplateID) {
		return
	}
	changed := false
	for i := range plan.Fields {
		stateIndex, ok := priorConfiguredFieldIndex(state.Fields, config.Fields[i].FieldName.ValueString())
		if !ok {
			continue
		}
		if config.Fields[i].ItemValue.IsNull() && (plan.Fields[i].ItemValue.IsNull() || plan.Fields[i].ItemValue.IsUnknown()) {
			plan.Fields[i].ItemValue = state.Fields[stateIndex].ItemValue
			changed = true
		}
		if config.Fields[i].Filename.IsNull() && (plan.Fields[i].Filename.IsNull() || plan.Fields[i].Filename.IsUnknown()) {
			plan.Fields[i].Filename = state.Fields[stateIndex].Filename
			changed = true
		}
	}
	if changed {
		resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
	}
}

func priorConfiguredFieldIndex(fields []SecretField, configuredName string) (int, bool) {
	matched := -1
	for i := range fields {
		if !strings.EqualFold(fields[i].FieldName.ValueString(), configuredName) {
			continue
		}
		if matched != -1 {
			return 0, false
		}
		matched = i
	}
	return matched, matched != -1
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
	if err := mergeWriteOnlyFieldValues(plan.Fields, config.Fields); err != nil {
		resp.Diagnostics.AddError("Secret Data Error", err.Error())
		return
	}
	if err := validatePasswordRotationVersions(plan.Fields, nil, nil); err != nil {
		resp.Diagnostics.AddError("Secret Data Error", err.Error())
		return
	}

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	client := r.client

	template, err := r.getSecretTemplate(ctx, &plan, client)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", fmt.Sprintf("Failed to prepare secret data: %s", err))
		return
	}
	if err := validateConfiguredPasswordItemValues(config.Fields, template); err != nil {
		resp.Diagnostics.AddError("Secret Data Error", err.Error())
		return
	}

	// Get the secret data
	newSecret, err := r.getSecretDataWithTemplate(ctx, &plan, client, template)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", fmt.Sprintf("Failed to prepare secret data: %s", err))
		return
	}

	// Use the client to create the secret
	createdSecret, err := client.CreateSecretContext(ctx, *newSecret)
	if err != nil {
		// A PartialWriteError means Secret Server accepted the create but a
		// later SDK step (refresh, file upload) failed or was cancelled. The
		// secret exists server-side; record its ID so Terraform tracks it as
		// tainted instead of orphaning it and creating a duplicate next apply.
		if pwe, ok := errors.AsType[*server.PartialWriteError](err); ok {
			if pwe.SecretID != 0 {
				recordTaintedSecret(ctx, resp, plan, pwe.SecretID, "Secret Partially Created",
					fmt.Sprintf("Secret %d was created on Secret Server but a later step failed: %s.", pwe.SecretID, err))
				return
			}
			resp.Diagnostics.AddError("Secret Possibly Created",
				fmt.Sprintf("Secret Server may have accepted the create, but the response did not yield a usable secret ID: %s. %s", err, duplicateCheckHint(newSecret.Name)))
			return
		}
		if isServerRejection(err) {
			resp.Diagnostics.AddError("Secret Creation Error", fmt.Sprintf("Failed to create secret: %s", err))
			return
		}
		resp.Diagnostics.AddError("Secret Creation Error",
			fmt.Sprintf("Failed to create secret: %s. The request may still have reached Secret Server. %s", err, duplicateCheckHint(newSecret.Name)))
		return
	}

	// The v3 SDK's write calls return the secret already refreshed
	// server-side; reuse it instead of paying a second GET (plus attachment
	// downloads) per apply.
	newState, err := secretToState(createdSecret, plan.Fields)
	if err != nil {
		// The create succeeded; only local state conversion failed. Record
		// recovery identifiers so Terraform tracks the secret as tainted.
		recordTaintedSecret(ctx, resp, plan, createdSecret.ID, "Secret Created, State Conversion Failed",
			fmt.Sprintf("Secret %d was created but its returned representation could not be converted to state: %s.", createdSecret.ID, err))
		return
	}

	// Preserve the SSH key args from the plan since the server doesn't return them
	if plan.SshKeyArgs != nil {
		newState.SshKeyArgs = plan.SshKeyArgs
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
	if err := mergeWriteOnlyFieldValues(plan.Fields, config.Fields); err != nil {
		resp.Diagnostics.AddError("Secret Data Error", err.Error())
		return
	}

	if !requireClient(r.client, &resp.Diagnostics) {
		return
	}

	client := r.client
	secretID, err := toPositiveServerInt(state.ID.ValueInt64(), "id")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", err.Error())
		return
	}

	// Get the secret data
	// During update, we shouldn't send SSH key generation parameters
	// because the server doesn't support SSH key generation during update
	updatePlan := plan
	fillUnknownSecretSettingsFromState(&updatePlan, state)
	if updatePlan.Active.IsNull() {
		updatePlan.Active = state.Active
	}

	// Don't send SSH key args during update - they're only for creation
	updatePlan.SshKeyArgs = nil
	templateUnchanged := plan.SecretTemplateID.Equal(state.SecretTemplateID)
	referenceFields := discardStaleFieldSlugs(updatePlan.Fields, state.Fields, templateUnchanged)
	updatePlan.Fields = referenceFields

	template, err := r.getSecretTemplate(ctx, &updatePlan, client)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", fmt.Sprintf("Failed to prepare secret data: %s", err))
		return
	}
	if err := validateConfiguredPasswordItemValues(config.Fields, template); err != nil {
		resp.Diagnostics.AddError("Secret Data Error", err.Error())
		return
	}
	normalizedFields, matchedState, err := normalizeUpdateFieldValues(referenceFields, config.Fields, state.Fields, template, templateUnchanged)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", err.Error())
		return
	}
	normalizedFields, err = preparePasswordRotationFields(normalizedFields, state.Fields, template, templateUnchanged)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", err.Error())
		return
	}
	updatePlan.Fields, err = updateRequestFields(normalizedFields, config.Fields, matchedState)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", err.Error())
		return
	}
	updatedSecret, err := r.getSecretDataWithTemplate(ctx, &updatePlan, client, template)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", fmt.Sprintf("Failed to prepare secret data: %s", err))
		return
	}

	preserveSSHKeyFieldValues(updatedSecret.Fields, state.Fields, referenceFields, state.SshKeyArgs)

	// Update the secret
	updatedSecret.ID = secretID
	updatedResult, err := client.UpdateSecretContext(ctx, *updatedSecret)
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

	// The v3 SDK's write calls return the secret already refreshed
	// server-side; reuse it instead of paying a second GET (plus attachment
	// downloads) per apply.
	newState, err := secretToState(updatedResult, referenceFields)
	if err != nil {
		resp.Diagnostics.AddError("State Error",
			fmt.Sprintf("Secret %d was updated but its returned representation could not be converted to state: %s. State keeps the pre-update values; the next plan re-syncs.", updatedSecret.ID, err))
		return
	}

	// Preserve the SSH key args from the plan since the server doesn't return them
	if plan.SshKeyArgs != nil {
		newState.SshKeyArgs = plan.SshKeyArgs
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

	client := r.client

	// Delete the secret
	secretID, err := toPositiveServerInt(state.ID.ValueInt64(), "id")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", err.Error())
		return
	}
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
			// Do not drop a possibly-live secret from state.
			resp.Diagnostics.AddError("Secret Deletion Unverified", deleteAmbiguousDetail(secretID, err, true))
		default:
			resp.Diagnostics.AddError("Secret Deletion Error", fmt.Sprintf("Failed to delete secret: %s", err))
		}
		return
	}
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
			"secretpolicyid":            optionalComputedInt64("The ID of the secret policy."),
			"passwordtypewebscriptid":   optionalComputedInt64("The ID of the password type web script."),
			"launcherconnectassecretid": optionalComputedInt64("The ID of the launcher connect-as secret."),
			"checkoutintervalminutes":   optionalComputedInt64("The checkout interval in minutes."),
			"active":                    optionalComputedBool("Whether the secret is active. New secrets default to active when omitted."),
			"checkedout":                optionalComputedBool("Whether the secret is checked out."),
			"checkoutenabled":           optionalComputedBool("Whether checkout is enabled for the secret."),
			"autochangenabled":          optionalComputedBool("Whether auto-change is enabled for the secret."),
			"id": schema.Int64Attribute{
				Computed:    true,
				Description: "The ID of the secret.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"checkoutchangepasswordenabled":    optionalComputedBool("Whether checkout change password is enabled."),
			"delayindexing":                    optionalComputedBool("Whether delay indexing is enabled."),
			"enableinheritpermissions":         optionalComputedBool("Whether inherit permissions is enabled."),
			"enableinheritsecretpolicy":        optionalComputedBool("Whether inherit secret policy is enabled."),
			"proxyenabled":                     optionalComputedBool("Whether proxy is enabled."),
			"requirescomment":                  optionalComputedBool("Whether a comment is required."),
			"sessionrecordingenabled":          optionalComputedBool("Whether session recording is enabled."),
			"weblauncherrequiresincognitomode": optionalComputedBool("Whether the web launcher requires incognito mode."),
		},
		Blocks: map[string]schema.Block{
			"fields": schema.ListNestedBlock{
				Description: "List of fields for the secret.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"fieldname": schema.StringAttribute{
							Required:    true,
							Description: "The template field's display name or slug (case-insensitive).",
						},
						"itemvalue": schema.StringAttribute{
							Optional:    true,
							Computed:    true,
							Sensitive:   true,
							Description: "The value of the field. For SSH key generation, this will be computed by the server. Prefer password_value for password fields so the value does not land in state.",
							PlanModifiers: []planmodifier.String{
								sshKeyFieldPlanModifier{},
							},
						},
						"password_value": schema.StringAttribute{
							Optional:    true,
							WriteOnly:   true,
							Sensitive:   true,
							Description: "Write-only password value for password fields. Never persisted in Terraform state (framework-enforced). Requires password_wo_version so Terraform can detect later rotations.",
							Validators: []validator.String{
								passwordValueValidator{},
							},
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
							Computed:    true,
							Description: "Server-assigned attachment ID for a file-type field. Populated after apply; do not set in config.",
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
							Optional:    true,
							Computed:    true,
							Description: "Filename for a file-type field. Configured filenames must end in an extension containing only ASCII letters, digits, or underscores; omit this attribute to use Secret Server's File.txt default.",
							Validators: []validator.String{
								filenameValidator{},
							},
						},
						"isfile": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether the template defines this as a file field.",
						},
						"isnotes": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether the template defines this as a notes field.",
						},
						"ispassword": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether the template defines this as a password field.",
						},
						"islist": schema.BoolAttribute{
							Computed:    true,
							Description: "Whether the template defines this as a list field, when returned by Secret Server.",
						},
						"listtype": schema.StringAttribute{
							Computed:    true,
							Description: "The template list type, when returned by Secret Server.",
						},
					},
				},
			},
			"sshkeyargs": schema.SingleNestedBlock{
				Description: "SSH key generation arguments used during creation. Changing this block replaces the secret.",
				PlanModifiers: []planmodifier.Object{
					objectplanmodifier.RequiresReplace(),
				},
				Attributes: map[string]schema.Attribute{
					"generatepassphrase": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "Whether to generate a passphrase for the SSH key.",
					},
					"generatesshkeys": schema.BoolAttribute{
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
						Description: "Whether to generate SSH keys.",
					},
				},
			},
		},
	}
}

// optionalComputedBool and optionalComputedInt64 declare the secret-level
// settings Secret Server owns unless the configuration sets them. When any
// attribute changes, the framework marks every null-config Computed attribute
// unknown in the plan; without UseStateForUnknown an update would send the
// zero value and silently disable server-side settings such as checkout,
// comment requirements, or permission inheritance.
func optionalComputedBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		Optional:    true,
		Computed:    true,
		Description: description + " When omitted, the value Secret Server holds is preserved across updates.",
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseStateForUnknown(),
		},
	}
}

func optionalComputedInt64(description string) schema.Int64Attribute {
	return schema.Int64Attribute{
		Optional:    true,
		Computed:    true,
		Description: description + " When omitted, the value Secret Server holds is preserved across updates.",
		PlanModifiers: []planmodifier.Int64{
			int64planmodifier.UseStateForUnknown(),
		},
	}
}

// fillUnknownSecretSettingsFromState is the apply-time backstop for the plan
// modifiers above: any secret-level setting still unknown when Update runs is
// taken from prior state so the PUT never carries a zero value the operator
// did not configure.
func fillUnknownSecretSettingsFromState(plan *SecretResourceState, state SecretResourceState) {
	bools := []struct{ plan, state *types.Bool }{
		{&plan.Active, &state.Active},
		{&plan.CheckedOut, &state.CheckedOut},
		{&plan.CheckOutEnabled, &state.CheckOutEnabled},
		{&plan.AutoChangeEnabled, &state.AutoChangeEnabled},
		{&plan.CheckOutChangePasswordEnabled, &state.CheckOutChangePasswordEnabled},
		{&plan.DelayIndexing, &state.DelayIndexing},
		{&plan.EnableInheritPermissions, &state.EnableInheritPermissions},
		{&plan.EnableInheritSecretPolicy, &state.EnableInheritSecretPolicy},
		{&plan.ProxyEnabled, &state.ProxyEnabled},
		{&plan.RequiresComment, &state.RequiresComment},
		{&plan.SessionRecordingEnabled, &state.SessionRecordingEnabled},
		{&plan.WebLauncherRequiresIncognitoMode, &state.WebLauncherRequiresIncognitoMode},
	}
	for _, b := range bools {
		if b.plan.IsUnknown() {
			*b.plan = *b.state
		}
	}
	ints := []struct{ plan, state *types.Int64 }{
		{&plan.SecretPolicyID, &state.SecretPolicyID},
		{&plan.PasswordTypeWebScriptID, &state.PasswordTypeWebScriptID},
		{&plan.LauncherConnectAsSecretID, &state.LauncherConnectAsSecretID},
		{&plan.CheckOutIntervalMinutes, &state.CheckOutIntervalMinutes},
	}
	for _, i := range ints {
		if i.plan.IsUnknown() {
			*i.plan = *i.state
		}
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

	client := r.client

	// Retrieve the secret
	secretID, err := toPositiveServerInt(state.ID.ValueInt64(), "id")
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", err.Error())
		return
	}
	newState, readDiags := r.readSecretByID(ctx, secretID, client, state.Fields)
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Preserve the SSH key args from the current state since the server doesn't return them
	if state.SshKeyArgs != nil {
		newState.SshKeyArgs = state.SshKeyArgs
	}

	// Set the state
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
}

// recordTaintedSecret persists the identifiers needed to verify absence and
// destroy a created-but-unconfirmed secret without retaining write-only data.
func recordTaintedSecret(ctx context.Context, resp *resource.CreateResponse, plan SecretResourceState, id int, summary, detail string) {
	partialState := SecretResourceState{
		ID:               types.Int64Value(int64(id)),
		Name:             plan.Name,
		FolderID:         plan.FolderID,
		SiteID:           plan.SiteID,
		SecretTemplateID: plan.SecretTemplateID,
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &partialState)...)
	resp.Diagnostics.AddError(summary,
		detail+" The secret identifiers have been recorded in state so Terraform can attempt replacement on the next apply. If Secret Server no longer has the secret and its absence cannot be verified automatically, confirm that it is gone and run `terraform state rm` before re-applying.")
}

// readSecretByID fetches a secret and converts it to resource state.
func (r *TSSSecretResource) readSecretByID(ctx context.Context, id int, client *server.Server, reference []SecretField) (state *SecretResourceState, diags diag.Diagnostics) {
	// Retrieve the secret using the provided client
	secret, err := client.SecretContext(ctx, id)
	if err != nil {
		if isAuthenticationFailure(err) {
			return nil, diag.Diagnostics{
				diag.NewErrorDiagnostic("Secret Authentication or Authorization Failed", authenticationFailureDetail("read", id, err)),
			}
		}
		if isSecretNotFound(err) {
			return nil, diag.Diagnostics{
				diag.NewErrorDiagnostic("Secret Retrieval Error",
					fmt.Sprintf("Secret %d returned 404, but the SDK cannot distinguish a missing secret from a missing attachment on a live secret. Verify the secret and its attachments in Secret Server; if the secret is confirmed gone, run `terraform state rm` before re-applying.", id)),
			}
		}
		detail := fmt.Sprintf("Failed to retrieve secret: %s.", err)
		if isSecretGone(err) {
			detail += fmt.Sprintf(" Secret Server reports deleted secrets as access denied, so if secret %d was deleted outside Terraform, remove it with `terraform state rm` and re-apply.", id)
		}
		return nil, diag.Diagnostics{
			diag.NewErrorDiagnostic("Secret Retrieval Error", detail),
		}
	}

	state, err = secretToState(secret, reference)
	if err != nil {
		return nil, diag.Diagnostics{
			diag.NewErrorDiagnostic("State Error", fmt.Sprintf("Failed to flatten secret: %s", err)),
		}
	}

	return state, nil
}

// secretToState converts an SDK secret to resource state aligned to the
// caller's reference fields.
func secretToState(secret *server.Secret, reference []SecretField) (*SecretResourceState, error) {
	state, err := flattenSecret(secret)
	if err != nil {
		return nil, err
	}
	for _, field := range reference {
		if !referenceTreatsFieldAsPassword(field) {
			continue
		}
		if i, ok := referenceFieldIndex(state.Fields, field); ok && !state.Fields[i].IsPassword.ValueBool() {
			return nil, fmt.Errorf("field %q was previously treated as a password but Secret Server now reports it as a non-password field; refusing to persist its value in Terraform state", field.FieldName.ValueString())
		}
	}
	state.Fields = alignFieldsToReference(state.Fields, reference)
	return state, nil
}

// suppressUnrotatedGenerates returns a copy of planFields with generate
// cleared on every field that already had generate=true in state with an
// unchanged password_wo_version, so only a version bump re-triggers
// server-side regeneration. A field newly switched to generate=true is a
// rotation request and is left alone.
func suppressUnrotatedGenerates(planFields, stateFields []SecretField, template *server.SecretTemplate) []SecretField {
	out := make([]SecretField, len(planFields))
	copy(out, planFields)
	for i, p := range out {
		if !p.Generate.ValueBool() {
			continue
		}
		if templateField, matched := resolveTemplateField(template, p); matched && !templateField.IsPassword {
			continue
		}
		// A field that also sets password_value or itemvalue must keep
		// generate=true so getSecretData's mutual-exclusion validation fires
		// instead of the conflicting value being sent silently.
		if (!p.PasswordValue.IsNull() && p.PasswordValue.ValueString() != "") ||
			(!p.ItemValue.IsNull() && p.ItemValue.ValueString() != "") {
			continue
		}
		for _, s := range stateFields {
			if sameTemplateField(template, p, s) {
				// Removing the password_wo_version line (plan null) is
				// cleanup, not a rotation request.
				if s.Generate.ValueBool() && (p.PasswordWoVersion.IsNull() || p.PasswordWoVersion.Equal(s.PasswordWoVersion)) {
					out[i].Generate = types.BoolValue(false)
				}
				break
			}
		}
	}
	return out
}

// mergeWriteOnlyFieldValues copies write-only attribute values (password_value)
// from the config-derived slice into the same list positions in the
// plan-derived slice. WriteOnly attributes are absent from Plan and State by
// framework design and are only available via req.Config; Create/Update merge
// them back in before building the TSS API request.
func mergeWriteOnlyFieldValues(plan, config []SecretField) error {
	if len(plan) != len(config) {
		return fmt.Errorf("fields plan/config length mismatch (%d != %d); refusing to associate write-only password values", len(plan), len(config))
	}
	for i := range plan {
		if !strings.EqualFold(plan[i].FieldName.ValueString(), config[i].FieldName.ValueString()) {
			return fmt.Errorf("fields plan/config mismatch at index %d; refusing to associate a write-only password value with a different field", i)
		}
		if !config[i].PasswordValue.IsNull() {
			plan[i].PasswordValue = config[i].PasswordValue
		}
	}
	return nil
}

func discardStaleFieldSlugs(planFields, stateFields []SecretField, templateUnchanged bool) []SecretField {
	out := make([]SecretField, len(planFields))
	copy(out, planFields)
	for i := range out {
		out[i].Slug = types.StringNull()
		if !templateUnchanged {
			continue
		}
		for _, stateField := range stateFields {
			if strings.EqualFold(out[i].FieldName.ValueString(), stateField.FieldName.ValueString()) {
				out[i].Slug = stateField.Slug
				break
			}
		}
	}
	return out
}

type updateFieldMetadata struct {
	matchedState            bool
	existingEmptyAttachment bool
	filenameChanged         bool
}

func normalizeUpdateFieldValues(planFields, configFields, stateFields []SecretField, template *server.SecretTemplate, templateUnchanged bool) ([]SecretField, []updateFieldMetadata, error) {
	if len(planFields) != len(configFields) {
		return nil, nil, fmt.Errorf("fields plan/config length mismatch (%d != %d); refusing to associate field values", len(planFields), len(configFields))
	}
	out := make([]SecretField, len(planFields))
	copy(out, planFields)
	metadata := make([]updateFieldMetadata, len(out))
	for i := range out {
		stateIndex := -1
		if templateUnchanged {
			for j := range stateFields {
				if !sameTemplateField(template, out[i], stateFields[j]) {
					continue
				}
				if stateIndex != -1 {
					return nil, nil, fmt.Errorf("field %q matches multiple prior-state fields; refusing to associate their values", out[i].FieldName.ValueString())
				}
				stateIndex = j
			}
		}
		if stateIndex != -1 {
			stateField := stateFields[stateIndex]
			metadata[i].matchedState = true
			metadata[i].existingEmptyAttachment = stateField.FileAttachmentID.ValueInt64() > 0 && stateField.ItemValue.ValueString() == ""
			metadata[i].filenameChanged = !configFields[i].Filename.IsNull() && !configFields[i].Filename.IsUnknown() && configFields[i].Filename.ValueString() != stateField.Filename.ValueString()
		}
		switch {
		case !configFields[i].ItemValue.IsNull():
			out[i].ItemValue = configFields[i].ItemValue
		case stateIndex != -1:
			out[i].ItemValue = stateFields[stateIndex].ItemValue
		default:
			out[i].ItemValue = types.StringNull()
		}
		switch {
		case !configFields[i].Filename.IsNull():
			out[i].Filename = configFields[i].Filename
		case stateIndex != -1:
			out[i].Filename = stateFields[stateIndex].Filename
		default:
			out[i].Filename = types.StringNull()
		}
	}
	return out, metadata, nil
}

func updateRequestFields(fields, configFields []SecretField, metadata []updateFieldMetadata) ([]SecretField, error) {
	if len(fields) != len(configFields) || len(fields) != len(metadata) {
		return nil, fmt.Errorf("fields update metadata length mismatch; refusing to construct the request")
	}
	requestFields := make([]SecretField, 0, len(fields))
	for i, field := range fields {
		explicitItemValue := !configFields[i].ItemValue.IsNull()
		explicitFilename := !configFields[i].Filename.IsNull()
		if explicitItemValue && configFields[i].ItemValue.IsUnknown() {
			return nil, fmt.Errorf("field %q has an unknown itemvalue; the value must be known before apply", field.FieldName.ValueString())
		}
		if explicitFilename && configFields[i].Filename.IsUnknown() {
			return nil, fmt.Errorf("field %q has an unknown filename; the value must be known before apply", field.FieldName.ValueString())
		}
		if !metadata[i].matchedState && explicitFilename && !explicitItemValue {
			return nil, fmt.Errorf("field %q sets filename but was not previously managed; configure itemvalue as well so the provider does not replace an existing attachment with empty content", field.FieldName.ValueString())
		}
		if metadata[i].existingEmptyAttachment && !explicitItemValue {
			if metadata[i].filenameChanged {
				return nil, fmt.Errorf("field %q changes the filename of an existing zero-byte attachment; configure itemvalue as well because Secret Server treats an empty file update as attachment deletion", field.FieldName.ValueString())
			}
			continue
		}
		passwordWrite := !field.PasswordValue.IsNull() || field.Generate.ValueBool()
		if metadata[i].matchedState || explicitItemValue || passwordWrite {
			requestFields = append(requestFields, field)
		}
	}
	return requestFields, nil
}

func validatePasswordRotationVersions(planFields, stateFields []SecretField, template *server.SecretTemplate) error {
	for _, planField := range planFields {
		if planField.PasswordValue.IsUnknown() {
			return fmt.Errorf("field %q has an unknown password_value; the write-only password must be known before apply", planField.FieldName.ValueString())
		}
		hasPasswordValue := !planField.PasswordValue.IsNull()
		if hasPasswordValue && planField.PasswordValue.ValueString() == "" {
			return fmt.Errorf("field %q sets password_value to an empty string; configure a non-empty password or omit password_value", planField.FieldName.ValueString())
		}
		if hasPasswordValue && planField.PasswordWoVersion.IsNull() {
			return fmt.Errorf("field %q sets password_value without password_wo_version; configure both so Terraform can detect and apply later password rotations", planField.FieldName.ValueString())
		}
		if planField.PasswordWoVersion.IsNull() {
			continue
		}
		requiresPasswordRequest := true
		for _, stateField := range stateFields {
			if !sameTemplateField(template, planField, stateField) {
				continue
			}
			if planField.PasswordWoVersion.Equal(stateField.PasswordWoVersion) {
				requiresPasswordRequest = false
			}
			break
		}
		if !requiresPasswordRequest {
			continue
		}
		if !hasPasswordValue && !planField.Generate.ValueBool() {
			return fmt.Errorf("field %q sets or changes password_wo_version without setting password_value or generate=true; configure the rotation value and password request together", planField.FieldName.ValueString())
		}
	}
	return nil
}

func preparePasswordRotationFields(planFields, stateFields []SecretField, template *server.SecretTemplate, templateUnchanged bool) ([]SecretField, error) {
	if !templateUnchanged {
		stateFields = nil
	}
	if err := validatePasswordRotationVersions(planFields, stateFields, template); err != nil {
		return nil, err
	}
	planFields = suppressUnrotatedPasswordValues(planFields, stateFields, template)
	return suppressUnrotatedGenerates(planFields, stateFields, template), nil
}

func suppressUnrotatedPasswordValues(planFields, stateFields []SecretField, template *server.SecretTemplate) []SecretField {
	out := make([]SecretField, len(planFields))
	copy(out, planFields)
	for i, planField := range out {
		if planField.PasswordValue.IsNull() || planField.PasswordValue.ValueString() == "" ||
			planField.Generate.ValueBool() || !planField.ItemValue.IsNull() && planField.ItemValue.ValueString() != "" {
			continue
		}
		if templateField, matched := resolveTemplateField(template, planField); matched && !templateField.IsPassword {
			continue
		}
		for _, stateField := range stateFields {
			if !sameTemplateField(template, planField, stateField) {
				continue
			}
			if !stateField.Generate.ValueBool() &&
				(planField.PasswordWoVersion.IsNull() || planField.PasswordWoVersion.Equal(stateField.PasswordWoVersion)) {
				out[i].PasswordValue = types.StringNull()
			}
			break
		}
	}
	return out
}

// alignFieldsToReference returns fields whose names appear in reference, in reference
// order. Used so post-apply state mirrors the fields the user specified in config;
// without this, TSS templates that define more fields than the user listed would
// trigger Terraform's "Provider produced inconsistent result after apply" error.
// For each matched field, user-set attributes that the TSS API does not round-trip
// are copied from the reference into the aligned result: password_wo_version is
// the rotation trigger for the write-only password_value attribute, and generate
// is the request-server-side-generation flag — both live only on the Terraform
// side and must be preserved from plan (on Create/Update) or prior state (on Read).
func alignFieldsToReference(fields []SecretField, reference []SecretField) []SecretField {
	aligned := make([]SecretField, 0, len(reference))
	for _, r := range reference {
		if i, ok := referenceFieldIndex(fields, r); ok {
			f := fields[i]
			// fieldname is configured, not computed. Preserve the exact spelling
			// from config (including a slug) so post-apply state remains
			// consistent with the plan rather than replacing it with the server's
			// canonical display name.
			f.FieldName = r.FieldName
			f.PasswordWoVersion = r.PasswordWoVersion
			f.Generate = r.Generate
			if referenceTreatsFieldAsPassword(r) {
				f.ItemValue = types.StringValue("")
			}
			aligned = append(aligned, f)
		}
	}
	return aligned
}

func referenceTreatsFieldAsPassword(field SecretField) bool {
	return field.IsPassword.ValueBool() || !field.PasswordValue.IsNull() || !field.PasswordWoVersion.IsNull() || field.Generate.ValueBool()
}

func validateConfiguredPasswordItemValues(fields []SecretField, template *server.SecretTemplate) error {
	for _, field := range fields {
		if field.ItemValue.IsNull() {
			continue
		}
		matches := templateFieldCandidates(template, field)
		if len(matches) == 1 && matches[0].IsPassword {
			return fmt.Errorf("field %q is a password field on template %q, and password fields do not accept itemvalue; set password_value together with password_wo_version (or generate = true) instead", field.FieldName.ValueString(), template.Name)
		}
	}
	return nil
}

func referenceFieldIndex(fields []SecretField, reference SecretField) (int, bool) {
	matched := -1
	for i := len(fields) - 1; i >= 0; i-- {
		if fieldNameMatches(reference.FieldName.ValueString(), fields[i].FieldName.ValueString(), fields[i].Slug.ValueString()) {
			if matched != -1 {
				if strings.EqualFold(fields[matched].FieldName.ValueString(), fields[i].FieldName.ValueString()) &&
					strings.EqualFold(fields[matched].Slug.ValueString(), fields[i].Slug.ValueString()) {
					continue
				}
				return 0, false
			}
			matched = i
		}
	}
	if matched != -1 {
		return matched, true
	}
	if slug := reference.Slug.ValueString(); slug != "" {
		matched = -1
		for i := len(fields) - 1; i >= 0; i-- {
			if fieldNameMatches(slug, fields[i].FieldName.ValueString(), fields[i].Slug.ValueString()) {
				if matched != -1 {
					if strings.EqualFold(fields[matched].FieldName.ValueString(), fields[i].FieldName.ValueString()) &&
						strings.EqualFold(fields[matched].Slug.ValueString(), fields[i].Slug.ValueString()) {
						continue
					}
					return 0, false
				}
				matched = i
			}
		}
		if matched != -1 {
			return matched, true
		}
	}
	return 0, false
}

func fieldNameMatches(name, canonical, slug string) bool {
	return strings.EqualFold(name, canonical) || slug != "" && strings.EqualFold(name, slug)
}

func fieldAliasesMatch(aName, aSlug, bName, bSlug string) bool {
	return fieldNameMatches(aName, bName, bSlug) || fieldNameMatches(aSlug, bName, bSlug)
}

func terraformFieldsMatch(a, b SecretField) bool {
	return fieldAliasesMatch(a.FieldName.ValueString(), a.Slug.ValueString(), b.FieldName.ValueString(), b.Slug.ValueString())
}

func serverFieldMatchesTerraform(serverField server.SecretField, terraformField SecretField) bool {
	return fieldAliasesMatch(serverField.FieldName, serverField.Slug, terraformField.FieldName.ValueString(), terraformField.Slug.ValueString())
}

func preserveSSHKeyFieldValues(fields []server.SecretField, stateFields, planFields []SecretField, args *SshKeyArgs) {
	if args == nil {
		return
	}
	for i, field := range fields {
		generatedFile := args.GenerateSshKeys.ValueBool() && field.IsFile
		generatedPassword := args.GeneratePassphrase.ValueBool() && field.IsPassword
		if !generatedFile && !generatedPassword {
			continue
		}
		for _, stateField := range stateFields {
			if !serverFieldMatchesTerraform(field, stateField) {
				continue
			}
			planFound := false
			preserveFilename := true
			for _, planField := range planFields {
				if !serverFieldMatchesTerraform(field, planField) {
					continue
				}
				planFound = true
				preserveFilename = planField.Filename.IsNull() || planField.Filename.IsUnknown()
				hasPasswordValue := !planField.PasswordValue.IsNull() && planField.PasswordValue.ValueString() != ""
				hasGenerate := !planField.Generate.IsNull() && planField.Generate.ValueBool()
				if !hasPasswordValue && !hasGenerate && (planField.ItemValue.IsNull() || planField.ItemValue.IsUnknown()) {
					fields[i].ItemValue = stateField.ItemValue.ValueString()
				}
				break
			}
			if !planFound {
				fields[i].ItemValue = stateField.ItemValue.ValueString()
			}
			if preserveFilename && !stateField.Filename.IsNull() && stateField.Filename.ValueString() != "" {
				fields[i].Filename = stateField.Filename.ValueString()
			}
			break
		}
	}
}

func templateFieldCandidates(template *server.SecretTemplate, field SecretField) []server.SecretTemplateField {
	if template == nil {
		return nil
	}
	matches := make([]server.SecretTemplateField, 0, 1)
	for _, candidate := range template.Fields {
		if fieldNameMatches(field.FieldName.ValueString(), candidate.Name, candidate.FieldSlugName) {
			matches = append(matches, candidate)
		}
	}
	if len(matches) != 0 {
		return matches
	}
	if slug := field.Slug.ValueString(); slug != "" {
		for _, candidate := range template.Fields {
			if fieldNameMatches(slug, candidate.Name, candidate.FieldSlugName) {
				matches = append(matches, candidate)
			}
		}
	}
	return matches
}

func resolveTemplateField(template *server.SecretTemplate, field SecretField) (server.SecretTemplateField, bool) {
	matches := templateFieldCandidates(template, field)
	if len(matches) != 1 {
		return server.SecretTemplateField{}, false
	}
	return matches[0], true
}

func sameTemplateField(template *server.SecretTemplate, a, b SecretField) bool {
	aTemplateField, aOK := resolveTemplateField(template, a)
	bTemplateField, bOK := resolveTemplateField(template, b)
	if aOK || bOK {
		return aOK && bOK && aTemplateField.SecretTemplateFieldID != 0 && aTemplateField.SecretTemplateFieldID == bTemplateField.SecretTemplateFieldID
	}
	return terraformFieldsMatch(a, b)
}

func (r *TSSSecretResource) getSecretTemplate(ctx context.Context, state *SecretResourceState, client *server.Server) (*server.SecretTemplate, error) {
	templateID, err := stringToInt(state.SecretTemplateID)
	if err != nil {
		return nil, fmt.Errorf("invalid Template ID: %w", err)
	}
	template, err := client.SecretTemplateContext(ctx, templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve secret template: %w", err)
	}
	return template, nil
}

func (r *TSSSecretResource) getSecretData(ctx context.Context, state *SecretResourceState, client *server.Server) (*server.Secret, error) {
	template, err := r.getSecretTemplate(ctx, state, client)
	if err != nil {
		return nil, err
	}
	return r.getSecretDataWithTemplate(ctx, state, client, template)
}

func (r *TSSSecretResource) getSecretDataWithTemplate(ctx context.Context, state *SecretResourceState, client *server.Server, template *server.SecretTemplate) (*server.Secret, error) {
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
	secretPolicyID, err := optionalServerInt(state.SecretPolicyID, "secretpolicyid")
	if err != nil {
		return nil, err
	}
	passwordTypeWebScriptID, err := optionalServerInt(state.PasswordTypeWebScriptID, "passwordtypewebscriptid")
	if err != nil {
		return nil, err
	}
	launcherConnectAsSecretID, err := optionalServerInt(state.LauncherConnectAsSecretID, "launcherconnectassecretid")
	if err != nil {
		return nil, err
	}
	checkOutIntervalMinutes, err := optionalServerInt(state.CheckOutIntervalMinutes, "checkoutintervalminutes")
	if err != nil {
		return nil, err
	}

	// Construct the fields dynamically
	var fields []server.SecretField
	resolvedFields := make(map[int]string, len(state.Fields))
	for _, field := range state.Fields {
		fieldName := field.FieldName.ValueString()
		matches := templateFieldCandidates(template, field)
		if len(matches) == 0 {
			return nil, fmt.Errorf("field %q is not defined on template %q (id %d); check the fieldname against the template's field names or slugs", fieldName, template.Name, templateID)
		}
		if len(matches) > 1 {
			return nil, fmt.Errorf("field %q is ambiguous on template %q (id %d): it matches more than one field name or slug; use a unique field name or change the template aliases", fieldName, template.Name, templateID)
		}
		templateField := matches[0]
		if priorName, duplicate := resolvedFields[templateField.SecretTemplateFieldID]; duplicate {
			return nil, fmt.Errorf("fields %q and %q both resolve to template field %q (id %d); configure each template field only once", priorName, fieldName, templateField.Name, templateField.SecretTemplateFieldID)
		}
		resolvedFields[templateField.SecretTemplateFieldID] = fieldName

		hasGenerate := !field.Generate.IsNull() && field.Generate.ValueBool()
		if field.PasswordValue.IsUnknown() {
			return nil, fmt.Errorf("field %q has an unknown password_value; the write-only password must be known before apply", fieldName)
		}
		hasPasswordValue := !field.PasswordValue.IsNull()
		if hasPasswordValue && field.PasswordValue.ValueString() == "" {
			return nil, fmt.Errorf("field %q sets password_value to an empty string; configure a non-empty password or omit password_value", fieldName)
		}
		hasItemValue := !field.ItemValue.IsNull() && field.ItemValue.ValueString() != ""
		hasFilename := !field.Filename.IsNull() && !field.Filename.IsUnknown() && field.Filename.ValueString() != ""

		if hasFilename && !templateField.IsFile {
			return nil, fmt.Errorf("field %q sets filename but is not a file field on template %q; filename is only valid for fields with isFile=true", fieldName, template.Name)
		}

		if hasPasswordValue && !templateField.IsPassword {
			return nil, fmt.Errorf("field %q has password_value set but is not a password field on template %q; password_value is write-only only for fields with isPassword=true", fieldName, template.Name)
		}
		if hasPasswordValue && hasItemValue {
			return nil, fmt.Errorf("field %q has both password_value and itemvalue set; they are mutually exclusive on a password field", fieldName)
		}
		if hasItemValue && templateField.IsPassword {
			// State never holds a password value, so a configured itemvalue on a
			// password field can never match post-apply state and Terraform
			// would fail with "inconsistent result after apply". Reject it up
			// front with the migration path instead.
			return nil, fmt.Errorf("field %q is a password field on template %q, and password fields do not accept itemvalue; set password_value together with password_wo_version (or generate = true) instead", fieldName, template.Name)
		}
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
			// No password write was requested, so omit the field and let TSS
			// preserve any existing server-side value.
			continue
		default:
			itemValue = ""
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

		if templateField.IsFile {
			if !field.Filename.IsNull() && !field.Filename.IsUnknown() {
				secretField.Filename = field.Filename.ValueString()
			}
		}

		fields = append(fields, secretField)
	}

	// Populate the secret object
	secret := &server.Secret{
		Name:                      state.Name.ValueString(),
		FolderID:                  folderID,
		SiteID:                    siteID,
		SecretTemplateID:          templateID,
		Fields:                    fields,
		Active:                    activeValueForWrite(state.Active),
		SecretPolicyID:            secretPolicyID,
		PasswordTypeWebScriptID:   passwordTypeWebScriptID,
		LauncherConnectAsSecretID: launcherConnectAsSecretID,
		CheckOutIntervalMinutes:   checkOutIntervalMinutes,
	}

	// Handle SSH key args if provided - only during create operations
	// (We ensure this is nil during updates in the Update method)
	if state.SshKeyArgs != nil {
		secret.SshKeyArgs = &server.SshKeyArgs{
			GeneratePassphrase: state.SshKeyArgs.GeneratePassphrase.ValueBool(),
			GenerateSshKeys:    state.SshKeyArgs.GenerateSshKeys.ValueBool(),
		}
	}

	// The SDK request type represents these settings with non-pointer scalar
	// fields. On Create an unknown value therefore has the same wire value as
	// false or zero; the refreshed server response supplies the computed state.
	// On Update, fillUnknownSecretSettingsFromState replaces unknowns first.
	if isKnownValue(state.CheckedOut) {
		secret.CheckedOut = state.CheckedOut.ValueBool()
	}
	if isKnownValue(state.CheckOutEnabled) {
		secret.CheckOutEnabled = state.CheckOutEnabled.ValueBool()
	}
	if isKnownValue(state.AutoChangeEnabled) {
		secret.AutoChangeEnabled = state.AutoChangeEnabled.ValueBool()
	}
	if isKnownValue(state.CheckOutChangePasswordEnabled) {
		secret.CheckOutChangePasswordEnabled = state.CheckOutChangePasswordEnabled.ValueBool()
	}
	if isKnownValue(state.DelayIndexing) {
		secret.DelayIndexing = state.DelayIndexing.ValueBool()
	}
	if isKnownValue(state.EnableInheritPermissions) {
		secret.EnableInheritPermissions = state.EnableInheritPermissions.ValueBool()
	}
	if isKnownValue(state.EnableInheritSecretPolicy) {
		secret.EnableInheritSecretPolicy = state.EnableInheritSecretPolicy.ValueBool()
	}
	if isKnownValue(state.ProxyEnabled) {
		secret.ProxyEnabled = state.ProxyEnabled.ValueBool()
	}
	if isKnownValue(state.RequiresComment) {
		secret.RequiresComment = state.RequiresComment.ValueBool()
	}
	if isKnownValue(state.SessionRecordingEnabled) {
		secret.SessionRecordingEnabled = state.SessionRecordingEnabled.ValueBool()
	}
	if isKnownValue(state.WebLauncherRequiresIncognitoMode) {
		secret.WebLauncherRequiresIncognitoMode = state.WebLauncherRequiresIncognitoMode.ValueBool()
	}

	return secret, nil
}

func isKnownValue(value attr.Value) bool {
	return !value.IsNull() && !value.IsUnknown()
}

func optionalServerInt(value types.Int64, field string) (int, error) {
	if !isKnownValue(value) {
		return 0, nil
	}
	return toServerInt(value.ValueInt64(), field)
}

func activeValueForWrite(value types.Bool) bool {
	if value.IsNull() || value.IsUnknown() {
		return true
	}
	return value.ValueBool()
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

	state.SecretPolicyID = types.Int64Value(int64(secret.SecretPolicyID))
	state.PasswordTypeWebScriptID = types.Int64Value(int64(secret.PasswordTypeWebScriptID))
	state.LauncherConnectAsSecretID = types.Int64Value(int64(secret.LauncherConnectAsSecretID))
	state.CheckOutIntervalMinutes = types.Int64Value(int64(secret.CheckOutIntervalMinutes))
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
	parsed, err := strconv.ParseInt(value.ValueString(), 10, 64)
	if err != nil {
		return 0, err
	}
	return toServerInt(parsed, "ID")
}

type filenameValidator struct{}

var preservedUploadFilename = regexp.MustCompile(`[^.]+\.\w+$`)

func (v filenameValidator) Description(context.Context) string {
	return "The filename must end in an extension containing only ASCII letters, digits, or underscores when configured."
}

func (v filenameValidator) MarkdownDescription(context.Context) string {
	return "The filename must end in an extension containing only ASCII letters, digits, or underscores when configured."
}

func (v filenameValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	filename := req.ConfigValue.ValueString()
	if filename == "" || !preservedUploadFilename.MatchString(filename) {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid File Name",
			fmt.Sprintf("The configured filename %q does not end in an extension containing only ASCII letters, digits, or underscores. The Secret Server SDK would store a different name by appending .txt, which Terraform cannot reconcile with the configured value. Supply a filename with a supported extension, or omit filename to use the server default File.txt.", filename),
		)
	}
}

type passwordValueValidator struct{}

func (v passwordValueValidator) Description(context.Context) string {
	return "The write-only password must not be empty when configured."
}

func (v passwordValueValidator) MarkdownDescription(context.Context) string {
	return "The write-only password must not be empty when configured."
}

func (v passwordValueValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() || req.ConfigValue.ValueString() != "" {
		return
	}
	resp.Diagnostics.AddAttributeError(
		req.Path,
		"Invalid Password Value",
		"password_value must be non-empty when configured. Omit password_value when no explicit password should be sent.",
	)
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

	// If user explicitly set a value (including empty string) in the config, respect it
	if !req.ConfigValue.IsNull() {
		resp.PlanValue = req.ConfigValue
		return
	}

	// For creation with potentially computed values
	if req.State.Raw.IsNull() && (req.PlanValue.IsNull() || req.PlanValue.ValueString() == "") {
		// Determine if this value should be computed by SSH key generation
		if shouldComputeSshKeyValue(req) {
			resp.PlanValue = types.StringUnknown()
			return
		}
	}

	// For null values in the plan, convert to empty string for consistency
	if req.PlanValue.IsNull() {
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
	if !req.ConfigValue.IsNull() && req.ConfigValue.ValueString() == "" {
		// User explicitly set an empty string, preserve it
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
