package delinea

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUpdateGenerateConflict_SuppressionPreservesValidation(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v1/secret-templates/2" {
			respondTemplate(w, 2, false)
			return true
		}
		return false
	})
	r := &TSSSecretResource{client: client}

	state := SecretResourceState{
		Fields: []SecretField{{
			FieldName:         types.StringValue("Password"),
			Generate:          types.BoolValue(true),
			PasswordWoVersion: types.Int64Value(1),
		}},
	}
	plan := SecretResourceState{
		Name:             types.StringValue("conflict"),
		FolderID:         types.StringValue("7"),
		SiteID:           types.StringValue("1"),
		SecretTemplateID: types.StringValue("2"),
		Fields: []SecretField{{
			FieldName:         types.StringValue("Password"),
			Generate:          types.BoolValue(true),
			PasswordValue:     types.StringValue("x"),
			PasswordWoVersion: types.Int64Value(1),
		}},
	}

	template, err := r.getSecretTemplate(context.Background(), &plan, client)
	if err != nil {
		t.Fatal(err)
	}
	plan.Fields = suppressUnrotatedGenerates(plan.Fields, state.Fields, template)
	_, err = r.getSecretDataWithTemplate(context.Background(), &plan, client, template)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("getSecretData error = %v, want the generate/password_value mutual-exclusion error", err)
	}
}
