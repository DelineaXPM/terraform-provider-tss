package delinea

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
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

func rotationTemplate() *server.SecretTemplate {
	return &server.SecretTemplate{Fields: []server.SecretTemplateField{
		{SecretTemplateFieldID: 7, Name: "Password", FieldSlugName: "password", IsPassword: true},
		{SecretTemplateFieldID: 8, Name: "Private Key Passphrase", FieldSlugName: "private-key-passphrase", IsPassword: true},
	}}
}

func TestAlignFieldsToReference_NilReferenceReturnsNoFields(t *testing.T) {
	fields := []SecretField{field("Resource"), field("Username"), field("Password"), field("Notes")}

	got := alignFieldsToReference(fields, nil)

	if len(got) != 0 {
		t.Fatalf("got %v, want no fields", got)
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
	want := []string{"username", "PASSWORD"}

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

func TestAlignFieldsToReference_NullFieldNameTreatedAsEmptyString(t *testing.T) {
	apiResponse := []SecretField{
		{FieldName: types.StringValue(""), ItemValue: types.StringValue("value-for-empty")},
	}
	userConfig := []SecretField{{FieldName: types.StringNull()}}

	got := alignFieldsToReference(apiResponse, userConfig)

	want := []SecretField{
		{FieldName: types.StringNull(), ItemValue: types.StringValue("value-for-empty")},
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

func TestFlattenSecret_BlanksItemValueForPasswordFields(t *testing.T) {
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

func TestSchema_ServerAssignedFieldsAreComputedOnly(t *testing.T) {
	attrs := fieldsBlockAttributes(t)
	for _, name := range []string{"itemid", "fieldid", "fileattachmentid", "slug", "fielddescription"} {
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

func TestSchema_FilenameValidatesConfiguredNames(t *testing.T) {
	attrs := fieldsBlockAttributes(t)
	attr, ok := attrs["filename"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("filename: attribute missing or wrong type: %T", attrs["filename"])
	}
	if len(attr.PlanModifiers) != 0 {
		t.Fatalf("filename: got %d plan modifiers, want identity-aware resource planning", len(attr.PlanModifiers))
	}
	if len(attr.Validators) != 1 {
		t.Fatalf("filename: got %d validators, want filename validation", len(attr.Validators))
	}
	if _, ok := attr.Validators[0].(filenameValidator); !ok {
		t.Fatalf("filename: validator = %T, want filenameValidator", attr.Validators[0])
	}
}

func TestNormalizeUpdateFieldValuesMatchesStateByIdentity(t *testing.T) {
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{
		{SecretTemplateFieldID: 1, Name: "First", FieldSlugName: "first"},
		{SecretTemplateFieldID: 2, Name: "Second", FieldSlugName: "second"},
	}}
	plan := []SecretField{
		{FieldName: types.StringValue("Second"), ItemValue: types.StringValue("wrong-first")},
		{FieldName: types.StringValue("First"), ItemValue: types.StringValue("wrong-second")},
	}
	config := []SecretField{
		{FieldName: types.StringValue("Second"), ItemValue: types.StringNull(), Filename: types.StringNull()},
		{FieldName: types.StringValue("First"), ItemValue: types.StringNull(), Filename: types.StringNull()},
	}
	state := []SecretField{
		{FieldName: types.StringValue("First"), Slug: types.StringValue("first"), ItemValue: types.StringValue("first-value")},
		{FieldName: types.StringValue("Second"), Slug: types.StringValue("second"), ItemValue: types.StringValue("second-value")},
	}
	got, matched, err := normalizeUpdateFieldValues(plan, config, state, template, true)
	if err != nil {
		t.Fatal(err)
	}
	if !matched[0].matchedState || !matched[1].matchedState || got[0].ItemValue.ValueString() != "second-value" || got[1].ItemValue.ValueString() != "first-value" {
		t.Fatalf("normalized fields = %#v, matched = %v", got, matched)
	}
}

func TestNormalizeUpdateFieldValuesDoesNotCarryRepointedValues(t *testing.T) {
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{
		{SecretTemplateFieldID: 1, Name: "Old", FieldSlugName: "old"},
		{SecretTemplateFieldID: 2, Name: "New", FieldSlugName: "new"},
	}}
	plan := []SecretField{{FieldName: types.StringValue("New"), ItemValue: types.StringValue("old-value"), Filename: types.StringValue("old.txt")}}
	config := []SecretField{{FieldName: types.StringValue("New"), ItemValue: types.StringNull(), Filename: types.StringNull()}}
	state := []SecretField{{FieldName: types.StringValue("Old"), Slug: types.StringValue("old"), ItemValue: types.StringValue("old-value"), Filename: types.StringValue("old.txt")}}
	got, matched, err := normalizeUpdateFieldValues(plan, config, state, template, true)
	if err != nil {
		t.Fatal(err)
	}
	if matched[0].matchedState || !got[0].ItemValue.IsNull() || !got[0].Filename.IsNull() {
		t.Fatalf("normalized repointed field = %#v, matched = %v", got[0], matched)
	}
	request, err := updateRequestFields(got, config, matched)
	if err != nil {
		t.Fatal(err)
	}
	if len(request) != 0 {
		t.Fatalf("repointed field without a write produced request fields: %#v", request)
	}
}

func TestPriorConfiguredFieldIndexDoesNotMatchSlugAlias(t *testing.T) {
	fields := []SecretField{{FieldName: types.StringValue("Old Display Name"), Slug: types.StringValue("new-name")}}
	if _, ok := priorConfiguredFieldIndex(fields, "new-name"); ok {
		t.Fatal("priorConfiguredFieldIndex matched a computed slug instead of the configured field name")
	}
}

func TestUpdateRequestFieldsRequiresContentForNewFilename(t *testing.T) {
	fields := []SecretField{{FieldName: types.StringValue("Attachment"), Filename: types.StringValue("new.pem")}}
	config := []SecretField{{FieldName: types.StringValue("Attachment"), ItemValue: types.StringNull(), Filename: types.StringValue("new.pem")}}
	if _, err := updateRequestFields(fields, config, []updateFieldMetadata{{}}); err == nil {
		t.Fatal("new filename without file content was accepted")
	}
}

func TestUpdateRequestFieldsPreservesZeroByteAttachment(t *testing.T) {
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{
		{SecretTemplateFieldID: 1, Name: "Attachment", FieldSlugName: "attachment", IsFile: true},
	}}
	plan := []SecretField{{FieldName: types.StringValue("Attachment")}}
	config := []SecretField{{FieldName: types.StringValue("Attachment"), ItemValue: types.StringNull(), Filename: types.StringNull()}}
	state := []SecretField{{
		FieldName:        types.StringValue("Attachment"),
		Slug:             types.StringValue("attachment"),
		ItemValue:        types.StringValue(""),
		Filename:         types.StringValue("empty.txt"),
		FileAttachmentID: types.Int64Value(42),
		IsFile:           types.BoolValue(true),
	}}
	normalized, matched, err := normalizeUpdateFieldValues(plan, config, state, template, true)
	if err != nil {
		t.Fatal(err)
	}
	request, err := updateRequestFields(normalized, config, matched)
	if err != nil {
		t.Fatal(err)
	}
	if len(request) != 0 {
		t.Fatalf("omitted zero-byte attachment produced request fields: %#v", request)
	}

	config[0].Filename = types.StringValue("empty.txt")
	normalized, matched, err = normalizeUpdateFieldValues(plan, config, state, template, true)
	if err != nil {
		t.Fatal(err)
	}
	request, err = updateRequestFields(normalized, config, matched)
	if err != nil {
		t.Fatal(err)
	}
	if len(request) != 0 {
		t.Fatalf("unchanged zero-byte attachment filename produced request fields: %#v", request)
	}

	config[0].Filename = types.StringValue("renamed.txt")
	normalized, matched, err = normalizeUpdateFieldValues(plan, config, state, template, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = updateRequestFields(normalized, config, matched); err == nil {
		t.Fatal("zero-byte attachment filename change was accepted without content")
	}

	config[0].ItemValue = types.StringValue("")
	normalized, matched, err = normalizeUpdateFieldValues(plan, config, state, template, true)
	if err != nil {
		t.Fatal(err)
	}
	request, err = updateRequestFields(normalized, config, matched)
	if err != nil {
		t.Fatal(err)
	}
	if len(request) != 1 {
		t.Fatalf("explicit zero-byte attachment deletion produced %d request fields, want 1", len(request))
	}
}

func TestSchema_PasswordValueRejectsEmptyStrings(t *testing.T) {
	attrs := fieldsBlockAttributes(t)
	attr, ok := attrs["password_value"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("password_value: attribute missing or wrong type: %T", attrs["password_value"])
	}
	if len(attr.Validators) != 1 {
		t.Fatalf("password_value: got %d validators, want non-empty validation", len(attr.Validators))
	}
	if _, ok := attr.Validators[0].(passwordValueValidator); !ok {
		t.Fatalf("password_value: validator = %T, want passwordValueValidator", attr.Validators[0])
	}
}

func TestFilenameValidator(t *testing.T) {
	tests := map[string]struct {
		config    types.String
		wantError bool
	}{
		"null":              {config: types.StringNull()},
		"unknown":           {config: types.StringUnknown()},
		"empty":             {config: types.StringValue(""), wantError: true},
		"extensionless":     {config: types.StringValue("private-key"), wantError: true},
		"extension":         {config: types.StringValue("private-key.pem")},
		"hyphen extension":  {config: types.StringValue("private-key.pub-key"), wantError: true},
		"plus extension":    {config: types.StringValue("module.c++"), wantError: true},
		"unicode extension": {config: types.StringValue("résumé.密钥"), wantError: true},
		"multiple periods":  {config: types.StringValue("private-key.tar.gz")},
		"numeric extension": {config: types.StringValue("certificate.123")},
		"underscore suffix": {config: types.StringValue("archive.tar_gz")},
		"dotfile":           {config: types.StringValue(".env"), wantError: true},
		"trailing period":   {config: types.StringValue("private-key."), wantError: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			request := validator.StringRequest{Path: path.Root("filename"), ConfigValue: test.config}
			response := validator.StringResponse{}
			filenameValidator{}.ValidateString(context.Background(), request, &response)
			if response.Diagnostics.HasError() != test.wantError {
				t.Fatalf("HasError = %t, want %t: %v", response.Diagnostics.HasError(), test.wantError, response.Diagnostics)
			}
		})
	}
}

func TestPasswordValueValidator(t *testing.T) {
	tests := map[string]struct {
		config    types.String
		wantError bool
	}{
		"null":     {config: types.StringNull()},
		"unknown":  {config: types.StringUnknown()},
		"empty":    {config: types.StringValue(""), wantError: true},
		"nonempty": {config: types.StringValue("password")},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			request := validator.StringRequest{Path: path.Root("password_value"), ConfigValue: test.config}
			response := validator.StringResponse{}
			passwordValueValidator{}.ValidateString(context.Background(), request, &response)
			if response.Diagnostics.HasError() != test.wantError {
				t.Fatalf("HasError = %t, want %t: %v", response.Diagnostics.HasError(), test.wantError, response.Diagnostics)
			}
		})
	}
}

func TestSchema_SshKeyArgsRequiresReplace(t *testing.T) {
	resp := &resource.SchemaResponse{}
	(&TSSSecretResource{}).Schema(context.Background(), resource.SchemaRequest{}, resp)
	block, ok := resp.Schema.Blocks["sshkeyargs"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatalf("sshkeyargs block = %#v", resp.Schema.Blocks["sshkeyargs"])
	}
	want := objectplanmodifier.RequiresReplace()
	for _, modifier := range block.PlanModifiers {
		if modifier.Description(context.Background()) == want.Description(context.Background()) {
			return
		}
	}
	t.Fatalf("sshkeyargs plan modifiers %v lack RequiresReplace", block.PlanModifiers)
}

func TestSchema_TemplateFieldMetadataIsReadOnly(t *testing.T) {
	resp := &resource.SchemaResponse{}
	(&TSSSecretResource{}).Schema(context.Background(), resource.SchemaRequest{}, resp)
	block, ok := resp.Schema.Blocks["fields"].(schema.ListNestedBlock)
	if !ok {
		t.Fatalf("fields block = %#v", resp.Schema.Blocks["fields"])
	}
	for _, name := range []string{"isfile", "isnotes", "ispassword", "islist", "listtype"} {
		attribute := block.NestedObject.Attributes[name]
		if attribute.IsOptional() || !attribute.IsComputed() {
			t.Errorf("fields.%s optional=%t computed=%t, want false/true", name, attribute.IsOptional(), attribute.IsComputed())
		}
	}
}

func TestSchema_SshKeyArgsChildrenDefaultFalse(t *testing.T) {
	resp := &resource.SchemaResponse{}
	(&TSSSecretResource{}).Schema(context.Background(), resource.SchemaRequest{}, resp)
	block, ok := resp.Schema.Blocks["sshkeyargs"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatalf("sshkeyargs block = %#v", resp.Schema.Blocks["sshkeyargs"])
	}
	for _, name := range []string{"generatepassphrase", "generatesshkeys"} {
		attribute, ok := block.Attributes[name].(schema.BoolAttribute)
		if !ok {
			t.Fatalf("sshkeyargs.%s = %#v", name, block.Attributes[name])
		}
		if attribute.Default == nil {
			t.Errorf("sshkeyargs.%s has no default", name)
		}
	}
}

func TestFlattenSecret_PreservesZeroOptionalIntegers(t *testing.T) {
	state, err := flattenSecret(&server.Secret{})
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]types.Int64{
		"secretpolicyid":            state.SecretPolicyID,
		"passwordtypewebscriptid":   state.PasswordTypeWebScriptID,
		"launcherconnectassecretid": state.LauncherConnectAsSecretID,
		"checkoutintervalminutes":   state.CheckOutIntervalMinutes,
	} {
		if value.IsNull() || value.IsUnknown() || value.ValueInt64() != 0 {
			t.Errorf("%s = %v, want known zero", name, value)
		}
	}
}

func TestSuppressUnrotatedGenerates_UnchangedVersionCleared(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedGenerates(plan, state, rotationTemplate())
	if got[0].Generate.ValueBool() {
		t.Error("generate must be cleared when password_wo_version is unchanged")
	}
	if !plan[0].Generate.ValueBool() {
		t.Error("input plan slice must not be mutated")
	}
}

func TestSuppressUnrotatedGenerates_BumpedVersionKept(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(2),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedGenerates(plan, state, rotationTemplate())
	if !got[0].Generate.ValueBool() {
		t.Error("generate must survive a password_wo_version bump")
	}
}

func TestSuppressUnrotatedGenerates_NewFieldKept(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedGenerates(plan, nil, rotationTemplate())
	if !got[0].Generate.ValueBool() {
		t.Error("generate must survive when the field has no state counterpart")
	}
}

func TestSuppressUnrotatedGenerates_NewlyEnabledKept(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Null(),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(false),
		PasswordWoVersion: types.Int64Null(),
	}}
	got := suppressUnrotatedGenerates(plan, state, rotationTemplate())
	if !got[0].Generate.ValueBool() {
		t.Error("switching generate false->true is a rotation request and must be kept")
	}
}

