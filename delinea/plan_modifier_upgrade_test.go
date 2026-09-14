package delinea

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func itemValuePlanRequest(t *testing.T, configValue, planValue types.String, hasState bool) planmodifier.StringRequest {
	t.Helper()
	ctx := context.Background()
	response := &resource.SchemaResponse{}
	(&TSSSecretResource{}).Schema(ctx, resource.SchemaRequest{}, response)
	schemaType := response.Schema.Type().TerraformType(ctx)
	state := tfsdk.State{Schema: response.Schema, Raw: tftypes.NewValue(schemaType, nil)}
	if hasState {
		state.Raw = tftypes.NewValue(schemaType, tftypes.UnknownValue)
	}
	return planmodifier.StringRequest{
		Path:        path.Root("fields").AtListIndex(0).AtName("itemvalue"),
		ConfigValue: configValue,
		PlanValue:   planValue,
		State:       state,
	}
}

func TestSshKeyFieldPlanModifier(t *testing.T) {
	cases := map[string]struct {
		config, plan types.String
		hasState     bool
		want         types.String
	}{
		"configured value wins":                    {config: types.StringValue("v"), plan: types.StringValue("v"), want: types.StringValue("v")},
		"configured empty string is kept":          {config: types.StringValue(""), plan: types.StringValue(""), want: types.StringValue("")},
		"create with no config becomes unknown":    {config: types.StringNull(), plan: types.StringUnknown(), want: types.StringUnknown()},
		"create with null plan becomes unknown":    {config: types.StringNull(), plan: types.StringNull(), want: types.StringUnknown()},
		"update with null plan becomes empty":      {config: types.StringNull(), plan: types.StringNull(), hasState: true, want: types.StringValue("")},
		"update keeps planned value":               {config: types.StringNull(), plan: types.StringValue("kept"), hasState: true, want: types.StringValue("kept")},
		"update leaves unknown for UseStateForUnk": {config: types.StringNull(), plan: types.StringUnknown(), hasState: true, want: types.StringUnknown()},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			request := itemValuePlanRequest(t, c.config, c.plan, c.hasState)
			response := &planmodifier.StringResponse{PlanValue: request.PlanValue}
			sshKeyFieldPlanModifier{}.PlanModifyString(context.Background(), request, response)
			if !response.PlanValue.Equal(c.want) {
				t.Fatalf("plan value = %v, want %v", response.PlanValue, c.want)
			}
		})
	}
}

func TestShouldComputeSshKeyValue_IgnoresOtherPaths(t *testing.T) {
	request := itemValuePlanRequest(t, types.StringNull(), types.StringNull(), false)
	request.Path = path.Root("name")
	if shouldComputeSshKeyValue(request) {
		t.Fatal("a top-level attribute must never be marked as SSH-generated")
	}
}

func TestUpgradeState_V0ToV1RoundTripsThroughFramework(t *testing.T) {
	ctx := context.Background()
	upgraders := (&TSSSecretResource{}).UpgradeState(ctx)
	upgrader, ok := upgraders[0]
	if !ok || upgrader.PriorSchema == nil {
		t.Fatalf("no version-0 upgrader with a prior schema: %v", upgraders)
	}

	prior := secretResourceStateV0{
		ID:               types.Int64Value(42),
		Name:             types.StringValue("legacy"),
		FolderID:         types.StringValue("7"),
		SiteID:           types.StringValue("1"),
		SecretTemplateID: types.StringValue("2"),
		Active:           types.BoolValue(true),
		CheckOutEnabled:  types.BoolValue(true),
		SshKeyArgs:       &SshKeyArgs{GeneratePassphrase: types.BoolValue(true), GenerateSshKeys: types.BoolValue(true)},
		Fields: []secretFieldV0{
			{FieldName: types.StringValue("Password"), ItemValue: types.StringValue("legacy"), ItemID: types.Int64Value(5), FieldID: types.Int64Value(7), Slug: types.StringValue("password"), IsPassword: types.BoolValue(true)},
		},
	}
	priorState := tfsdk.State{Schema: *upgrader.PriorSchema, Raw: tftypes.NewValue(upgrader.PriorSchema.Type().TerraformType(ctx), nil)}
	if diags := priorState.Set(ctx, &prior); diags.HasError() {
		t.Fatalf("building v0 state through the prior schema: %v", diags)
	}

	current := &resource.SchemaResponse{}
	(&TSSSecretResource{}).Schema(ctx, resource.SchemaRequest{}, current)
	response := &resource.UpgradeStateResponse{State: tfsdk.State{Schema: current.Schema, Raw: tftypes.NewValue(current.Schema.Type().TerraformType(ctx), nil)}}
	upgrader.StateUpgrader(ctx, resource.UpgradeStateRequest{State: &priorState}, response)
	if response.Diagnostics.HasError() {
		t.Fatalf("upgrader diagnostics: %v", response.Diagnostics)
	}

	var upgraded SecretResourceState
	if diags := response.State.Get(ctx, &upgraded); diags.HasError() {
		t.Fatalf("reading upgraded state with the current schema: %v", diags)
	}
	if upgraded.ID.ValueInt64() != 42 || upgraded.Name.ValueString() != "legacy" || !upgraded.CheckOutEnabled.ValueBool() {
		t.Fatalf("top-level attributes not carried across: %+v", upgraded)
	}
	if upgraded.SshKeyArgs == nil || !upgraded.SshKeyArgs.GenerateSshKeys.ValueBool() {
		t.Fatalf("sshkeyargs not carried across: %+v", upgraded.SshKeyArgs)
	}
	if len(upgraded.Fields) != 1 {
		t.Fatalf("fields = %+v, want one", upgraded.Fields)
	}
	field := upgraded.Fields[0]
	if field.FieldName.ValueString() != "Password" || field.ItemValue.ValueString() != "legacy" || field.ItemID.ValueInt64() != 5 {
		t.Fatalf("field values not carried across: %+v", field)
	}
	if !field.PasswordValue.IsNull() || !field.PasswordWoVersion.IsNull() || !field.Generate.IsNull() {
		t.Fatalf("new v1 attributes must start null: %+v", field)
	}
}

