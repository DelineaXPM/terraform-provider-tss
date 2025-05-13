package delinea

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Define the resource implementation.
type PrintSecretResource struct{}

// Define the data model.
type PrintSecretResourceModel struct {
	Secret types.String `tfsdk:"secret"`
}

// Ensure the resource implementation satisfies the interface.
var _ resource.Resource = &PrintSecretResource{}

// New instance.
func NewPrintSecretResource() resource.Resource {
	return &PrintSecretResource{}
}

func (r *PrintSecretResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = "tss_print_secret"
}

func (r *PrintSecretResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Prints a secret value for debug/logging purposes. Do not use in production.",
		Attributes: map[string]schema.Attribute{
			"secret": schema.StringAttribute{
				Required:    true,
				Description: "The secret value to print.",
				WriteOnly:   true,
			},
		},
	}
}

func (r *PrintSecretResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PrintSecretResourceModel

	// Get input config
	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	//For the DEBUG environment, uncomment this line to unit test whether the secret value is being fetched successfully.
	//log.Printf("[DEBUG] Received secret: %s", data.Secret.ValueString())

	// Set state
	diags = resp.State.Set(ctx, data)
	resp.Diagnostics.Append(diags...)
}

func (r *PrintSecretResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// No-op: no external data to refresh
}

func (r *PrintSecretResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddWarning("Update Not Supported", "This resource does not support update.")
}

func (r *PrintSecretResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// No-op: nothing to delete
}
