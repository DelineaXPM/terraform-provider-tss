package delinea

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretResource defines the resource implementation
type TSSSecretResource struct {
	clientConfig *server.Configuration // Store the provider configuration
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
	FieldName        types.String `tfsdk:"fieldname"`
	ItemValue        types.String `tfsdk:"itemvalue"`
	ItemID           types.Int64  `tfsdk:"itemid"`
	FieldID          types.Int64  `tfsdk:"fieldid"`
	FileAttachmentID types.Int64  `tfsdk:"fileattachmentid"`
	Slug             types.String `tfsdk:"slug"`
	FieldDescription types.String `tfsdk:"fielddescription"`
	Filename         types.String `tfsdk:"filename"`
	IsFile           types.Bool   `tfsdk:"isfile"`
	IsNotes          types.Bool   `tfsdk:"isnotes"`
	IsPassword       types.Bool   `tfsdk:"ispassword"`
	IsList           types.Bool   `tfsdk:"islist"`
	ListType         types.String `tfsdk:"listtype"`
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

// Create creates the resource

// Create creates the resource
func (r *TSSSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SecretResourceState

	// Read the configuration
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

	// Get the secret data
	newSecret, err := r.getSecretData(ctx, &plan, client)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", fmt.Sprintf("Failed to prepare secret data: %s", err))
		return
	}

	// Use the client to create the secret
	createdSecret, err := client.CreateSecret(*newSecret)
	if err != nil {
		resp.Diagnostics.AddError("Secret Creation Error", fmt.Sprintf("Failed to create secret: %s", err))
		return
	}

	//Refresh state
	newState, readDiags := r.readSecretByID(ctx, createdSecret.ID, client)
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set the state
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource
func (r *TSSSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan SecretResourceState
	var state SecretResourceState

	// Read the plan
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	diags = req.State.Get(ctx, &state)
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

	// Get the secret data
	updatedSecret, err := r.getSecretData(ctx, &plan, client)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", fmt.Sprintf("Failed to prepare secret data: %s", err))
		return
	}

	// Update the secret
	updatedSecret.ID = int(state.ID.ValueInt64())
	_, err = client.UpdateSecret(*updatedSecret)
	if err != nil {
		resp.Diagnostics.AddError("Secret Update Error", fmt.Sprintf("Failed to update secret: %s", err))
		return
	}

	//Refresh state
	newState, readDiags := r.readSecretByID(ctx, updatedSecret.ID, client)
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
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

	// Delete the secret
	err = client.DeleteSecret(int(state.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Secret Deletion Error", fmt.Sprintf("Failed to delete secret: %s", err))
		return
	}
}

// Schema defines the schema for the resource
func (r *TSSSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
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
							Optional: true,
						},
						"itemid": schema.Int64Attribute{
							Optional: true,
							Computed: true,
						},
						"fieldid": schema.Int64Attribute{
							Optional: true,
							Computed: true,
						},
						"fileattachmentid": schema.Int64Attribute{
							Optional: true,
							Computed: true,
						},
						"slug": schema.StringAttribute{
							Optional: true,
							Computed: true,
						},
						"fielddescription": schema.StringAttribute{
							Optional: true,
							Computed: true,
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

	// Retrieve the secret
	newState, readDiags := r.readSecretByID(ctx, int(state.ID.ValueInt64()), client)
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set the state
	diags = resp.State.Set(ctx, newState)
	resp.Diagnostics.Append(diags...)
}

func (r *TSSSecretResource) readSecretByID(ctx context.Context, id int, client *server.Server) (*SecretResourceState, diag.Diagnostics) {
	// Create the server client
	client, err := server.New(*r.clientConfig)
	if err != nil {
		return nil, diag.Diagnostics{
			diag.NewErrorDiagnostic("Configuration Error", fmt.Sprintf("Failed to create server client: %s", err)),
		}
	}

	// Retrieve the secret
	secret, err := client.Secret(id)
	if err != nil {
		return nil, diag.Diagnostics{
			diag.NewErrorDiagnostic("Secret Retrieval Error", fmt.Sprintf("Failed to retrieve secret: %s", err)),
		}
	}

	state, err := flattenSecret(secret)
	if err != nil {
		return nil, diag.Diagnostics{
			diag.NewErrorDiagnostic("State Error", fmt.Sprintf("Failed to flatten secret: %s", err)),
		}
	}

	return state, nil
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
	template, err := client.SecretTemplate(templateID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve secret template: %w", err)
	}

	// Construct the fields dynamically
	var fields []server.SecretField
	for _, field := range state.Fields {
		templateField := server.SecretTemplateField{}
		fieldName := field.FieldName.ValueString()

		// Match the field name with the template fields
		for _, record := range template.Fields {
			if strings.EqualFold(record.Name, fieldName) || strings.EqualFold(record.FieldSlugName, fieldName) {
				templateField = record
				break
			}
		}

		// Populate the field object
		fields = append(fields, server.SecretField{
			FieldDescription: templateField.Description,
			FieldID:          templateField.SecretTemplateFieldID,
			FieldName:        templateField.Name,
			FileAttachmentID: func() int {
				if !field.ItemValue.IsNull() {
					value, err := strconv.Atoi(field.ItemValue.ValueString())
					if err == nil {
						return value
					}
				}
				return 0
			}(),
			IsFile:     templateField.IsFile,
			IsNotes:    templateField.IsNotes,
			IsPassword: templateField.IsPassword,
			ItemValue:  field.ItemValue.ValueString(),
			Slug:       templateField.FieldSlugName,
		})
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
		fields = append(fields, SecretField{
			FieldName:        types.StringValue(f.FieldName),
			ItemValue:        types.StringValue(f.ItemValue),
			ItemID:           types.Int64Value(int64(f.ItemID)),
			FieldID:          types.Int64Value(int64(f.FieldID)),
			FileAttachmentID: types.Int64Value(int64(f.FileAttachmentID)),
			Slug:             types.StringValue(f.Slug),
			FieldDescription: types.StringValue(f.FieldDescription),
			Filename:         types.StringValue(f.Filename),
			IsFile:           types.BoolValue(f.IsFile),
			IsNotes:          types.BoolValue(f.IsNotes),
			IsPassword:       types.BoolValue(f.IsPassword),
		})
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
