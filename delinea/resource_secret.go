package delinea

import (
	"context"
	"fmt"
	"strconv"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
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
	ID               types.Int64  `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	FolderID         types.String `tfsdk:"folderid"`
	SiteID           types.String `tfsdk:"siteid"`
	SecretTemplateID types.String `tfsdk:"secrettemplateid"`
	Fields           []struct {
		FieldName types.String `tfsdk:"fieldname"`
		ItemValue types.String `tfsdk:"itemvalue"`
	} `tfsdk:"fields"`
	Active                           types.Bool  `tfsdk:"active"`
	Ephemeral                        types.Bool  `tfsdk:"ephemeral"`
	SecretPolicyID                   types.Int64 `tfsdk:"secretpolicyid"`
	PasswordTypeWebScriptID          types.Int64 `tfsdk:"passwordtypewebscriptid"`
	LauncherConnectAsSecretID        types.Int64 `tfsdk:"launcherconnectassecretid"`
	CheckOutIntervalMinutes          types.Int64 `tfsdk:"checkoutintervalminutes"`
	CheckedOut                       types.Bool  `tfsdk:"checkedout"`
	CheckOutEnabled                  types.Bool  `tfsdk:"checkoutenabled"`
	AutoChangeEnabled                types.Bool  `tfsdk:"autochangenabled"`
	CheckOutChangePasswordEnabled    types.Bool  `tfsdk:"checkoutchangepasswordenabled"`
	DelayIndexing                    types.Bool  `tfsdk:"delayindexing"`
	EnableInheritPermissions         types.Bool  `tfsdk:"enableinheritpermissions"`
	EnableInheritSecretPolicy        types.Bool  `tfsdk:"enableinheritsecretpolicy"`
	ProxyEnabled                     types.Bool  `tfsdk:"proxyenabled"`
	RequiresComment                  types.Bool  `tfsdk:"requirescomment"`
	SessionRecordingEnabled          types.Bool  `tfsdk:"sessionrecordingenabled"`
	WebLauncherRequiresIncognitoMode types.Bool  `tfsdk:"weblauncherrequiresincognitomode"`
}

// Metadata provides the resource type name
func (r *TSSSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "tss_resource_secret"
}

// Configure initializes the resource with the provider configuration
func (r *TSSSecretResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	var state SecretResourceState

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

	// Convert string attributes to integers
	folderID, err := stringToInt(state.FolderID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Folder ID", "Folder ID must be a valid integer")
		return
	}
	siteID, err := stringToInt(state.SiteID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Site ID", "Site ID must be a valid integer")
		return
	}
	templateID, err := stringToInt(state.SecretTemplateID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Template ID", "Template ID must be a valid integer")
		return
	}

	// Get the secret data
	newSecret, err := r.getSecretData(ctx, &state, client)
	if err != nil {
		resp.Diagnostics.AddError("Secret Data Error", fmt.Sprintf("Failed to prepare secret data: %s", err))
		return
	}

	// Update the secret object with converted IDs
	newSecret.FolderID = folderID
	newSecret.SiteID = siteID
	newSecret.SecretTemplateID = templateID

	// Use the client to create the secret
	createdSecret, err := client.CreateSecret(*newSecret)
	if err != nil {
		resp.Diagnostics.AddError("Secret Creation Error", fmt.Sprintf("Failed to create secret: %s", err))
		return
	}

	// Set the secret ID in the state
	state.ID = types.Int64Value(int64(createdSecret.ID))

	// Check if the value should be ephemeral
	if state.Ephemeral.ValueBool() {
		resp.Diagnostics.AddWarning("Ephemeral Value", "The secret value is marked as ephemeral and will not be saved in the Terraform state.")
		for i := range state.Fields {
			state.Fields[i].ItemValue = types.StringNull()
		}
	}

	// Set the state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
}

// Update updates the resource
func (r *TSSSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state SecretResourceState

	// Read the plan
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

	// Get the secret data
	updatedSecret, err := r.getSecretData(ctx, &state, client)
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

	// Set the state
	diags = resp.State.Set(ctx, &state)
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
				Description: "The ID of the secret policy.",
			},
			"passwordtypewebscriptid": schema.Int64Attribute{
				Optional:    true,
				Description: "The ID of the password type web script.",
			},
			"launcherconnectassecretid": schema.Int64Attribute{
				Optional:    true,
				Description: "The ID of the launcher connect-as secret.",
			},
			"checkoutintervalminutes": schema.Int64Attribute{
				Optional:    true,
				Description: "The checkout interval in minutes.",
			},
			"active": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether the secret is active.",
			},
			"checkedout": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether the secret is checked out.",
			},
			"checkoutenabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether checkout is enabled for the secret.",
			},
			"autochangenabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether auto-change is enabled for the secret.",
			},

			"id": schema.Int64Attribute{
				Computed:    true,
				Description: "The ID of the secret.",
			},

			"checkoutchangepasswordenabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether checkout change password is enabled.",
			},
			"delayindexing": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether delay indexing is enabled.",
			},
			"enableinheritpermissions": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether inherit permissions is enabled.",
			},
			"enableinheritsecretpolicy": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether inherit secret policy is enabled.",
			},
			"proxyenabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether proxy is enabled.",
			},
			"requirescomment": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether a comment is required.",
			},
			"sessionrecordingenabled": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether session recording is enabled.",
			},
			"weblauncherrequiresincognitomode": schema.BoolAttribute{
				Optional:    true,
				Description: "Whether the web launcher requires incognito mode.",
			},
		},
		Blocks: map[string]schema.Block{
			"fields": schema.ListNestedBlock{
				Description: "Fields of the secret.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"fieldid": schema.Int64Attribute{
							Optional: true,
						},
						"fileattachmentid": schema.Int64Attribute{
							Optional: true,
						},
						"fieldname": schema.StringAttribute{
							Required: true,
						},
						"slug": schema.StringAttribute{
							Optional: true,
						},
						"fielddescription": schema.StringAttribute{
							Optional: true,
						},
						"filename": schema.StringAttribute{
							Optional: true,
						},
						"itemvalue": schema.StringAttribute{
							Required: true,
						},
						"isfile": schema.BoolAttribute{
							Optional: true,
						},
						"isnotes": schema.BoolAttribute{
							Optional: true,
						},
						"ispassword": schema.BoolAttribute{
							Optional: true,
						},
						"islist": schema.BoolAttribute{
							Optional: true,
						},
						"listtype": schema.StringAttribute{
							Optional: true,
						},
					},
				},
			},
			"sshkeyargs": schema.SetNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"generatepassphrase": schema.BoolAttribute{
							Required: true,
						},
						"generatesshkey": schema.BoolAttribute{
							Required: true,
						},
					},
				},
				Description: "SSH key arguments for the secret",
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
	secret, err := client.Secret(int(state.ID.ValueInt64()))
	if err != nil {
		resp.Diagnostics.AddError("Secret Retrieval Error", fmt.Sprintf("Failed to retrieve secret: %s", err))
		return
	}

	// Update the state with the retrieved secret
	state.Name = types.StringValue(secret.Name)
	state.FolderID = types.StringValue(strconv.Itoa(secret.FolderID))                 // Convert int to string
	state.SiteID = types.StringValue(strconv.Itoa(secret.SiteID))                     // Convert int to string
	state.SecretTemplateID = types.StringValue(strconv.Itoa(secret.SecretTemplateID)) // Convert int to string
	state.Active = types.BoolValue(secret.Active)

	// Set the state
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
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
	for key, value := range state.Fields {
		fields = append(fields, server.SecretField{
			FieldName: strconv.Itoa(key),
			ItemValue: value.ItemValue.ValueString(),
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

// Helper function to convert string to int
func stringToInt(value types.String) (int, error) {
	if value.IsNull() {
		return 0, nil
	}
	return strconv.Atoi(value.ValueString())
}