func TestSuppressUnrotatedGenerates_VersionRemovedIsNotRotation(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Null(),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedGenerates(plan, state, rotationTemplate())
	if got[0].Generate.ValueBool() {
		t.Error("removing password_wo_version is cleanup, not a rotation request")
	}
}

func TestSuppressUnrotatedGenerates_ConflictingValueKeptForValidation(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordValue:     types.StringValue("x"),
		PasswordWoVersion: types.Int64Value(1),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedGenerates(plan, state, rotationTemplate())
	if !got[0].Generate.ValueBool() {
		t.Error("generate with a conflicting password_value must survive so mutual-exclusion validation errors")
	}
}

func TestSuppressUnrotatedGenerates_NonPasswordKeptForValidation(t *testing.T) {
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{{
		SecretTemplateFieldID: 7,
		Name:                  "Password",
		FieldSlugName:         "password",
		IsPassword:            false,
	}}}
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Password"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedGenerates(plan, state, template)
	if !got[0].Generate.ValueBool() {
		t.Fatal("generate was suppressed before non-password validation")
	}
}

func TestSuppressUnrotatedGenerates_SlugFieldnameMatchesState(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("private-key-passphrase"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Private Key Passphrase"),
		Slug:              types.StringValue("private-key-passphrase"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedGenerates(plan, state, rotationTemplate())
	if got[0].Generate.ValueBool() {
		t.Error("a config switched to the field slug must still match state and be suppressed")
	}
}

func TestSuppressUnrotatedGenerates_DisplayNameMatchesSlugState(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Private Key Passphrase"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("private-key-passphrase"),
		Slug:              types.StringValue("private-key-passphrase"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedGenerates(plan, state, rotationTemplate())
	if got[0].Generate.ValueBool() {
		t.Error("a config switched from slug to display name must still be suppressed")
	}
}

func TestSuppressUnrotatedGenerates_TemplateRenameUsesStoredSlug(t *testing.T) {
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{{
		SecretTemplateFieldID: 8,
		Name:                  "Renamed Passphrase",
		FieldSlugName:         "private-key-passphrase",
		IsPassword:            true,
	}}}
	plan := []SecretField{{
		FieldName:         types.StringValue("Renamed Passphrase"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Old Passphrase Name"),
		Slug:              types.StringValue("private-key-passphrase"),
		Generate:          types.BoolValue(true),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedGenerates(plan, state, template)
	if got[0].Generate.ValueBool() {
		t.Error("a template display-name change must still match the stored slug")
	}
}

func TestResolveTemplateField_RejectsAmbiguity(t *testing.T) {
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{
		{SecretTemplateFieldID: 1, Name: "Password", FieldSlugName: "password"},
		{SecretTemplateFieldID: 2, Name: "Password", FieldSlugName: "other-password"},
	}}
	if got, ok := resolveTemplateField(template, SecretField{FieldName: types.StringValue("Password")}); ok {
		t.Fatalf("resolved ambiguous field = %#v", got)
	}
}

func TestResolveTemplateField_ConfigNamePrecedesStoredSlug(t *testing.T) {
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{
		{SecretTemplateFieldID: 1, Name: "Old Field", FieldSlugName: "old-field"},
		{SecretTemplateFieldID: 2, Name: "New Field", FieldSlugName: "new-field"},
	}}
	got, ok := resolveTemplateField(template, SecretField{
		FieldName: types.StringValue("New Field"),
		Slug:      types.StringValue("old-field"),
	})
	if !ok || got.SecretTemplateFieldID != 2 {
		t.Fatalf("resolved field = %#v, ok=%v", got, ok)
	}
}

func TestDiscardStaleFieldSlugs_ChangedNameCannotResolveThroughOldSlug(t *testing.T) {
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{{
		SecretTemplateFieldID: 1,
		Name:                  "Old Field",
		FieldSlugName:         "old-field",
	}}}
	plan := []SecretField{{
		FieldName: types.StringValue("Misspelled Field"),
		Slug:      types.StringValue("old-field"),
	}}
	state := []SecretField{{
		FieldName: types.StringValue("Old Field"),
		Slug:      types.StringValue("old-field"),
	}}
	got := discardStaleFieldSlugs(plan, state, true)
	if _, ok := resolveTemplateField(template, got[0]); ok {
		t.Fatalf("changed field resolved through stale slug: %#v", got[0])
	}
}

func TestDiscardStaleFieldSlugs_UnchangedNameRetainsRenameFallback(t *testing.T) {
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{{
		SecretTemplateFieldID: 1,
		Name:                  "Renamed Field",
		FieldSlugName:         "stable-field",
	}}}
	plan := []SecretField{{
		FieldName: types.StringValue("Old Field"),
		Slug:      types.StringValue("stable-field"),
	}}
	state := []SecretField{{
		FieldName: types.StringValue("Old Field"),
		Slug:      types.StringValue("stable-field"),
	}}
	got := discardStaleFieldSlugs(plan, state, true)
	resolved, ok := resolveTemplateField(template, got[0])
	if !ok || resolved.SecretTemplateFieldID != 1 {
		t.Fatalf("resolved field = %#v, ok=%v", resolved, ok)
	}
}

func TestDiscardStaleFieldSlugs_TemplateChangeDropsMatchingNameSlug(t *testing.T) {
	plan := []SecretField{{
		FieldName: types.StringValue("Old Field"),
		Slug:      types.StringValue("old-field"),
	}}
	state := []SecretField{{
		FieldName: types.StringValue("Old Field"),
		Slug:      types.StringValue("old-field"),
	}}
	got := discardStaleFieldSlugs(plan, state, false)
	if !got[0].Slug.IsNull() {
		t.Fatalf("template change retained stale slug: %#v", got[0])
	}
}

func TestValidatePasswordRotationVersions_RejectsVersionOnlyChange(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		PasswordWoVersion: types.Int64Value(2),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Password"),
		PasswordWoVersion: types.Int64Value(1),
	}}
	if err := validatePasswordRotationVersions(plan, state, rotationTemplate()); err == nil {
		t.Fatal("version-only password rotation was accepted")
	}
}

