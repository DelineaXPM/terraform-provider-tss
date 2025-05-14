package delinea

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/DelineaXPM/tss-sdk-go/v2/server"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TSSSecretResource defines the resource implementation
type TSSSecretsEphemeralResource struct {
	clientConfig *server.Configuration // Store the provider configuration
}

func (r *TSSSecretsEphemeralResource) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = "tss_secrets"
}

// Define the model for your resource state
type TSSSecretsEphemeralResourceModel struct {
	IDs     []types.Int64 `tfsdk:"ids"`
	Field   types.String  `tfsdk:"field"`
	Secrets []SecretModel `tfsdk:"secrets"`
}

type SecretModel struct {
	ID    types.Int64  `tfsdk:"id"`
	Value types.String `tfsdk:"value"`
}

// Define private data structure (optional)
type TSSSecretsPrivateData struct {
	IDs     []types.Int64 `tfsdk:"ids"`
	Field   string        `json:"field"`
	Secrets []SecretModel `tfsdk:"secrets"`
}

func (r *TSSSecretsEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"ids": schema.ListAttribute{
				ElementType: types.Int64Type,
				Required:    true,
				Description: "A list of IDs of the secrets",
			},
			"field": schema.StringAttribute{
				Required:    true,
				Description: "The field to extract from the secrets",
			},
			"secrets": schema.ListNestedAttribute{
				Computed:    true,
				Description: "A list of secrets with their field values",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.Int64Attribute{
							Computed:    true,
							Description: "The ID of the secret",
						},
						"value": schema.StringAttribute{
							Computed:    true,
							Description: "The ephemeral value of the field of the secret",
						},
					},
				},
			},
		},
	}
}

func (r *TSSSecretsEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	// Create a model to hold the input configuration
	var data TSSSecretsEphemeralResourceModel

	// Read the Terraform config data into the model
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if r.clientConfig == nil {
		resp.Diagnostics.AddError("Provider not configured", "Cannot fetch secrets because the provider is not configured.")
		return
	}

	// Check for required fields in the model (secret_ids and field)
	if data.IDs == nil || len(data.IDs) == 0 || data.Field.IsNull() {
		resp.Diagnostics.AddError("Missing Required Field", "Both secret_ids and field are required")
		return
	}

	// Initialize your Delinea API client (e.g., using the secret_id and field)
	client, err := server.New(*r.clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Client Creation Error", err.Error())
		return
	}

	// Fetch secrets
	var results []SecretModel

	for _, id := range data.IDs {
		secretID := int(id.ValueInt64())

		log.Printf("[DEBUG] getting secret with id %d", secretID)

		// Fetch the secret
		secret, err := client.Secret(secretID)
		if err != nil {
			resp.Diagnostics.AddWarning("Secret Fetch Warning", fmt.Sprintf("Failed to fetch secret with ID %d: %s", secretID, err))
			continue // Skip this ID and continue with the rest
		}

		log.Printf("[DEBUG] using '%s' field of secret with id %d", data.Field.ValueString(), secretID)

		// Extract the requested field value (assuming Field() method is available)
		fieldValue, ok := secret.Field(data.Field.ValueString())
		if !ok {
			resp.Diagnostics.AddError("Field Not Found", fmt.Sprintf("Field %s not found in the secret", data.Field.ValueString()))
			continue
		}

		// Save the secret value in the state
		results = append(results, struct {
			ID    types.Int64  `tfsdk:"id"`
			Value types.String `tfsdk:"value"`
		}{
			ID:    types.Int64Value(int64(secretID)),
			Value: types.StringValue(fieldValue),
		})
	}

	// Set the secret value in the result
	data.Secrets = results

	// Save the data into the ephemeral result state
	resp.Diagnostics.Append(resp.Result.Set(ctx, &data)...)

	// Set a renewal time for the resource
	resp.RenewAt = time.Now().Add(5 * time.Minute)

	// Store private data for use during renewal
	privateData, _ := json.Marshal(TSSSecretsPrivateData{
		IDs:     data.IDs,
		Field:   data.Field.ValueString(),
		Secrets: data.Secrets,
	})
	resp.Private.SetKey(ctx, "tss_secrets_data", privateData)
}

func (r *TSSSecretsEphemeralResource) Renew(ctx context.Context, req ephemeral.RenewRequest, resp *ephemeral.RenewResponse) {
	// Retrieve the private data that was stored during Open
	privateBytes, _ := req.Private.GetKey(ctx, "tss_secrets_data")
	if privateBytes == nil {
		resp.Diagnostics.AddError("Missing Private Data", "Private data was not found for renewal.")
		return
	}

	// Unmarshal private data
	var privateData TSSSecretsPrivateData
	if err := json.Unmarshal(privateBytes, &privateData); err != nil {
		resp.Diagnostics.AddError("Invalid Private Data", "Failed to unmarshal private data.")
		return
	}

	// Ensure that secret_id and field are available in the private data
	if privateData.IDs == nil || len(privateData.IDs) == 0 || privateData.Field == "" {
		resp.Diagnostics.AddError("Missing Private Data Fields", "Secret ID and field are required.")
		return
	}

	// Initialize your Delinea API client
	client, err := server.New(*r.clientConfig)
	if err != nil {
		resp.Diagnostics.AddError("Client Creation Error", err.Error())
		return
	}

	// Fetch secrets
	var results []SecretModel

	for _, id := range privateData.IDs {
		secretID := int(id.ValueInt64())

		log.Printf("[DEBUG] getting secret with id %d to renew data", secretID)

		// Fetch the secret
		secret, err := client.Secret(secretID)
		if err != nil {
			resp.Diagnostics.AddWarning("Secret Fetch Warning", fmt.Sprintf("Failed to fetch secret with ID %d: %s", secretID, err))
			continue // Skip this ID and continue with the rest
		}

		log.Printf("[DEBUG] using '%s' field of secret with id %d to renew data", privateData.Field, secretID)

		// Extract the requested field value (assuming Field() method is available)
		fieldValue, ok := secret.Field(privateData.Field)
		if !ok {
			resp.Diagnostics.AddError("Field Not Found", fmt.Sprintf("Field %s not found in the secret", privateData.Field))
			continue
		}

		// Save the secret value in the state
		results = append(results, struct {
			ID    types.Int64  `tfsdk:"id"`
			Value types.String `tfsdk:"value"`
		}{
			ID:    types.Int64Value(int64(secretID)),
			Value: types.StringValue(fieldValue),
		})
	}

	// Update the private data with the new secret value
	privateData.Secrets = results

	// Store the updated private data for the next renewal
	privateDataBytes, _ := json.Marshal(privateData)
	resp.Private.SetKey(ctx, "tss_secrets_data", privateDataBytes)

	// Set the renewal time (e.g., 5 minutes from now)
	resp.RenewAt = time.Now().Add(5 * time.Minute)
}

func (r *TSSSecretsEphemeralResource) Close(ctx context.Context, req ephemeral.CloseRequest, resp *ephemeral.CloseResponse) {

}

func (r *TSSSecretsEphemeralResource) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
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
