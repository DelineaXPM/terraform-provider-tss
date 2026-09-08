package delinea

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func deletionReadState(t *testing.T) tfsdk.State {
	t.Helper()
	schemaResponse := &resource.SchemaResponse{}
	(&TSSSecretDeletionResource{}).Schema(context.Background(), resource.SchemaRequest{}, schemaResponse)
	if schemaResponse.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResponse.Diagnostics)
	}
	state := tfsdk.State{Schema: schemaResponse.Schema}
	diags := state.Set(context.Background(), SecretDeletionResourceState{
		SecretID: types.Int64Value(1),
		ID:       types.StringValue("secret_1"),
	})
	if diags.HasError() {
		t.Fatalf("state diagnostics: %v", diags)
	}
	return state
}

func TestSecretDeletionRead_NotFoundPreservesSuccessfulDeletionSilently(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/secrets/1" {
			return false
		}
		respondStatus(w, http.StatusNotFound)
		return true
	})
	state := deletionReadState(t)
	response := &resource.ReadResponse{State: state}
	(&TSSSecretDeletionResource{client: client}).Read(context.Background(), resource.ReadRequest{State: state}, response)
	if len(response.Diagnostics) != 0 || response.State.Raw.IsNull() {
		t.Fatalf("diagnostics = %v", response.Diagnostics)
	}
}

func TestSecretDeletionRead_BadRequestPreservesSuccessfulDeletionSilently(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/secrets/1" {
			return false
		}
		respondAccessDenied(w)
		return true
	})
	state := deletionReadState(t)
	response := &resource.ReadResponse{State: state}
	(&TSSSecretDeletionResource{client: client}).Read(context.Background(), resource.ReadRequest{State: state}, response)
	if len(response.Diagnostics) != 0 || response.State.Raw.IsNull() {
		t.Fatalf("diagnostics = %v", response.Diagnostics)
	}
}

func TestSecretDeletionRead_InactivePreservesSuccessfulDeletionSilently(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/secrets/1" {
			return false
		}
		respondSecret(w, 1, false)
		return true
	})
	state := deletionReadState(t)
	response := &resource.ReadResponse{State: state}
	(&TSSSecretDeletionResource{client: client}).Read(context.Background(), resource.ReadRequest{State: state}, response)
	if len(response.Diagnostics) != 0 || response.State.Raw.IsNull() {
		t.Fatalf("diagnostics = %v", response.Diagnostics)
	}
}

func TestSecretDeletionRead_ForbiddenReportsAuthentication(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/secrets/1" {
			return false
		}
		respondStatus(w, http.StatusForbidden)
		return true
	})
	state := deletionReadState(t)
	response := &resource.ReadResponse{State: state}
	(&TSSSecretDeletionResource{client: client}).Read(context.Background(), resource.ReadRequest{State: state}, response)
	if !response.Diagnostics.HasError() || response.Diagnostics[0].Summary() != "Secret Authentication or Authorization Failed" ||
		strings.Contains(response.Diagnostics[0].Detail(), "state rm") || response.State.Raw.IsNull() {
		t.Fatalf("diagnostics = %v state = %v", response.Diagnostics, response.State.Raw)
	}
}

func TestSecretDeletionSchema_SecretIDRequiresReplace(t *testing.T) {
	response := &resource.SchemaResponse{}
	(&TSSSecretDeletionResource{}).Schema(context.Background(), resource.SchemaRequest{}, response)
	attribute, ok := response.Schema.Attributes["secret_id"].(schema.Int64Attribute)
	if !ok {
		t.Fatalf("secret_id attribute = %#v", response.Schema.Attributes["secret_id"])
	}
	want := int64planmodifier.RequiresReplace()
	for _, modifier := range attribute.PlanModifiers {
		if modifier.Description(context.Background()) == want.Description(context.Background()) {
			return
		}
	}
	t.Fatalf("secret_id plan modifiers %v lack RequiresReplace", attribute.PlanModifiers)
}

func TestSecretDeletionRead_ActiveSecretWarnsWithoutRetry(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/secrets/1" {
			return false
		}
		respondSecret(w, 1, true)
		return true
	})
	state := deletionReadState(t)
	response := &resource.ReadResponse{State: state}
	(&TSSSecretDeletionResource{client: client}).Read(context.Background(), resource.ReadRequest{State: state}, response)
	if response.Diagnostics.HasError() || response.Diagnostics.WarningsCount() != 1 || response.State.Raw.IsNull() ||
		!strings.Contains(response.Diagnostics[0].Detail(), "will not delete the secret again automatically") {
		t.Fatalf("diagnostics = %v state = %v", response.Diagnostics, response.State.Raw)
	}
}