func TestClientFromProviderData(t *testing.T) {
	var diags diag.Diagnostics
	if client := clientFromProviderData(nil, &diags); client != nil || diags.HasError() {
		t.Fatalf("nil provider data: client=%v diags=%v", client, diags)
	}
	if client := clientFromProviderData("wrong type", &diags); client != nil || diags.ErrorsCount() != 1 || !strings.Contains(diags[0].Detail(), "string") {
		t.Fatalf("wrong provider data type: client=%v diags=%v", client, diags)
	}
	diags = nil
	if requireClient(nil, &diags) || diags.ErrorsCount() != 1 || diags[0].Summary() != "Provider Not Configured" {
		t.Fatalf("requireClient(nil) diags = %v", diags)
	}
}

func TestReadSecretByID_AmbiguousBadRequestKeepsStateWithHint(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/secrets/9" {
			return false
		}
		respondAccessDenied(w)
		return true
	})
	state, diags := (&TSSSecretResource{}).readSecretByID(context.Background(), 9, client, nil)
	if state != nil || diags.ErrorsCount() != 1 || diags[0].Summary() != "Secret Retrieval Error" ||
		!strings.Contains(diags[0].Detail(), "terraform state rm") || !strings.Contains(diags[0].Detail(), "deleted outside Terraform") {
		t.Fatalf("state=%v diags=%v", state, diags)
	}
}

func TestFillUnknownSecretSettingsFromState(t *testing.T) {
	state := SecretResourceState{
		Active:                    types.BoolValue(true),
		CheckOutEnabled:           types.BoolValue(true),
		RequiresComment:           types.BoolValue(false),
		LauncherConnectAsSecretID: types.Int64Value(55),
		SecretPolicyID:            types.Int64Null(),
	}
	plan := SecretResourceState{
		Active:                    types.BoolUnknown(),
		CheckOutEnabled:           types.BoolUnknown(),
		RequiresComment:           types.BoolValue(true),
		LauncherConnectAsSecretID: types.Int64Unknown(),
		SecretPolicyID:            types.Int64Unknown(),
		CheckOutIntervalMinutes:   types.Int64Value(5),
	}
	fillUnknownSecretSettingsFromState(&plan, state)
	if !plan.Active.Equal(types.BoolValue(true)) || !plan.CheckOutEnabled.Equal(types.BoolValue(true)) {
		t.Fatalf("unknown bools not filled from state: %+v", plan)
	}
	if !plan.RequiresComment.Equal(types.BoolValue(true)) || !plan.CheckOutIntervalMinutes.Equal(types.Int64Value(5)) {
		t.Fatalf("known plan values must be kept: %+v", plan)
	}
	if !plan.LauncherConnectAsSecretID.Equal(types.Int64Value(55)) || !plan.SecretPolicyID.IsNull() {
		t.Fatalf("unknown ints not filled from state: %+v", plan)
	}
}

func TestGetSecretDataWithTemplate_UnknownSettingsAreOmitted(t *testing.T) {
	state := &SecretResourceState{
		Name:                    types.StringValue("n"),
		FolderID:                types.StringValue("7"),
		SiteID:                  types.StringValue("1"),
		SecretTemplateID:        types.StringValue("2"),
		CheckOutEnabled:         types.BoolUnknown(),
		CheckOutIntervalMinutes: types.Int64Unknown(),
		RequiresComment:         types.BoolValue(true),
	}
	secret, err := (&TSSSecretResource{}).getSecretDataWithTemplate(context.Background(), state, nil, &server.SecretTemplate{Name: "tmpl"})
	if err != nil {
		t.Fatal(err)
	}
	if secret.CheckOutEnabled || secret.CheckOutIntervalMinutes != 0 || !secret.RequiresComment || !secret.Active {
		t.Fatalf("request = %+v; unknown settings must stay zero, known ones must be sent", secret)
	}
}
