package delinea

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func priorSchemaV0FieldsAttrs(t *testing.T) map[string]schema.Attribute {
	t.Helper()
	s := priorSchemaV0()
	block, ok := s.Blocks["fields"].(schema.ListNestedBlock)
	if !ok {
		t.Fatalf("priorSchemaV0 fields block missing or wrong type: %T", s.Blocks["fields"])
	}
	return block.NestedObject.Attributes
}

func TestUpgradeFieldsV0ToV1_NilReturnsNil(t *testing.T) {
	got := upgradeFieldsV0ToV1(nil)
	if got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestUpgradeFieldsV0ToV1_EmptyReturnsEmpty(t *testing.T) {
	got := upgradeFieldsV0ToV1([]secretFieldV0{})
	if len(got) != 0 {
		t.Fatalf("got %d fields, want 0", len(got))
	}
}

func TestUpgradeFieldsV0ToV1_CopiesEveryV0Attribute(t *testing.T) {
	prior := []secretFieldV0{
		{
			FieldName:        types.StringValue("Password"),
			ItemValue:        types.StringValue("legacy-plaintext"),
			ItemID:           types.Int64Value(101),
			FieldID:          types.Int64Value(7),
			FileAttachmentID: types.Int64Value(0),
			Slug:             types.StringValue("password"),
			FieldDescription: types.StringValue("the password used to access information"),
			Filename:         types.StringNull(),
			IsFile:           types.BoolValue(false),
			IsNotes:          types.BoolValue(false),
			IsPassword:       types.BoolValue(true),
			IsList:           types.BoolValue(false),
			ListType:         types.StringValue("None"),
		},
	}

	got := upgradeFieldsV0ToV1(prior)
	if len(got) != 1 {
		t.Fatalf("got %d fields, want 1", len(got))
	}

	want := SecretField{
		FieldName:        types.StringValue("Password"),
		ItemValue:        types.StringValue("legacy-plaintext"),
		ItemID:           types.Int64Value(101),
		FieldID:          types.Int64Value(7),
		FileAttachmentID: types.Int64Value(0),
		Slug:             types.StringValue("password"),
		FieldDescription: types.StringValue("the password used to access information"),
		Filename:         types.StringNull(),
		IsFile:           types.BoolValue(false),
		IsNotes:          types.BoolValue(false),
		IsPassword:       types.BoolValue(true),
		IsList:           types.BoolValue(false),
		ListType:         types.StringValue("None"),
		// New v1 attributes default to null/zero — that is the contract.
	}
	if !reflect.DeepEqual(got[0], want) {
		t.Fatalf("got %+v, want %+v", got[0], want)
	}
}

// The new v1 attributes (password_value, password_wo_version, generate)
// must be null on upgraded rows so a refresh against v4 doesn't see
// state-vs-config drift on rows that the user never set them on.
func TestUpgradeFieldsV0ToV1_NewAttributesAreNull(t *testing.T) {
	prior := []secretFieldV0{
		{FieldName: types.StringValue("Password"), ItemValue: types.StringValue("legacy")},
	}

	got := upgradeFieldsV0ToV1(prior)

	if !got[0].PasswordValue.IsNull() {
		t.Errorf("PasswordValue: got %v, want null", got[0].PasswordValue)
	}
	if !got[0].PasswordWoVersion.IsNull() {
		t.Errorf("PasswordWoVersion: got %v, want null", got[0].PasswordWoVersion)
	}
	if !got[0].Generate.IsNull() {
		t.Errorf("Generate: got %v, want null", got[0].Generate)
	}
}

func TestUpgradeFieldsV0ToV1_OrderPreserved(t *testing.T) {
	prior := []secretFieldV0{
		{FieldName: types.StringValue("Username")},
		{FieldName: types.StringValue("Password")},
		{FieldName: types.StringValue("Notes")},
	}
	got := upgradeFieldsV0ToV1(prior)
	want := []string{"Username", "Password", "Notes"}
	for i, name := range want {
		if got[i].FieldName.ValueString() != name {
			t.Fatalf("position %d: got %q, want %q", i, got[i].FieldName.ValueString(), name)
		}
	}
}

func TestPriorSchemaV0_HasFieldsBlock(t *testing.T) {
	s := priorSchemaV0()
	if _, ok := s.Blocks["fields"]; !ok {
		t.Fatal("priorSchemaV0 must declare a fields block; got none")
	}
}

// PBI 718755 reverted: in v0/v3, itemid/fieldid/slug/fielddescription
// were Optional+Computed (not yet locked down). The upgrader must
// declare them that way, otherwise framework state parsing rejects
// any v3 state file that has values for those attributes.
func TestPriorSchemaV0_ServerAssignedFieldsAreOptionalAndComputed(t *testing.T) {
	attrs := priorSchemaV0FieldsAttrs(t)
	for _, name := range []string{"itemid", "fieldid", "slug", "fielddescription"} {
		attr, ok := attrs[name]
		if !ok {
			t.Fatalf("v0 fields.%s: attribute missing", name)
		}
		if !attr.IsOptional() {
			t.Errorf("v0 %s: IsOptional() = false, want true", name)
		}
		if !attr.IsComputed() {
			t.Errorf("v0 %s: IsComputed() = false, want true", name)
		}
	}
}

// The v1 schema added password_value, password_wo_version, and generate.
// The v0 prior schema must not declare them, otherwise the framework
// will reject v3 state that lacks those keys.
func TestPriorSchemaV0_DoesNotDeclareV1OnlyAttributes(t *testing.T) {
	attrs := priorSchemaV0FieldsAttrs(t)
	for _, name := range []string{"password_value", "password_wo_version", "generate"} {
		if _, ok := attrs[name]; ok {
			t.Errorf("v0 fields.%s present; v0 must not declare attributes added in v1", name)
		}
	}
}