func TestValidatePasswordRotationVersions_RejectsVersionOnlyNewField(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		PasswordWoVersion: types.Int64Value(1),
	}}
	if err := validatePasswordRotationVersions(plan, nil, rotationTemplate()); err == nil {
		t.Fatal("version-only new field was accepted")
	}
}

func TestValidatePasswordRotationVersions_RejectsPasswordWithoutVersion(t *testing.T) {
	plan := []SecretField{{
		FieldName:     types.StringValue("Password"),
		PasswordValue: types.StringValue("rotated"),
	}}
	err := validatePasswordRotationVersions(plan, nil, rotationTemplate())
	if err == nil {
		t.Fatal("password_value without password_wo_version was accepted")
	}
	if !strings.Contains(err.Error(), "password_value") || !strings.Contains(err.Error(), "password_wo_version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePasswordRotationVersions_RejectsEmptyOrUnknownPassword(t *testing.T) {
	for name, value := range map[string]types.String{
		"empty":   types.StringValue(""),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			plan := []SecretField{{
				FieldName:         types.StringValue("Password"),
				PasswordValue:     value,
				PasswordWoVersion: types.Int64Value(1),
			}}
			err := validatePasswordRotationVersions(plan, nil, rotationTemplate())
			if err == nil {
				t.Fatalf("%s password_value was accepted", name)
			}
			if !strings.Contains(err.Error(), name) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidatePasswordRotationVersions_AcceptsPasswordAndGenerateChanges(t *testing.T) {
	state := []SecretField{{
		FieldName:         types.StringValue("Password"),
		PasswordWoVersion: types.Int64Value(1),
	}}
	for name, plan := range map[string][]SecretField{
		"password_value": {{
			FieldName:         types.StringValue("Password"),
			PasswordValue:     types.StringValue("rotated"),
			PasswordWoVersion: types.Int64Value(2),
		}},
		"generate": {{
			FieldName:         types.StringValue("Password"),
			Generate:          types.BoolValue(true),
			PasswordWoVersion: types.Int64Value(2),
		}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePasswordRotationVersions(plan, state, rotationTemplate()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreparePasswordRotationFields_TemplateChangeDoesNotSuppressRequests(t *testing.T) {
	state := []SecretField{{
		FieldName:         types.StringValue("Password"),
		PasswordWoVersion: types.Int64Value(1),
		Generate:          types.BoolValue(true),
	}}
	for name, plan := range map[string][]SecretField{
		"password_value": {{
			FieldName:         types.StringValue("Password"),
			PasswordValue:     types.StringValue("new-template-value"),
			PasswordWoVersion: types.Int64Value(1),
		}},
		"generate": {{
			FieldName:         types.StringValue("Password"),
			Generate:          types.BoolValue(true),
			PasswordWoVersion: types.Int64Value(1),
		}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := preparePasswordRotationFields(plan, state, rotationTemplate(), false)
			if err != nil {
				t.Fatal(err)
			}
			if name == "password_value" && got[0].PasswordValue.ValueString() != "new-template-value" {
				t.Fatalf("password_value was suppressed: %v", got[0].PasswordValue)
			}
			if name == "generate" && !got[0].Generate.ValueBool() {
				t.Fatal("generate was suppressed")
			}
		})
	}
}

func TestGetSecretDataWithTemplate_RejectsPasswordValueOnNonPasswordField(t *testing.T) {
	state := SecretResourceState{
		Name:             types.StringValue("test"),
		FolderID:         types.StringValue("1"),
		SiteID:           types.StringValue("1"),
		SecretTemplateID: types.StringValue("2"),
		Fields: []SecretField{{
			FieldName:     types.StringValue("Username"),
			PasswordValue: types.StringValue("must-not-enter-state"),
		}},
	}
	template := &server.SecretTemplate{
		Name: "Password",
		Fields: []server.SecretTemplateField{{
			SecretTemplateFieldID: 1,
			Name:                  "Username",
			FieldSlugName:         "username",
		}},
	}
	_, err := (&TSSSecretResource{}).getSecretDataWithTemplate(context.Background(), &state, nil, template)
	if err == nil || !strings.Contains(err.Error(), "not a password field") {
		t.Fatalf("getSecretDataWithTemplate error = %v", err)
	}
}

func TestGetSecretDataWithTemplate_RejectsPasswordValueAndItemValue(t *testing.T) {
	state := SecretResourceState{
		Name:             types.StringValue("test"),
		FolderID:         types.StringValue("1"),
		SiteID:           types.StringValue("1"),
		SecretTemplateID: types.StringValue("2"),
		Fields: []SecretField{{
			FieldName:     types.StringValue("Password"),
			PasswordValue: types.StringValue("write-only"),
			ItemValue:     types.StringValue("stateful"),
		}},
	}
	template := &server.SecretTemplate{
		Name: "Password",
		Fields: []server.SecretTemplateField{{
			SecretTemplateFieldID: 1,
			Name:                  "Password",
			FieldSlugName:         "password",
			IsPassword:            true,
		}},
	}
	_, err := (&TSSSecretResource{}).getSecretDataWithTemplate(context.Background(), &state, nil, template)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("getSecretDataWithTemplate error = %v", err)
	}
}

func TestGetSecretDataWithTemplate_RejectsEmptyOrUnknownPasswordValue(t *testing.T) {
	for name, value := range map[string]types.String{
		"empty":   types.StringValue(""),
		"unknown": types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			state := SecretResourceState{
				Name:             types.StringValue("test"),
				FolderID:         types.StringValue("1"),
				SiteID:           types.StringValue("1"),
				SecretTemplateID: types.StringValue("2"),
				Fields: []SecretField{{
					FieldName:     types.StringValue("Password"),
					PasswordValue: value,
				}},
			}
			template := &server.SecretTemplate{Name: "Password", Fields: []server.SecretTemplateField{{
				SecretTemplateFieldID: 1,
				Name:                  "Password",
				FieldSlugName:         "password",
				IsPassword:            true,
			}}}
			_, err := (&TSSSecretResource{}).getSecretDataWithTemplate(context.Background(), &state, nil, template)
			if err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("getSecretDataWithTemplate error = %v", err)
			}
		})
	}
}

func TestGetSecretDataWithTemplate_Active(t *testing.T) {
	tests := map[string]struct {
		value types.Bool
		want  bool
	}{
		"null":    {value: types.BoolNull(), want: true},
		"unknown": {value: types.BoolUnknown(), want: true},
		"true":    {value: types.BoolValue(true), want: true},
		"false":   {value: types.BoolValue(false), want: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			state := SecretResourceState{
				Name:             types.StringValue("test"),
				FolderID:         types.StringValue("1"),
				SiteID:           types.StringValue("1"),
				SecretTemplateID: types.StringValue("2"),
				Active:           test.value,
			}
			secret, err := (&TSSSecretResource{}).getSecretDataWithTemplate(context.Background(), &state, nil, &server.SecretTemplate{})
			if err != nil {
				t.Fatal(err)
			}
			if secret.Active != test.want {
				t.Fatalf("Active = %t, want %t", secret.Active, test.want)
			}
		})
	}
}

func TestGetSecretDataWithTemplate_UsesTemplateForFileMetadataAndOmitsAttachmentID(t *testing.T) {
	state := SecretResourceState{
		Name:             types.StringValue("test"),
		FolderID:         types.StringValue("1"),
		SiteID:           types.StringValue("1"),
		SecretTemplateID: types.StringValue("2"),
		Fields: []SecretField{{
			FieldName:        types.StringValue("Attachment"),
			ItemValue:        types.StringValue("contents"),
			FileAttachmentID: types.Int64Value(17),
			Filename:         types.StringValue("payload.bin"),
			IsFile:           types.BoolUnknown(),
		}},
	}
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{{
		SecretTemplateFieldID: 9,
		Name:                  "Attachment",
		FieldSlugName:         "attachment",
		IsFile:                true,
	}}}
	secret, err := (&TSSSecretResource{}).getSecretDataWithTemplate(context.Background(), &state, nil, template)
	if err != nil {
		t.Fatal(err)
	}
	if len(secret.Fields) != 1 || !secret.Fields[0].IsFile || secret.Fields[0].FileAttachmentID != 0 || secret.Fields[0].Filename != "payload.bin" {
		t.Fatalf("file field = %#v", secret.Fields)
	}
}

