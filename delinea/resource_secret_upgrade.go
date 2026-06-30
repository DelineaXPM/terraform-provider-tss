package delinea

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// State versioning history for tss_resource_secret:
//
//   - Schema version 0: provider versions v3.x. Same struct shape as today
//     except no password_value, no password_wo_version, no generate;
//     itemvalue not Sensitive; itemid/fieldid/slug/fielddescription
//     declared Optional+Computed (rather than Computed-only).
//
//   - Schema version 1: provider version v4.0.0. Adds password_value
//     (WriteOnly), password_wo_version (Int64), and generate (Bool) to
//     the fields block; flips itemvalue to Sensitive; locks
//     itemid/fieldid/slug/fielddescription to Computed-only.
//
// The version 0 -> 1 upgrade is purely additive on the state side: every
// version-0 attribute exists in version 1 with the same underlying type,
// and the new attributes default to null. We read the prior state with
// the version-0 schema, copy values across into the v4 struct, and let
// the new attributes stay null.
//
// Provider v2.x state (terraform-plugin-sdk v2 era) is not handled here.
// Its on-disk structure is incompatible enough that an in-place upgrade
// would be a separate, larger piece of work. v2.x users should upgrade
// to v3.x first via the recommended manual path documented in the
// release notes.

func (r *TSSSecretResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: priorSchemaV0(),
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var prior secretResourceStateV0
				resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
				if resp.Diagnostics.HasError() {
					return
				}

				upgraded := SecretResourceState{
					ID:                               prior.ID,
					Name:                             prior.Name,
					FolderID:                         prior.FolderID,
					SiteID:                           prior.SiteID,
					SecretTemplateID:                 prior.SecretTemplateID,
					SshKeyArgs:                       prior.SshKeyArgs,
					Active:                           prior.Active,
					SecretPolicyID:                   prior.SecretPolicyID,
					PasswordTypeWebScriptID:          prior.PasswordTypeWebScriptID,
					LauncherConnectAsSecretID:        prior.LauncherConnectAsSecretID,
					CheckOutIntervalMinutes:          prior.CheckOutIntervalMinutes,
					CheckedOut:                       prior.CheckedOut,
					CheckOutEnabled:                  prior.CheckOutEnabled,
					AutoChangeEnabled:                prior.AutoChangeEnabled,
					CheckOutChangePasswordEnabled:    prior.CheckOutChangePasswordEnabled,
					DelayIndexing:                    prior.DelayIndexing,
					EnableInheritPermissions:         prior.EnableInheritPermissions,
					EnableInheritSecretPolicy:        prior.EnableInheritSecretPolicy,
					ProxyEnabled:                     prior.ProxyEnabled,
					RequiresComment:                  prior.RequiresComment,
					SessionRecordingEnabled:          prior.SessionRecordingEnabled,
					WebLauncherRequiresIncognitoMode: prior.WebLauncherRequiresIncognitoMode,
				}
				upgraded.Fields = upgradeFieldsV0ToV1(prior.Fields)

				resp.Diagnostics.Append(resp.State.Set(ctx, &upgraded)...)
			},
		},
	}
}

