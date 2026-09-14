package delinea

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
)

func TestEphemeralSecretValuesAreSensitive(t *testing.T) {
	single := &ephemeral.SchemaResponse{}
	(&TSSSecretEphemeralResource{}).Schema(context.Background(), ephemeral.SchemaRequest{}, single)
	singleValue, ok := single.Schema.Attributes["value"].(schema.StringAttribute)
	if !ok || !singleValue.Sensitive {
		t.Fatalf("single value attribute = %#v", single.Schema.Attributes["value"])
	}

	multiple := &ephemeral.SchemaResponse{}
	(&TSSSecretsEphemeralResource{}).Schema(context.Background(), ephemeral.SchemaRequest{}, multiple)
	secrets, ok := multiple.Schema.Attributes["secrets"].(schema.ListNestedAttribute)
	if !ok {
		t.Fatalf("secrets attribute = %#v", multiple.Schema.Attributes["secrets"])
	}
	value, ok := secrets.NestedObject.Attributes["value"].(schema.StringAttribute)
	if !ok || !value.Sensitive {
		t.Fatalf("nested value attribute = %#v", secrets.NestedObject.Attributes["value"])
	}
}