func TestGetSecretDataWithTemplate_RejectsFilenameOnNonFileField(t *testing.T) {
	for name, filename := range map[string]types.String{
		"configured": types.StringValue("payload.bin"),
		"empty":      types.StringValue(""),
		"null":       types.StringNull(),
		"unknown":    types.StringUnknown(),
	} {
		t.Run(name, func(t *testing.T) {
			state := SecretResourceState{
				Name:             types.StringValue("test"),
				FolderID:         types.StringValue("1"),
				SiteID:           types.StringValue("1"),
				SecretTemplateID: types.StringValue("2"),
				Fields: []SecretField{{
					FieldName: types.StringValue("Username"),
					ItemValue: types.StringValue("admin"),
					Filename:  filename,
				}},
			}
			template := &server.SecretTemplate{Name: "Password", Fields: []server.SecretTemplateField{{
				SecretTemplateFieldID: 1,
				Name:                  "Username",
				FieldSlugName:         "username",
			}}}
			_, err := (&TSSSecretResource{}).getSecretDataWithTemplate(context.Background(), &state, nil, template)
			if (err != nil) != (name == "configured") {
				t.Fatalf("getSecretDataWithTemplate error = %v", err)
			}
			if err != nil && !strings.Contains(err.Error(), "not a file field") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestPreserveSSHKeyFieldValues_UsesTemplateFlags(t *testing.T) {
	fields := []server.SecretField{{FieldName: "Credential Material", Slug: "credential-material", IsFile: true}}
	state := []SecretField{{
		FieldName: types.StringValue("credential-material"),
		Slug:      types.StringValue("credential-material"),
		ItemValue: types.StringValue("generated-key"),
		Filename:  types.StringValue("id_rsa"),
	}}
	plan := []SecretField{{FieldName: types.StringValue("credential-material"), ItemValue: types.StringNull()}}
	preserveSSHKeyFieldValues(fields, state, plan, &SshKeyArgs{GenerateSshKeys: types.BoolValue(true)})
	if fields[0].ItemValue != "generated-key" || fields[0].Filename != "id_rsa" {
		t.Fatalf("SSH field was not preserved: %#v", fields[0])
	}
}

func TestPreserveSSHKeyFieldValues_HonorsExplicitFilename(t *testing.T) {
	fields := []server.SecretField{{FieldName: "Credential Material", Slug: "credential-material", IsFile: true, Filename: "new.pem"}}
	state := []SecretField{{
		FieldName: types.StringValue("credential-material"),
		Slug:      types.StringValue("credential-material"),
		ItemValue: types.StringValue("generated-key"),
		Filename:  types.StringValue("old.pem"),
	}}
	plan := []SecretField{{
		FieldName: types.StringValue("credential-material"),
		ItemValue: types.StringValue("generated-key"),
		Filename:  types.StringValue("new.pem"),
	}}
	preserveSSHKeyFieldValues(fields, state, plan, &SshKeyArgs{GenerateSshKeys: types.BoolValue(true)})
	if fields[0].Filename != "new.pem" {
		t.Fatalf("Filename = %q, want explicit plan filename", fields[0].Filename)
	}
}

func TestPreserveSSHKeyFieldValues_HonorsExplicitEmptyItemValue(t *testing.T) {
	fields := []server.SecretField{{FieldName: "Credential Material", Slug: "credential-material", IsFile: true}}
	state := []SecretField{{
		FieldName: types.StringValue("credential-material"),
		Slug:      types.StringValue("credential-material"),
		ItemValue: types.StringValue("generated-key"),
	}}
	plan := []SecretField{{
		FieldName: types.StringValue("credential-material"),
		ItemValue: types.StringValue(""),
	}}
	preserveSSHKeyFieldValues(fields, state, plan, &SshKeyArgs{GenerateSshKeys: types.BoolValue(true)})
	if fields[0].ItemValue != "" {
		t.Fatalf("ItemValue = %q, want explicit empty value", fields[0].ItemValue)
	}
}

func TestPreserveSSHKeyFieldValues_DoesNotOverwritePasswordRequest(t *testing.T) {
	for name, plan := range map[string]SecretField{
		"password_value": {
			FieldName:     types.StringValue("Private Key Passphrase"),
			PasswordValue: types.StringValue("rotated"),
		},
		"generate": {
			FieldName: types.StringValue("Private Key Passphrase"),
			Generate:  types.BoolValue(true),
		},
	} {
		t.Run(name, func(t *testing.T) {
			fields := []server.SecretField{{
				FieldName:  "Private Key Passphrase",
				Slug:       "private-key-passphrase",
				IsPassword: true,
				ItemValue:  "rotated",
			}}
			state := []SecretField{{
				FieldName: types.StringValue("Private Key Passphrase"),
				Slug:      types.StringValue("private-key-passphrase"),
				ItemValue: types.StringValue(""),
			}}
			preserveSSHKeyFieldValues(fields, state, []SecretField{plan}, &SshKeyArgs{GeneratePassphrase: types.BoolValue(true)})
			if fields[0].ItemValue != "rotated" {
				t.Fatalf("password request was overwritten: %#v", fields[0])
			}
		})
	}
}

func TestAlignFieldsToReference_SlugReferenceMatchesServerName(t *testing.T) {
	fields := []SecretField{{
		FieldName: types.StringValue("Private Key Passphrase"),
		Slug:      types.StringValue("private-key-passphrase"),
		ItemValue: types.StringValue("v"),
	}}
	reference := []SecretField{{
		FieldName:         types.StringValue("private-key-passphrase"),
		PasswordWoVersion: types.Int64Value(2),
		Generate:          types.BoolValue(true),
	}}
	got := alignFieldsToReference(fields, reference)
	if len(got) != 1 {
		t.Fatalf("got %d aligned fields, want 1", len(got))
	}
	if got[0].ItemValue.ValueString() != "" {
		t.Errorf("ItemValue = %q, want password value scrubbed", got[0].ItemValue.ValueString())
	}
	if got[0].FieldName.ValueString() != "private-key-passphrase" {
		t.Errorf("FieldName = %q, want the configured slug", got[0].FieldName.ValueString())
	}
	if !got[0].Generate.ValueBool() || got[0].PasswordWoVersion.ValueInt64() != 2 {
		t.Error("reference-owned generate and password_wo_version must be preserved")
	}
}

func TestAlignFieldsToReference_CanonicalNameStillMatches(t *testing.T) {
	fields := []SecretField{{
		FieldName: types.StringValue("Password"),
		Slug:      types.StringValue("password"),
		ItemValue: types.StringValue("v"),
	}}
	reference := []SecretField{{FieldName: types.StringValue("password")}}
	got := alignFieldsToReference(fields, reference)
	if len(got) != 1 {
		t.Fatalf("case-insensitive name match: got %d aligned fields, want 1", len(got))
	}
	if got[0].FieldName.ValueString() != "password" {
		t.Errorf("FieldName = %q, want the configured spelling", got[0].FieldName.ValueString())
	}
}

func TestAlignFieldsToReference_ServerRenameMatchesStoredSlug(t *testing.T) {
	fields := []SecretField{{
		FieldName: types.StringValue("Renamed Password"),
		Slug:      types.StringValue("password"),
		ItemValue: types.StringValue("v"),
	}}
	reference := []SecretField{{
		FieldName: types.StringValue("Old Password Name"),
		Slug:      types.StringValue("password"),
	}}
	got := alignFieldsToReference(fields, reference)
	if len(got) != 1 || got[0].FieldName.ValueString() != "Old Password Name" {
		t.Fatalf("aligned fields = %#v", got)
	}
}

func TestAlignFieldsToReference_ConfigNamePrecedesStaleSlug(t *testing.T) {
	fields := []SecretField{
		{FieldName: types.StringValue("Old Field"), Slug: types.StringValue("old-field"), ItemValue: types.StringValue("old")},
		{FieldName: types.StringValue("New Field"), Slug: types.StringValue("new-field"), ItemValue: types.StringValue("new")},
	}
	reference := []SecretField{{
		FieldName: types.StringValue("New Field"),
		Slug:      types.StringValue("old-field"),
	}}
	got := alignFieldsToReference(fields, reference)
	if len(got) != 1 || got[0].ItemValue.ValueString() != "new" {
		t.Fatalf("aligned fields = %#v", got)
	}
}

func TestAlignFieldsToReference_RejectsNameSlugCollision(t *testing.T) {
	fields := []SecretField{
		{FieldName: types.StringValue("Account"), Slug: types.StringValue("account-name")},
		{FieldName: types.StringValue("Account Name"), Slug: types.StringValue("account")},
	}
	got := alignFieldsToReference(fields, []SecretField{{FieldName: types.StringValue("account")}})
	if len(got) != 0 {
		t.Fatalf("ambiguous aligned fields = %#v", got)
	}
}

func TestMergeWriteOnlyFieldValues_CopiesPasswordValueByName(t *testing.T) {
	plan := []SecretField{{FieldName: types.StringValue("Password")}}
	config := []SecretField{{
		FieldName:     types.StringValue("password"),
		PasswordValue: types.StringValue("wo-value"),
	}}
	if err := mergeWriteOnlyFieldValues(plan, config); err != nil {
		t.Fatalf("mergeWriteOnlyFieldValues: %v", err)
	}
	if plan[0].PasswordValue.ValueString() != "wo-value" {
		t.Errorf("PasswordValue = %q, want the config write-only value", plan[0].PasswordValue.ValueString())
	}
}

func TestMergeWriteOnlyFieldValues_PreservesDuplicatePositions(t *testing.T) {
	plan := []SecretField{
		{FieldName: types.StringValue("Password")},
		{FieldName: types.StringValue("Password")},
	}
	config := []SecretField{
		{FieldName: types.StringValue("Password"), PasswordValue: types.StringValue("first")},
		{FieldName: types.StringValue("Password"), PasswordValue: types.StringValue("second")},
	}
	if err := mergeWriteOnlyFieldValues(plan, config); err != nil {
		t.Fatalf("mergeWriteOnlyFieldValues: %v", err)
	}
	if plan[0].PasswordValue.ValueString() != "first" || plan[1].PasswordValue.ValueString() != "second" {
		t.Fatalf("PasswordValue positions = %q, %q", plan[0].PasswordValue.ValueString(), plan[1].PasswordValue.ValueString())
	}
}

func TestMergeWriteOnlyFieldValues_RejectsMisalignment(t *testing.T) {
	plan := []SecretField{{FieldName: types.StringValue("Password")}}
	config := []SecretField{{FieldName: types.StringValue("Username"), PasswordValue: types.StringValue("wrong")}}
	if err := mergeWriteOnlyFieldValues(plan, config); err == nil {
		t.Fatal("expected mismatched plan and config fields to fail")
	}
}

func TestSuppressUnrotatedPasswordValues(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		PasswordValue:     types.StringValue("changed"),
		PasswordWoVersion: types.Int64Value(1),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Old Password Name"),
		Slug:              types.StringValue("password"),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedPasswordValues(plan, state, rotationTemplate())
	if !got[0].PasswordValue.IsNull() {
		t.Fatalf("unchanged version retained password_value: %v", got[0].PasswordValue)
	}
	plan[0].PasswordWoVersion = types.Int64Value(2)
	got = suppressUnrotatedPasswordValues(plan, state, rotationTemplate())
	if got[0].PasswordValue.ValueString() != "changed" {
		t.Fatalf("version bump suppressed password_value: %v", got[0].PasswordValue)
	}
}

func TestSuppressUnrotatedPasswordValues_GenerateToExplicitIsNotSuppressed(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		PasswordValue:     types.StringValue("explicit"),
		PasswordWoVersion: types.Int64Value(1),
		Generate:          types.BoolValue(false),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Password"),
		PasswordWoVersion: types.Int64Value(1),
		Generate:          types.BoolValue(true),
	}}
	got := suppressUnrotatedPasswordValues(plan, state, rotationTemplate())
	if got[0].PasswordValue.ValueString() != "explicit" {
		t.Fatalf("generate-to-explicit password was suppressed: %v", got[0].PasswordValue)
	}
}

func TestSuppressUnrotatedPasswordValues_NonPasswordTemplateFieldIsNotSuppressed(t *testing.T) {
	plan := []SecretField{{
		FieldName:         types.StringValue("Password"),
		PasswordValue:     types.StringValue("explicit"),
		PasswordWoVersion: types.Int64Value(1),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Password"),
		PasswordWoVersion: types.Int64Value(1),
	}}
	template := &server.SecretTemplate{ID: 2, Name: "changed", Fields: []server.SecretTemplateField{{
		SecretTemplateFieldID: 7,
		Name:                  "Password",
		FieldSlugName:         "password",
	}}}
	got := suppressUnrotatedPasswordValues(plan, state, template)
	if got[0].PasswordValue.ValueString() != "explicit" {
		t.Fatalf("password value was suppressed after the field stopped being a password: %v", got[0].PasswordValue)
	}
	resourceState := &SecretResourceState{
		Name:             types.StringValue("s"),
		FolderID:         types.StringValue("1"),
		SiteID:           types.StringValue("1"),
		SecretTemplateID: types.StringValue("2"),
		Fields:           got,
	}
	if _, err := (&TSSSecretResource{}).getSecretDataWithTemplate(context.Background(), resourceState, nil, template); err == nil || !strings.Contains(err.Error(), "not a password field") {
		t.Fatalf("getSecretDataWithTemplate error = %v", err)
	}
}

func TestAlignFieldsToReference_ScrubsPasswordUsingPriorIdentity(t *testing.T) {
	fields := []SecretField{{
		FieldName:  types.StringValue("Password"),
		Slug:       types.StringValue("password"),
		ItemValue:  types.StringValue("server-secret"),
		IsPassword: types.BoolValue(false),
	}}
	reference := []SecretField{{
		FieldName:  types.StringValue("Password"),
		Slug:       types.StringValue("password"),
		IsPassword: types.BoolValue(true),
	}}
	got := alignFieldsToReference(fields, reference)
	if len(got) != 1 || got[0].ItemValue.ValueString() != "" {
		t.Fatalf("aligned password field leaked itemvalue: %#v", got)
	}
}

func TestGetSecretDataWithTemplate_RejectsAmbiguousFieldAlias(t *testing.T) {
	template := &server.SecretTemplate{ID: 2, Name: "collision", Fields: []server.SecretTemplateField{
		{SecretTemplateFieldID: 7, Name: "Account", FieldSlugName: "account-name"},
		{SecretTemplateFieldID: 8, Name: "Account Name", FieldSlugName: "account"},
	}}
	state := &SecretResourceState{
		Name:             types.StringValue("s"),
		FolderID:         types.StringValue("1"),
		SiteID:           types.StringValue("1"),
		SecretTemplateID: types.StringValue("2"),
		Fields:           []SecretField{{FieldName: types.StringValue("account")}},
	}
	if _, err := (&TSSSecretResource{}).getSecretDataWithTemplate(context.Background(), state, nil, template); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("getSecretDataWithTemplate error = %v", err)
	}
}

func TestServerIntegerConversionsUsePortableRange(t *testing.T) {
	if _, err := toPositiveServerInt(1<<32+7, "id"); err == nil {
		t.Fatal("oversized secret ID was accepted")
	}
	if _, err := parsePositiveServerInt("0", "id"); err == nil {
		t.Fatal("zero secret ID was accepted")
	}
	if got, err := toServerInt(-1, "setting"); err != nil || got != -1 {
		t.Fatalf("valid sentinel conversion = %d, %v", got, err)
	}
}

func TestUpdateReferenceFields_AlignRenamedServerField(t *testing.T) {
	plan := []SecretField{{FieldName: types.StringValue("Old Name"), Slug: types.StringUnknown()}}
	state := []SecretField{{FieldName: types.StringValue("Old Name"), Slug: types.StringValue("stable-slug")}}
	reference := discardStaleFieldSlugs(plan, state, true)
	result := []SecretField{{FieldName: types.StringValue("New Name"), Slug: types.StringValue("stable-slug")}}
	aligned := alignFieldsToReference(result, reference)
	if len(aligned) != 1 || aligned[0].FieldName.ValueString() != "Old Name" {
		t.Fatalf("aligned fields = %#v", aligned)
	}
}

func TestSuppressUnrotatedPasswordValues_RepointedFieldUsesConfiguredName(t *testing.T) {
	template := &server.SecretTemplate{Fields: []server.SecretTemplateField{
		{SecretTemplateFieldID: 1, Name: "Old Field", FieldSlugName: "old-field", IsPassword: true},
		{SecretTemplateFieldID: 2, Name: "New Field", FieldSlugName: "new-field", IsPassword: true},
	}}
	plan := []SecretField{{
		FieldName:         types.StringValue("New Field"),
		Slug:              types.StringValue("old-field"),
		PasswordValue:     types.StringValue("new-value"),
		PasswordWoVersion: types.Int64Value(1),
	}}
	state := []SecretField{{
		FieldName:         types.StringValue("Old Field"),
		Slug:              types.StringValue("old-field"),
		PasswordWoVersion: types.Int64Value(1),
	}}
	got := suppressUnrotatedPasswordValues(plan, state, template)
	if got[0].PasswordValue.ValueString() != "new-value" {
		t.Fatalf("repointed field password was suppressed: %v", got[0].PasswordValue)
	}
}

func TestReadSecretByID_NotFoundFailsClosed(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/secrets/1" {
			return false
		}
		respondStatus(w, http.StatusNotFound)
		return true
	})
	_, diags := (&TSSSecretResource{}).readSecretByID(context.Background(), 1, client, nil)
	if !diags.HasError() || !strings.Contains(diags[0].Detail(), "terraform state rm") {
		t.Fatalf("diagnostics = %v", diags)
	}
}