// secretFieldV0 mirrors the v3.x SecretField struct. v4 adds three
// attributes (password_value, password_wo_version, generate); existing
// attributes keep the same Go types and tfsdk tags.
type secretFieldV0 struct {
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

type secretResourceStateV0 struct {
	ID                               types.Int64     `tfsdk:"id"`
	Name                             types.String    `tfsdk:"name"`
	FolderID                         types.String    `tfsdk:"folderid"`
	SiteID                           types.String    `tfsdk:"siteid"`
	SecretTemplateID                 types.String    `tfsdk:"secrettemplateid"`
	Fields                           []secretFieldV0 `tfsdk:"fields"`
	SshKeyArgs                       *SshKeyArgs     `tfsdk:"sshkeyargs"`
	Active                           types.Bool      `tfsdk:"active"`
	SecretPolicyID                   types.Int64     `tfsdk:"secretpolicyid"`
	PasswordTypeWebScriptID          types.Int64     `tfsdk:"passwordtypewebscriptid"`
	LauncherConnectAsSecretID        types.Int64     `tfsdk:"launcherconnectassecretid"`
	CheckOutIntervalMinutes          types.Int64     `tfsdk:"checkoutintervalminutes"`
	CheckedOut                       types.Bool      `tfsdk:"checkedout"`
	CheckOutEnabled                  types.Bool      `tfsdk:"checkoutenabled"`
	AutoChangeEnabled                types.Bool      `tfsdk:"autochangenabled"`
	CheckOutChangePasswordEnabled    types.Bool      `tfsdk:"checkoutchangepasswordenabled"`
	DelayIndexing                    types.Bool      `tfsdk:"delayindexing"`
	EnableInheritPermissions         types.Bool      `tfsdk:"enableinheritpermissions"`
	EnableInheritSecretPolicy        types.Bool      `tfsdk:"enableinheritsecretpolicy"`
	ProxyEnabled                     types.Bool      `tfsdk:"proxyenabled"`
	RequiresComment                  types.Bool      `tfsdk:"requirescomment"`
	SessionRecordingEnabled          types.Bool      `tfsdk:"sessionrecordingenabled"`
	WebLauncherRequiresIncognitoMode types.Bool      `tfsdk:"weblauncherrequiresincognitomode"`
}

// upgradeFieldsV0ToV1 copies every v0 field into a fresh v1 SecretField.
// password_value, password_wo_version, and generate stay at their zero
// values (null); they were not present in v0 state.
func upgradeFieldsV0ToV1(fields []secretFieldV0) []SecretField {
	if fields == nil {
		return nil
	}
	out := make([]SecretField, len(fields))
	for i, f := range fields {
		out[i] = SecretField{
			FieldName:        f.FieldName,
			ItemValue:        f.ItemValue,
			ItemID:           f.ItemID,
			FieldID:          f.FieldID,
			FileAttachmentID: f.FileAttachmentID,
			Slug:             f.Slug,
			FieldDescription: f.FieldDescription,
			Filename:         f.Filename,
			IsFile:           f.IsFile,
			IsNotes:          f.IsNotes,
			IsPassword:       f.IsPassword,
			IsList:           f.IsList,
			ListType:         f.ListType,
		}
	}
	return out
}

// priorSchemaV0 mirrors the v3.x schema for tss_resource_secret. It
// describes only what the framework needs to read v3.x state — no plan
// modifiers, validators, or markdown descriptions.
func priorSchemaV0() *schema.Schema {
	return &schema.Schema{
		Attributes: map[string]schema.Attribute{
			"name":                             schema.StringAttribute{Required: true},
			"folderid":                         schema.StringAttribute{Required: true},
			"siteid":                           schema.StringAttribute{Required: true},
			"secrettemplateid":                 schema.StringAttribute{Required: true},
			"id":                               schema.Int64Attribute{Computed: true},
			"active":                           schema.BoolAttribute{Optional: true, Computed: true},
			"secretpolicyid":                   schema.Int64Attribute{Optional: true, Computed: true},
			"passwordtypewebscriptid":          schema.Int64Attribute{Optional: true, Computed: true},
			"launcherconnectassecretid":        schema.Int64Attribute{Optional: true, Computed: true},
			"checkoutintervalminutes":          schema.Int64Attribute{Optional: true, Computed: true},
			"checkedout":                       schema.BoolAttribute{Optional: true, Computed: true},
			"checkoutenabled":                  schema.BoolAttribute{Optional: true, Computed: true},
			"autochangenabled":                 schema.BoolAttribute{Optional: true, Computed: true},
			"checkoutchangepasswordenabled":    schema.BoolAttribute{Optional: true, Computed: true},
			"delayindexing":                    schema.BoolAttribute{Optional: true, Computed: true},
			"enableinheritpermissions":         schema.BoolAttribute{Optional: true, Computed: true},
			"enableinheritsecretpolicy":        schema.BoolAttribute{Optional: true, Computed: true},
			"proxyenabled":                     schema.BoolAttribute{Optional: true, Computed: true},
			"requirescomment":                  schema.BoolAttribute{Optional: true, Computed: true},
			"sessionrecordingenabled":          schema.BoolAttribute{Optional: true, Computed: true},
			"weblauncherrequiresincognitomode": schema.BoolAttribute{Optional: true, Computed: true},
		},
		Blocks: map[string]schema.Block{
			"fields": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"fieldname":        schema.StringAttribute{Required: true},
						"itemvalue":        schema.StringAttribute{Optional: true, Computed: true},
						"itemid":           schema.Int64Attribute{Optional: true, Computed: true},
						"fieldid":          schema.Int64Attribute{Optional: true, Computed: true},
						"fileattachmentid": schema.Int64Attribute{Optional: true, Computed: true},
						"slug":             schema.StringAttribute{Optional: true, Computed: true},
						"fielddescription": schema.StringAttribute{Optional: true, Computed: true},
						"filename":         schema.StringAttribute{Optional: true, Computed: true},
						"isfile":           schema.BoolAttribute{Optional: true, Computed: true},
						"isnotes":          schema.BoolAttribute{Optional: true, Computed: true},
						"ispassword":       schema.BoolAttribute{Optional: true, Computed: true},
						"islist":           schema.BoolAttribute{Optional: true, Computed: true},
						"listtype":         schema.StringAttribute{Optional: true, Computed: true},
					},
				},
			},
			"sshkeyargs": schema.SingleNestedBlock{
				Attributes: map[string]schema.Attribute{
					"generatepassphrase": schema.BoolAttribute{Required: true},
					"generatesshkeys":    schema.BoolAttribute{Required: true},
				},
			},
		},
	}
}
