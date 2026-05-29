package delinea

import (
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// fakeEnv builds a getenv-compatible func from a map. Empty/missing keys
// return "", which the resolver treats the same as unset.
func fakeEnv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestResolveProviderConfig_AllFromEnv(t *testing.T) {
	in := TSSProviderModel{}
	env := fakeEnv(map[string]string{
		"TSS_SERVER_URL": "https://tss.example.com/SecretServer",
		"TSS_USERNAME":   "envuser",
		"TSS_PASSWORD":   "envpass",
	})

	got, errs := resolveProviderConfig(in, env)

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got.ServerURL.ValueString() != "https://tss.example.com/SecretServer" {
		t.Errorf("ServerURL: got %q, want env value", got.ServerURL.ValueString())
	}
	if got.Username.ValueString() != "envuser" {
		t.Errorf("Username: got %q, want envuser", got.Username.ValueString())
	}
	if got.Password.ValueString() != "envpass" {
		t.Errorf("Password: got %q, want envpass", got.Password.ValueString())
	}
}

// gh #108 reproducer: docs claim TSS_USERNAME / TSS_PASSWORD / TSS_SERVER_URL
// work with no provider block. Without the env-var fallback, terraform plan
// would prompt for provider.tss.password.
func TestResolveProviderConfig_GH108EnvOnlyAuthSucceeds(t *testing.T) {
	emptyConfig := TSSProviderModel{}
	env := fakeEnv(map[string]string{
		"TSS_SERVER_URL": "https://example/SecretServer",
		"TSS_USERNAME":   "alice",
		"TSS_PASSWORD":   "hunter2",
	})

	resolved, errs := resolveProviderConfig(emptyConfig, env)

	if len(errs) != 0 {
		t.Fatalf("env-only auth must not produce errors; got %v", errs)
	}
	if resolved.Username.ValueString() == "" {
		t.Error("expected username to be populated from TSS_USERNAME, got empty")
	}
}

func TestResolveProviderConfig_ConfigBeatsEnv(t *testing.T) {
	in := TSSProviderModel{
		ServerURL: types.StringValue("https://config.example.com/SecretServer"),
		Username:  types.StringValue("configuser"),
		Password:  types.StringValue("configpass"),
	}
	env := fakeEnv(map[string]string{
		"TSS_SERVER_URL": "https://env.example.com/SecretServer",
		"TSS_USERNAME":   "envuser",
		"TSS_PASSWORD":   "envpass",
	})

	got, errs := resolveProviderConfig(in, env)

	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := TSSProviderModel{
		ServerURL: types.StringValue("https://config.example.com/SecretServer"),
		Username:  types.StringValue("configuser"),
		Password:  types.StringValue("configpass"),
		Token:     types.String{},
		Domain:    types.String{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config did not take precedence over env: got %+v, want %+v", got, want)
	}
}

func TestResolveProviderConfig_EmptyStringInConfigFallsToEnv(t *testing.T) {
	in := TSSProviderModel{
		ServerURL: types.StringValue(""), // explicit empty
		Username:  types.StringValue(""),
		Password:  types.StringValue(""),
	}
	env := fakeEnv(map[string]string{
		"TSS_SERVER_URL": "https://env.example.com/SecretServer",
		"TSS_USERNAME":   "envuser",
		"TSS_PASSWORD":   "envpass",
	})

	got, errs := resolveProviderConfig(in, env)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if got.ServerURL.ValueString() != "https://env.example.com/SecretServer" {
		t.Errorf("explicit empty in config should fall through to env; got %q", got.ServerURL.ValueString())
	}
}

func TestResolveProviderConfig_TokenFromEnv(t *testing.T) {
	env := fakeEnv(map[string]string{
		"TSS_SERVER_URL": "https://example/SecretServer",
		"TSS_TOKEN":      "abc123",
	})
	_, errs := resolveProviderConfig(TSSProviderModel{}, env)
	if len(errs) != 0 {
		t.Fatalf("token-only auth via env must succeed; got %v", errs)
	}
}

// Validation cases.

func TestResolveProviderConfig_MissingServerURL(t *testing.T) {
	env := fakeEnv(map[string]string{
		"TSS_USERNAME": "alice",
		"TSS_PASSWORD": "hunter2",
	})
	_, errs := resolveProviderConfig(TSSProviderModel{}, env)
	if len(errs) == 0 {
		t.Fatal("expected error for missing server_url, got none")
	}
}

func TestResolveProviderConfig_MissingCredentials(t *testing.T) {
	env := fakeEnv(map[string]string{
		"TSS_SERVER_URL": "https://example/SecretServer",
	})
	_, errs := resolveProviderConfig(TSSProviderModel{}, env)
	if len(errs) == 0 {
		t.Fatal("expected error for missing credentials, got none")
	}
}

func TestResolveProviderConfig_UsernameWithoutPassword(t *testing.T) {
	in := TSSProviderModel{
		ServerURL: types.StringValue("https://example/SecretServer"),
		Username:  types.StringValue("alice"),
	}
	_, errs := resolveProviderConfig(in, fakeEnv(nil))
	if len(errs) == 0 {
		t.Fatal("expected error for username without password, got none")
	}
}

func TestResolveProviderConfig_PasswordWithoutUsername(t *testing.T) {
	in := TSSProviderModel{
		ServerURL: types.StringValue("https://example/SecretServer"),
		Password:  types.StringValue("hunter2"),
	}
	_, errs := resolveProviderConfig(in, fakeEnv(nil))
	if len(errs) == 0 {
		t.Fatal("expected error for password without username, got none")
	}
}

func TestResolveProviderConfig_BothUsernameAndToken(t *testing.T) {
	in := TSSProviderModel{
		ServerURL: types.StringValue("https://example/SecretServer"),
		Username:  types.StringValue("alice"),
		Password:  types.StringValue("hunter2"),
		Token:     types.StringValue("abc123"),
	}
	_, errs := resolveProviderConfig(in, fakeEnv(nil))
	if len(errs) == 0 {
		t.Fatal("expected error for username/password and token both set, got none")
	}
}

// Mixed config + env: username in config, password from env.
func TestResolveProviderConfig_MixedConfigAndEnv(t *testing.T) {
	in := TSSProviderModel{
		ServerURL: types.StringValue("https://example/SecretServer"),
		Username:  types.StringValue("alice"),
	}
	env := fakeEnv(map[string]string{
		"TSS_PASSWORD": "envpass",
	})
	resolved, errs := resolveProviderConfig(in, env)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if resolved.Password.ValueString() != "envpass" {
		t.Errorf("Password: got %q, want envpass (from env)", resolved.Password.ValueString())
	}
}
