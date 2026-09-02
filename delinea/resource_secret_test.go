package delinea

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func field(name string) SecretField {
	return SecretField{FieldName: types.StringValue(name)}
}

func fieldNames(fields []SecretField) []string {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.FieldName.ValueString()
	}
	return names
}

func TestAlignFieldsToReference_NilReferenceReturnsInputUnchanged(t *testing.T) {
	fields := []SecretField{field("Resource"), field("Username"), field("Password"), field("Notes")}

	got := alignFieldsToReference(fields, nil)

	if !reflect.DeepEqual(got, fields) {
		t.Fatalf("got %v, want %v", got, fields)
	}
}

func TestAlignFieldsToReference_FiltersToReferenceSubset(t *testing.T) {
	apiResponse := []SecretField{field("Resource"), field("Username"), field("Password"), field("Notes")}
	userConfig := []SecretField{field("Username"), field("Password")}

	got := fieldNames(alignFieldsToReference(apiResponse, userConfig))
	want := []string{"Username", "Password"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAlignFieldsToReference_PreservesReferenceOrder(t *testing.T) {
	apiResponse := []SecretField{field("Resource"), field("Username"), field("Password"), field("Notes")}
	userConfig := []SecretField{field("Password"), field("Username")}

	got := fieldNames(alignFieldsToReference(apiResponse, userConfig))
	want := []string{"Password", "Username"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAlignFieldsToReference_KeepsAllWhenReferenceCoversAll(t *testing.T) {
	apiResponse := []SecretField{field("Resource"), field("Username"), field("Password"), field("Notes")}
	userConfig := []SecretField{field("Resource"), field("Username"), field("Password"), field("Notes")}

	got := fieldNames(alignFieldsToReference(apiResponse, userConfig))
	want := []string{"Resource", "Username", "Password", "Notes"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAlignFieldsToReference_EmptyReferenceYieldsEmptyResult(t *testing.T) {
	apiResponse := []SecretField{field("Resource"), field("Username"), field("Password"), field("Notes")}
	userConfig := []SecretField{}

	got := alignFieldsToReference(apiResponse, userConfig)

	if len(got) != 0 {
		t.Fatalf("got %v, want empty slice", got)
	}
}

func TestAlignFieldsToReference_SkipsReferenceFieldNotInResponse(t *testing.T) {
	apiResponse := []SecretField{field("Username"), field("Password")}
	userConfig := []SecretField{field("Username"), field("NotInTemplate"), field("Password")}

	got := fieldNames(alignFieldsToReference(apiResponse, userConfig))
	want := []string{"Username", "Password"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAlignFieldsToReference_MatchesCaseInsensitively(t *testing.T) {
	apiResponse := []SecretField{field("Username"), field("Password")}
	userConfig := []SecretField{field("username"), field("PASSWORD")}

	got := fieldNames(alignFieldsToReference(apiResponse, userConfig))
	want := []string{"Username", "Password"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAlignFieldsToReference_PreservesPerFieldDataFromResponse(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue("Username"), ItemValue: types.StringValue("myuser"), FieldID: types.Int64Value(101)},
		{FieldName: types.StringValue("Password"), ItemValue: types.StringValue("SuperSecret"), FieldID: types.Int64Value(102)},
		{FieldName: types.StringValue("Notes"), ItemValue: types.StringValue("extra"), FieldID: types.Int64Value(103)},
	}
	userConfig := []SecretField{field("Password"), field("Username")}

	got := alignFieldsToReference(apiResponse, userConfig)

	want := []SecretField{
		{FieldName: types.StringValue("Password"), ItemValue: types.StringValue("SuperSecret"), FieldID: types.Int64Value(102)},
		{FieldName: types.StringValue("Username"), ItemValue: types.StringValue("myuser"), FieldID: types.Int64Value(101)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAlignFieldsToReference_DuplicateReferenceEntriesEachReceiveApiField(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue("Password"), ItemValue: types.StringValue("s3cret"), FieldID: types.Int64Value(102)},
	}
	userConfig := []SecretField{field("Password"), field("Password")}

	got := alignFieldsToReference(apiResponse, userConfig)

	want := []SecretField{
		{FieldName: types.StringValue("Password"), ItemValue: types.StringValue("s3cret"), FieldID: types.Int64Value(102)},
		{FieldName: types.StringValue("Password"), ItemValue: types.StringValue("s3cret"), FieldID: types.Int64Value(102)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Duplicate names from the API are collapsed: the map lookup keeps the last
// entry. Pinned to make the behavior deterministic if TSS ever returns duplicates.
func TestAlignFieldsToReference_DuplicateApiEntriesCollapseToLast(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue("Password"), ItemValue: types.StringValue("first"), FieldID: types.Int64Value(1)},
		{FieldName: types.StringValue("Password"), ItemValue: types.StringValue("second"), FieldID: types.Int64Value(2)},
	}
	userConfig := []SecretField{field("Password")}

	got := alignFieldsToReference(apiResponse, userConfig)

	want := []SecretField{
		{FieldName: types.StringValue("Password"), ItemValue: types.StringValue("second"), FieldID: types.Int64Value(2)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAlignFieldsToReference_EmptyFieldNameMatches(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue(""), ItemValue: types.StringValue("value-for-empty")},
		{FieldName: types.StringValue("Username"), ItemValue: types.StringValue("myuser")},
	}
	userConfig := []SecretField{field("")}

	got := alignFieldsToReference(apiResponse, userConfig)

	want := []SecretField{
		{FieldName: types.StringValue(""), ItemValue: types.StringValue("value-for-empty")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// A null FieldName renders to "" via ValueString() and keys the map the same
// way as an explicit empty string. Pins the Terraform types behavior.
func TestAlignFieldsToReference_NullFieldNameTreatedAsEmptyString(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue(""), ItemValue: types.StringValue("value-for-empty")},
	}
	userConfig := []SecretField{{FieldName: types.StringNull()}}

	got := alignFieldsToReference(apiResponse, userConfig)

	want := []SecretField{
		{FieldName: types.StringValue(""), ItemValue: types.StringValue("value-for-empty")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAlignFieldsToReference_NilFieldsWithNonNilReferenceReturnsEmpty(t *testing.T) {
	userConfig := []SecretField{field("Password"), field("Username")}

	got := alignFieldsToReference(nil, userConfig)

	if len(got) != 0 {
		t.Fatalf("got %v, want empty slice", got)
	}
}

// Pins the behavior that field names appearing in the API response but absent
// from the reference get dropped. This is load-bearing for SSH-key generation
// used without listing key fields in config: pre-fix that pattern hit the
// block-count mismatch; post-fix the generated keys are silently dropped from
// state. The documented SSH-key pattern (examples/secrets/secret_create.tf)
// always lists the key field names and is unaffected.
func TestAlignFieldsToReference_DropsApiFieldAbsentFromReference(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue("Username"), ItemValue: types.StringValue("myuser")},
		{FieldName: types.StringValue("Private Key"), ItemValue: types.StringValue("-----BEGIN...")},
		{FieldName: types.StringValue("Public Key"), ItemValue: types.StringValue("ssh-rsa AAAA")},
	}
	userConfig := []SecretField{field("Username")}

	got := fieldNames(alignFieldsToReference(apiResponse, userConfig))
	want := []string{"Username"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestAlignFieldsToReference_PreservesPasswordWoVersionFromReference(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue("Password"), PasswordWoVersion: types.Int64Null()},
	}
	userConfig := []SecretField{
		{FieldName: types.StringValue("Password"), PasswordWoVersion: types.Int64Value(7)},
	}

	got := alignFieldsToReference(apiResponse, userConfig)

	if len(got) != 1 {
		t.Fatalf("got %d fields, want 1", len(got))
	}
	if got[0].PasswordWoVersion.ValueInt64() != 7 {
		t.Fatalf("got password_wo_version=%v, want 7", got[0].PasswordWoVersion)
	}
}

func TestAlignFieldsToReference_NullPasswordWoVersionInReferenceStaysNull(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue("Password"), PasswordWoVersion: types.Int64Null()},
	}
	userConfig := []SecretField{
		{FieldName: types.StringValue("Password"), PasswordWoVersion: types.Int64Null()},
	}

	got := alignFieldsToReference(apiResponse, userConfig)

	if !got[0].PasswordWoVersion.IsNull() {
		t.Fatalf("got password_wo_version=%v, want null", got[0].PasswordWoVersion)
	}
}

func TestAlignFieldsToReference_PreservesGenerateFromReference(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue("Password"), Generate: types.BoolNull()},
	}
	userConfig := []SecretField{
		{FieldName: types.StringValue("Password"), Generate: types.BoolValue(true)},
	}

	got := alignFieldsToReference(apiResponse, userConfig)

	if len(got) != 1 {
		t.Fatalf("got %d fields, want 1", len(got))
	}
	if !got[0].Generate.ValueBool() {
		t.Fatalf("got generate=%v, want true", got[0].Generate)
	}
}

func TestAlignFieldsToReference_NullGenerateInReferenceStaysNull(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue("Password"), Generate: types.BoolNull()},
	}
	userConfig := []SecretField{
		{FieldName: types.StringValue("Password"), Generate: types.BoolNull()},
	}

	got := alignFieldsToReference(apiResponse, userConfig)

	if !got[0].Generate.IsNull() {
		t.Fatalf("got generate=%v, want null", got[0].Generate)
	}
}

func TestFlattenSecret_NullsItemValueForPasswordFields(t *testing.T) {
	secret := &server.Secret{
		ID:   42,
		Name: "test-secret",
		Fields: []server.SecretField{
			{FieldName: "Username", ItemValue: "myuser", IsPassword: false},
			{FieldName: "Password", ItemValue: "SuperSecret!", IsPassword: true},
			{FieldName: "Notes", ItemValue: "plain-text-ok", IsPassword: false},
		},
	}

	state, err := flattenSecret(secret)
	if err != nil {
		t.Fatalf("flattenSecret returned error: %v", err)
	}

	var pw, user, notes *SecretField
	for i := range state.Fields {
		switch state.Fields[i].FieldName.ValueString() {
		case "Username":
			user = &state.Fields[i]
		case "Password":
			pw = &state.Fields[i]
		case "Notes":
			notes = &state.Fields[i]
		}
	}
	if pw == nil || user == nil || notes == nil {
		t.Fatalf("expected three fields, got Username=%v Password=%v Notes=%v", user, pw, notes)
	}
	if pw.ItemValue.IsNull() || pw.ItemValue.ValueString() != "" {
		t.Fatalf("password itemvalue got %v, want empty string", pw.ItemValue)
	}
	if pw.ItemValue.ValueString() == "SuperSecret!" {
		t.Fatalf("password plaintext leaked into itemvalue: %q", pw.ItemValue.ValueString())
	}
	if user.ItemValue.ValueString() != "myuser" {
		t.Fatalf("username itemvalue got %v, want 'myuser'", user.ItemValue)
	}
	if notes.ItemValue.ValueString() != "plain-text-ok" {
		t.Fatalf("notes itemvalue got %v, want 'plain-text-ok'", notes.ItemValue)
	}
}

func TestFlattenSecret_PasswordValueAlwaysNull(t *testing.T) {
	secret := &server.Secret{
		ID:   42,
		Name: "test-secret",
		Fields: []server.SecretField{
			{FieldName: "Username", ItemValue: "u", IsPassword: false},
			{FieldName: "Password", ItemValue: "p", IsPassword: true},
		},
	}

	state, _ := flattenSecret(secret)

	for _, f := range state.Fields {
		if !f.PasswordValue.IsNull() {
			t.Fatalf("field %s password_value got %v, want null (WriteOnly)", f.FieldName.ValueString(), f.PasswordValue)
		}
	}
}

// PR 1 scenario: partial-fields config. Template id=2 ("Password") defines
// four fields; user config specifies two. Pre-fix, the mismatch between plan
// (2 blocks) and post-apply state (4 blocks) produced Terraform's "Provider
// produced inconsistent result after apply" error. Post-fix, alignment trims
// state to the two fields the user listed, and apply exits 0.
func Example_alignFieldsToReference_partialFields() {
	apiResponse := []SecretField{
		field("Resource"),
		field("Username"),
		field("Password"),
		field("Notes"),
	}
	userConfig := []SecretField{field("Username"), field("Password")}

	aligned := alignFieldsToReference(apiResponse, userConfig)
	for _, f := range aligned {
		fmt.Println(f.FieldName.ValueString())
	}
	// Output:
	// Username
	// Password
}

// PR 1 scenario: all-fields config. User specifies every field the template
// defines. Alignment is effectively a no-op on the set; ordering follows the
// user's config.
func Example_alignFieldsToReference_allFields() {
	apiResponse := []SecretField{
		field("Resource"),
		field("Username"),
		field("Password"),
		field("Notes"),
	}
	userConfig := []SecretField{
		field("Resource"),
		field("Username"),
		field("Password"),
		field("Notes"),
	}

	aligned := alignFieldsToReference(apiResponse, userConfig)
	for _, f := range aligned {
		fmt.Println(f.FieldName.ValueString())
	}
	// Output:
	// Resource
	// Username
	// Password
	// Notes
}

// PR 1 scenario: refresh after a partial-fields create produces no plan diff.
// On Read, the prior state's fields are the reference; alignment preserves
// exactly the field set that was previously in state, so Terraform sees the
// same block count and no drift.
func Example_alignFieldsToReference_refresh() {
	apiResponse := []SecretField{
		field("Resource"),
		field("Username"),
		field("Password"),
		field("Notes"),
	}
	priorState := []SecretField{field("Username"), field("Password")}

	aligned := alignFieldsToReference(apiResponse, priorState)
	for _, f := range aligned {
		fmt.Println(f.FieldName.ValueString())
	}
	// Output:
	// Username
	// Password
}

func fieldsBlockAttributes(t *testing.T) map[string]schema.Attribute {
	t.Helper()
	r := &TSSSecretResource{}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("Schema() returned diagnostics: %v", resp.Diagnostics)
	}
	block, ok := resp.Schema.Blocks["fields"].(schema.ListNestedBlock)
	if !ok {
		t.Fatalf("fields block missing or wrong type: %T", resp.Schema.Blocks["fields"])
	}
	return block.NestedObject.Attributes
}

// PBI 718755: these four attributes are server-assigned. Declaring them
// Computed-only (no Optional) is what makes Terraform reject configs that try
// to set them, which is the diagnostic the customer needed.
func TestSchema_ServerAssignedFieldsAreComputedOnly(t *testing.T) {
	attrs := fieldsBlockAttributes(t)
	for _, name := range []string{"itemid", "fieldid", "slug", "fielddescription"} {
		attr, ok := attrs[name]
		if !ok {
			t.Fatalf("%s: attribute missing from fields block", name)
		}
		if !attr.IsComputed() {
			t.Errorf("%s: IsComputed() = false, want true", name)
		}
		if attr.IsOptional() {
			t.Errorf("%s: IsOptional() = true, want false (setting in config must produce a plan-time error)", name)
		}
	}
}

// fileattachmentid is the documented exception: it is genuinely user-settable
// on file-type fields, so it must stay Optional+Computed.
func TestSchema_FileAttachmentIDIsOptionalAndComputed(t *testing.T) {
	attrs := fieldsBlockAttributes(t)
	attr, ok := attrs["fileattachmentid"]
	if !ok {
		t.Fatal("fileattachmentid: attribute missing from fields block")
	}
	if !attr.IsOptional() {
		t.Error("fileattachmentid: IsOptional() = false, want true")
	}
	if !attr.IsComputed() {
		t.Error("fileattachmentid: IsComputed() = false, want true")
	}
}