func TestReadSecretByID_ForbiddenReportsAuthenticationWithoutStateRemoval(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/secrets/1" {
			return false
		}
		respondStatus(w, http.StatusForbidden)
		return true
	})
	_, diags := (&TSSSecretResource{}).readSecretByID(context.Background(), 1, client, nil)
	if !diags.HasError() || diags[0].Summary() != "Secret Authentication or Authorization Failed" || strings.Contains(diags[0].Detail(), "state rm") {
		t.Fatalf("diagnostics = %v", diags)
	}
}

func TestRecordTaintedSecret_PersistsRecoveryIdentifiers(t *testing.T) {
	schemaResponse := &resource.SchemaResponse{}
	(&TSSSecretResource{}).Schema(context.Background(), resource.SchemaRequest{}, schemaResponse)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResponse.Diagnostics)
	}
	response := &resource.CreateResponse{
		State: tfsdk.State{Schema: schemaResponse.Schema},
	}
	plan := SecretResourceState{
		Name:             types.StringValue("partial"),
		FolderID:         types.StringValue("7"),
		SiteID:           types.StringValue("1"),
		SecretTemplateID: types.StringValue("2"),
	}
	recordTaintedSecret(context.Background(), response, plan, 123, "partial", "write failed.")
	if !response.Diagnostics.HasError() {
		t.Fatal("recordTaintedSecret must return the create error")
	}
	var state SecretResourceState
	diags := response.State.Get(context.Background(), &state)
	if diags.HasError() {
		t.Fatalf("reading partial state: %v", diags)
	}
	if state.ID.ValueInt64() != 123 ||
		state.Name.ValueString() != "partial" ||
		state.FolderID.ValueString() != "7" ||
		state.SiteID.ValueString() != "1" ||
		state.SecretTemplateID.ValueString() != "2" {
		t.Fatalf("recovery identifiers were not preserved: %#v", state)
	}
	if state.Fields != nil {
		t.Fatalf("partial state retained fields: %#v", state.Fields)
	}
}
