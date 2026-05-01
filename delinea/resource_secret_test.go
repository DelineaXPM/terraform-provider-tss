package delinea

import (
	"fmt"
	"reflect"
	"testing"

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
