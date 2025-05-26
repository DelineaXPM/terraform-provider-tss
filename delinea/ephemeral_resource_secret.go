package delinea

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretResource defines the resource implementation
type TSSSecretEphemeralResource struct {
	clientConfig *server.Configuration // Store the provider configuration
}

func (r *TSSSecretEphemeralResource) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = "tss_secret"
}

// Define the model for your resource state
type TSSSecretEphemeralResourceModel struct {
	SecretID    types.String `tfsdk:"id"`
	Field       types.String `tfsdk:"field"`
	SecretValue types.String `tfsdk:"value"`
}

// Define private data structure (optional)
type TSSSecretPrivateData struct {
	SecretID    string `json:"id"`
	Field       string `json:"field"`
	SecretValue string `json:"value"`
}

func (r *TSSSecretEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Required:    true,
				Description: "The ID of the secret to retrieve.",
			},
			"field": schema.StringAttribute{
				Required:    true,
				Description: "The field to extract from the secret.",
			},
			"value": schema.StringAttribute{
				Computed:    true,
				Description: "The value of the requested field from the secret.",
			},
		},
	}
}

func (r *TSSSecretEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	// Create a model to hold the input configuration
	var data TSSSecretEphemeralResourceModel

	// Read the Terraform config data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.clientConfig == nil {
		resp.Diagnostics.AddError("Provider not configured", "Cannot fetch secrets because the provider is not configured.")
		return
	}

	// Check for required fields in the model (secret_id and field)
	if data.SecretID.IsNull() || data.Field.IsNull() {
		resp.Diagnostics.AddError("Missing Required Field", "Both secret_id and field are required")
		return
	}

	// Initialize your Delinea API client (e.g., using the secret_id and field)
	client, err := server.New(*r.clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Client Creation Error", err.Error())
		return
	}

	// Convert SecretID to integer
	secretID, err := strconv.Atoi(data.SecretID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", "Secret ID must be an integer")
		return
	}

	log.Printf("[DEBUG] getting secret with id %d", secretID)

	// Fetch the secret from the server using Delinea SDK
	secret, err := client.Secret(secretID)
	if err != nil {
		resp.Diagnostics.AddError("Secret Fetch Error", err.Error())
		return
	}

	log.Printf("[DEBUG] using '%s' field of secret with id %d", data.Field.ValueString(), secretID)

	// Extract the requested field value (assuming Field() method is available)
	fieldValue, ok := secret.Field(data.Field.ValueString())
	if !ok {
		resp.Diagnostics.AddError("Field Not Found", fmt.Sprintf("Field %s not found in the secret", data.Field.ValueString()))
		return
	}

	// Set the secret value in the result
	data.SecretValue = types.StringValue(fieldValue)

	// Save the data into the ephemeral result state
	resp.Diagnostics.Append(resp.Result.Set(ctx, &data)...)

	// Set a renewal time for the resource
	resp.RenewAt = time.Now().Add(5 * time.Minute)

	// Store private data for use during renewal
	privateData, _ := json.Marshal(TSSSecretPrivateData{
		SecretID:    data.SecretID.ValueString(),
		Field:       data.Field.ValueString(),
		SecretValue: data.SecretValue.ValueString(),
	})
	resp.Private.SetKey(ctx, "tss_secret_data", privateData)
}

func (r *TSSSecretEphemeralResource) Renew(ctx context.Context, req ephemeral.RenewRequest, resp *ephemeral.RenewResponse) {
	// Retrieve the private data that was stored during Open
	privateBytes, _ := req.Private.GetKey(ctx, "tss_secret_data")
	if privateBytes == nil {
		resp.Diagnostics.AddError("Missing Private Data", "Private data was not found for renewal.")
		return
	}

	// Unmarshal private data
	var privateData TSSSecretPrivateData
	if err := json.Unmarshal(privateBytes, &privateData); err != nil {
		resp.Diagnostics.AddError("Invalid Private Data", "Failed to unmarshal private data.")
		return
	}

	// Ensure that secret_id and field are available in the private data
	if privateData.SecretID == "" || privateData.Field == "" {
		resp.Diagnostics.AddError("Missing Private Data Fields", "Secret ID and field are required.")
		return
	}

	// Initialize your Delinea API client
	client, err := server.New(*r.clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Client Creation Error", err.Error())
		return
	}

	// Convert SecretID to integer
	secretID, err := strconv.Atoi(privateData.SecretID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Secret ID", "Secret ID must be an integer.")
		return
	}

	log.Printf("[DEBUG] getting secret with id %d to renew data", secretID)

	// Fetch the secret from the server
	secret, err := client.Secret(secretID)
	if err != nil {
		resp.Diagnostics.AddError("Secret Fetch Error", err.Error())
		return
	}

	log.Printf("[DEBUG] using '%s' field of secret with id %d to renew data", privateData.Field, secretID)

	// Extract the requested field value
	fieldValue, ok := secret.Field(privateData.Field)
	if !ok {
		resp.Diagnostics.AddError("Field Not Found", fmt.Sprintf("Field %s not found in the secret", privateData.Field))
		return
	}

	// Update the private data with the new secret value
	privateData.SecretValue = fieldValue

	// Store the updated private data for the next renewal
	privateDataBytes, _ := json.Marshal(privateData)
	resp.Private.SetKey(ctx, "tss_secret_data", privateDataBytes)

	// Set the renewal time (e.g., 5 minutes from now)
	resp.RenewAt = time.Now().Add(5 * time.Minute)
}

func (r *TSSSecretEphemeralResource) Close(ctx context.Context, req ephemeral.CloseRequest, resp *ephemeral.CloseResponse) {

}

func (r *TSSSecretEphemeralResource) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	log.Printf("DEBUG: ProviderData received in Configure: %+v\n", req.ProviderData)
	client, ok := req.ProviderData.(*server.Configuration)
	if !ok {
		resp.Diagnostics.AddError("Invalid Provider Data", "Expected *ExampleClient")
		return
	}

	log.Printf("DEBUG: Successfully retrieved provider configuration: %+v\n", client)

	r.clientConfig = client
}
