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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
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

	fmt.Printf("[DEBUG] creating secret with name %s", newSecret.Name)

	// Use the client to create the secret
	createdSecret, err := client.CreateSecret(*newSecret)
	if err != nil {
		resp.Diagnostics.AddError("Secret Creation Error", fmt.Sprintf("Failed to create secret: %s", err))
		return
	}

	fmt.Printf("Secret is Created successfully...!")

	// Refresh state - let Terraform accept the computed values from the server
	newState, readDiags := r.readSecretByID(ctx, createdSecret.ID, client)
	resp.Diagnostics.Append(readDiags...)
	if resp.Diagnostics.HasError() {
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
	_, err = client.UpdateSecret(*updatedSecret)
	if err != nil {
		resp.Diagnostics.AddError("Secret Update Error", fmt.Sprintf("Failed to update secret: %s", err))
		return
	}

	fmt.Printf("Secret is Updated successfully...!")

	//Refresh state
	newState, readDiags := r.readSecretByID(ctx, updatedSecret.ID, client)
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

	// Ensure the client configuration is set
	if r.clientConfig == nil {
		resp.Diagnostics.AddError("Client Error", "The server client is not configured")
		return
	}

	fmt.Printf("[DEBUG] deleting secret with id %d", int(state.ID.ValueInt64()))

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

	fmt.Printf("Secret is Deleted successfully...!")
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
							Optional:    true,
							Computed:    true,
							Description: "The value of the field. For SSH key generation, this will be computed by the server.",
							PlanModifiers: []planmodifier.String{
								sshKeyFieldPlanModifier{},
								stringplanmodifier.UseStateForUnknown(),
							},
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

	fmt.Printf("[DEBUG] getting secret with id %d", int(state.ID.ValueInt64()))

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

func (r *TSSSecretResource) readSecretByID(ctx context.Context, id int, client *server.Server) (*SecretResourceState, diag.Diagnostics) {
	// Create the server client
	client, err := server.New(*r.clientConfig)
	if err != nil {
		return nil, diag.Diagnostics{
			diag.NewErrorDiagnostic("Configuration Error", fmt.Sprintf("Failed to create server client: %s", err)),
		}
	}

	// Retrieve the secret using the provided client
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

		// Handle field values appropriately - all optional fields should accept null or empty values
		var itemValue string

		// All fields can accept null or empty values (they're all optional in Terraform schema)
		if field.ItemValue.IsNull() {
			// For null values, use empty string
			itemValue = ""
			fmt.Printf("[DEBUG] Field with null value detected: %s, using empty string\n", fieldName)
		} else {
			// Otherwise use the actual value
			itemValue = field.ItemValue.ValueString()

			// Log empty strings but keep them as valid values
			if itemValue == "" {
				fmt.Printf("[DEBUG] Field with explicit empty string detected: %s\n", fieldName)
			}
		}

		// Populate the field object
		secretField := server.SecretField{
			FieldDescription: templateField.Description,
			FieldID:          templateField.SecretTemplateFieldID,
			FieldName:        templateField.Name,
			FileAttachmentID: func() int {
				if !field.ItemValue.IsNull() && field.ItemValue.ValueString() != "" {
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
			ItemValue:  itemValue,
			Slug:       templateField.FieldSlugName,
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
		// Handle ItemValue consistently for all fields - all fields can have empty values
		var itemValue types.String

		// All fields should use StringValue even for empty strings
		// This ensures Terraform treats empty strings as valid values rather than null
		itemValue = types.StringValue(f.ItemValue)

		// Add debug logging for empty values
		if f.ItemValue == "" {
			fmt.Printf("[DEBUG] Flatten: Field '%s' has empty value\n", f.FieldName)
		}

		field := SecretField{
			FieldName:        types.StringValue(f.FieldName),
			ItemValue:        itemValue,
			ItemID:           types.Int64Value(int64(f.ItemID)),
			FieldID:          types.Int64Value(int64(f.FieldID)),
			FileAttachmentID: types.Int64Value(int64(f.FileAttachmentID)),
			Slug:             types.StringValue(f.Slug),
			FieldDescription: types.StringValue(f.FieldDescription),
			Filename:         types.StringValue(f.Filename),
			IsFile:           types.BoolValue(f.IsFile),
			IsNotes:          types.BoolValue(f.IsNotes),
			IsPassword:       types.BoolValue(f.IsPassword),
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
