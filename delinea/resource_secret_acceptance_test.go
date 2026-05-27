package delinea

// Acceptance tests for tss_resource_secret. These are full end-to-end tests
// that run `terraform apply` against a live Secret Server tenant and assert
// on the resulting state.
//
// These tests are gated by TF_ACC=1 (the terraform-plugin-testing convention)
// so they do not run in the default `go test` pass and will not execute in CI.
// They are intended to be run by a developer against a tenant they control.
//
// Required environment variables:
//
//	TF_ACC=1                 Enables acceptance tests; otherwise resource.Test skips.
//	TSS_SERVER_URL           Base URL for the Secret Server REST API. For a Platform
//	                         tenant with TSS_USERNAME+TSS_PASSWORD, use the platform
//	                         URL (e.g. https://dbinger.secureplatform.io) — the SDK
//	                         discovers the vault URL from it. For TSS_TOKEN, use the
//	                         vault URL directly (e.g. https://xxx.devsecretservercloud.com),
//	                         since the SDK skips discovery when a token is pre-supplied.
//	TSS_TEST_FOLDER_ID       ID of a writable folder where test secrets will be created.
//
// Auth (exactly one of):
//
//	TSS_USERNAME + TSS_PASSWORD   Credentials for username/password auth.
//	TSS_TOKEN                     A pre-fetched bearer token.
//
// Optional environment variables:
//
//	TSS_TEST_SITE_ID         Site ID (default "1").
//	TSS_TEST_TEMPLATE_ID     Template ID (default "2", the Password template, with
//	                         four fields: Resource, Username, Password, Notes).
//	TSS_TEST_SSH_TEMPLATE_ID Template ID for a Unix-SSH-family template (Machine +
//	                         Username + Public Key + Private Key + Private Key
//	                         Passphrase; Password optional or absent). SS typically
//	                         ships "Unix Account (SSH Key Rotation - No Password)"
//	                         matching this shape; look it up on your tenant via
//	                         GET /api/v1/secret-templates/<id> to confirm fields
//	                         before setting this env var. If unset,
//	                         TestAccTSSSecret_SshKeyGeneration skips.
//	TSS_TEST_MIXED_TEMPLATE_ID Template ID for a template with both a Password
//	                         field and SSH key fields (typically "Unix Account (SSH
//	                         Key Rotation)"); built-in template IDs vary across
//	                         SS versions and installations, so verify on your
//	                         tenant. If unset,
//	                         TestAccTSSSecret_SshKeyAndPasswordMixed skips.
//
// Example invocation:
//
//	TF_ACC=1 \
//	  TSS_SERVER_URL=https://your-tenant/ \
//	  TSS_USERNAME=... \
//	  TSS_PASSWORD=... \
//	  TSS_TEST_FOLDER_ID=14 \
//	  go test ./delinea/ -run TestAccTSSSecret -v
//
// Each test creates one secret and relies on the framework's automatic
// CheckDestroy path to delete it at the end of the run. If a test is
// interrupted, residual secrets may need manual cleanup in the tenant.
//
// Caveat: if ~/.terraformrc contains a dev_overrides block for "DelineaXPM/tss",
// terraform ignores the reattach env that terraform-plugin-testing relies on
// and uses the override'd binary instead — which means the tests run against
// whatever's installed at that path, not the code under `go test`. Run
// `go install ./...` first so the override'd binary reflects the current code,
// or run with TF_CLI_CONFIG_FILE pointing to a file without dev_overrides.

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"tss": providerserver.NewProtocol6WithError(New()),
}

func testAccPreCheck(t *testing.T) {
	for _, key := range []string{"TSS_SERVER_URL", "TSS_TEST_FOLDER_ID"} {
		if os.Getenv(key) == "" {
			t.Fatalf("%s must be set for acceptance tests", key)
		}
	}
	hasToken := os.Getenv("TSS_TOKEN") != ""
	hasUserPass := os.Getenv("TSS_USERNAME") != "" && os.Getenv("TSS_PASSWORD") != ""
	if !hasToken && !hasUserPass {
		t.Fatal("set TSS_TOKEN or both TSS_USERNAME and TSS_PASSWORD for acceptance tests")
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func testAccProviderBlock() string {
	if token := os.Getenv("TSS_TOKEN"); token != "" {
		return fmt.Sprintf(`
provider "tss" {
  server_url = %q
  token      = %q
}
`, os.Getenv("TSS_SERVER_URL"), token)
	}
	return fmt.Sprintf(`
provider "tss" {
  server_url = %q
  username   = %q
  password   = %q
}
`, os.Getenv("TSS_SERVER_URL"), os.Getenv("TSS_USERNAME"), os.Getenv("TSS_PASSWORD"))
}

type fieldSpec struct {
	Name              string
	ItemValue         string // if non-empty, emit itemvalue
	PasswordValue     string // if non-empty, emit password_value (write-only)
	PasswordWoVersion int    // if non-zero, emit password_wo_version
	Generate          bool   // if true, emit generate=true
}

func testAccSecretConfig(name string, fields ...fieldSpec) string {
	var fieldBlocks string
	for _, f := range fields {
		fieldBlocks += fmt.Sprintf("\n  fields {\n    fieldname = %q\n", f.Name)
		if f.ItemValue != "" {
			fieldBlocks += fmt.Sprintf("    itemvalue = %q\n", f.ItemValue)
		}
		if f.PasswordValue != "" {
			fieldBlocks += fmt.Sprintf("    password_value = %q\n", f.PasswordValue)
		}
		if f.PasswordWoVersion != 0 {
			fieldBlocks += fmt.Sprintf("    password_wo_version = %d\n", f.PasswordWoVersion)
		}
		if f.Generate {
			fieldBlocks += "    generate = true\n"
		}
		fieldBlocks += "  }"
	}
	return fmt.Sprintf(`
terraform {
  required_version = ">= 1.11.0"
}
%s
resource "tss_resource_secret" "test" {
  name             = %q
  folderid         = %q
  siteid           = %q
  secrettemplateid = %q
%s
}
`,
		testAccProviderBlock(),
		name,
		os.Getenv("TSS_TEST_FOLDER_ID"),
		envOr("TSS_TEST_SITE_ID", "1"),
		envOr("TSS_TEST_TEMPLATE_ID", "2"),
		fieldBlocks,
	)
}

// Pre-fix, this scenario produced "Provider produced inconsistent result after
// apply" with block count 2→4. Post-fix, apply exits 0 and state has exactly
// the two fields the user configured. The Password field uses write-only
// password_value; its itemvalue is null in state.
func TestAccTSSSecret_PartialFields(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-partial")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSecretConfig(name,
					fieldSpec{Name: "Username", ItemValue: "testuser"},
					fieldSpec{Name: "Password", PasswordValue: "TestPassword123!", PasswordWoVersion: 1},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.#", "2"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.0.fieldname", "Username"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.fieldname", "Password"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.0.itemvalue", "testuser"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.itemvalue", ""),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.password_wo_version", "1"),
				),
			},
		},
	})
}

// Regression guard: listing every field the template defines must still work,
// with the password field using password_value.
func TestAccTSSSecret_AllFields(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-all")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSecretConfig(name,
					fieldSpec{Name: "Resource", ItemValue: "srv-acc"},
					fieldSpec{Name: "Username", ItemValue: "testuser"},
					fieldSpec{Name: "Password", PasswordValue: "TestPassword123!", PasswordWoVersion: 1},
					fieldSpec{Name: "Notes", ItemValue: "acceptance-test"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.#", "4"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.0.fieldname", "Resource"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.3.fieldname", "Notes"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.2.itemvalue", ""),
				),
			},
		},
	})
}

// PR 2 core: the password_value supplied via write-only attribute must never
// appear in any attribute in state. Checks itemvalue is null and password_value
// is absent/empty in the serialized state.
func TestAccTSSSecret_PasswordValueNotInState(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-pw-state")
	const pw = "SuperSecret-DoNotStore-xyz9"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSecretConfig(name,
					fieldSpec{Name: "Username", ItemValue: "testuser"},
					fieldSpec{Name: "Password", PasswordValue: pw, PasswordWoVersion: 1},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.itemvalue", ""),
					resource.TestCheckResourceAttrWith("tss_resource_secret.test", "fields.1.fieldname", func(v string) error {
						if v != "Password" {
							return fmt.Errorf("expected fields.1 to be Password, got %q", v)
						}
						return nil
					}),
				),
			},
		},
	})
}

// PR 2 core: rotation via password_wo_version bump. Step 1 creates with
// version 1; step 2 bumps version to 2 with a new password_value; the plan
// must show an update (non-empty plan expected between steps), and after
// apply, state still has no password and fields.#=2.
func TestAccTSSSecret_PasswordRotation(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-rotate")
	step1 := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "InitialPassword-1", PasswordWoVersion: 1},
	)
	step2 := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "RotatedPassword-2", PasswordWoVersion: 2},
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: step1,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.password_wo_version", "1"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.itemvalue", ""),
				),
			},
			{
				Config: step2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.password_wo_version", "2"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.itemvalue", ""),
				),
			},
		},
	})
}

// PR 2: re-planning a config that uses password_value after a successful
// apply must show no changes. Protects against the "WriteOnly attribute drifts
// against null state" failure mode: the framework must treat WriteOnly
// null-in-state as a no-diff signal, not as "state missing a required value."
func TestAccTSSSecret_PasswordValueRefreshNoDrift(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-pw-no-drift")
	config := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "SteadyPassword-abc1", PasswordWoVersion: 1},
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.#", "2"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.itemvalue", ""),
				),
			},
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// PR 2: changing password_value without bumping password_wo_version must NOT
// trigger an update. password_wo_version is the explicit rotation signal;
// WriteOnly password_value changes are invisible to plan comparison.
func TestAccTSSSecret_PasswordValueChangeWithoutVersionBumpIsNoOp(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-pw-noversion")
	step1 := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "OriginalPassword-1", PasswordWoVersion: 1},
	)
	// Same password_wo_version, different password_value. Plan must show "no changes".
	step2 := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "DifferentPassword-2", PasswordWoVersion: 1},
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: step1},
			{
				Config:             step2,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// gh #110: Create with generate=true asks TSS for a password matching the
// template's password-requirement policy and uses it as the field's
// itemvalue. State has no plaintext password (same WriteOnly null behavior
// as PasswordValue). The generated password should never appear in state.
func TestAccTSSSecret_GeneratePasswordFromTemplatePolicy(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-generate")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSecretConfig(name,
					fieldSpec{Name: "Username", ItemValue: "testuser"},
					fieldSpec{Name: "Password", Generate: true, PasswordWoVersion: 1},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.itemvalue", ""),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.generate", "true"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.password_wo_version", "1"),
				),
			},
		},
	})
}

// gh #110: Bumping password_wo_version while generate=true triggers an
// Update; the provider re-calls the generate endpoint and TSS stores the
// new generated password. State retains generate=true and the new version.
func TestAccTSSSecret_GeneratePasswordRotation(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-gen-rotate")
	step1 := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", Generate: true, PasswordWoVersion: 1},
	)
	step2 := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", Generate: true, PasswordWoVersion: 2},
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: step1,
				Check:  resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.password_wo_version", "1"),
			},
			{
				Config: step2,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.password_wo_version", "2"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.itemvalue", ""),
				),
			},
		},
	})
}

// gh #110: re-applying a generate=true config without bumping
// password_wo_version must be a no-op (no API call to generate-password,
// no diff). Same property as the password_value case.
func TestAccTSSSecret_GenerateNoBumpIsNoOp(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-gen-noop")
	config := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", Generate: true, PasswordWoVersion: 1},
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: config},
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// After a partial-fields apply, re-running the same config must produce no
// plan diff. This catches the "perpetual diff" failure mode where state and
// plan disagree on block count on every subsequent run.
func TestAccTSSSecret_PartialFieldsRefreshNoDrift(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-refresh")
	config := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "TestPassword123!", PasswordWoVersion: 1},
	)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.#", "2"),
			},
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// sshSecretConfig generates HCL for a secret on a Unix-SSH-family template
// (Machine + Username + SSH key fields, optionally Password). SSH key fields
// are declared with empty itemvalue so the sshKeyFieldPlanModifier marks them
// Unknown at plan time and the server fills them in at apply (triggered by
// sshkeyargs.generatesshkeys = true).
func sshSecretConfig(name, templateID string, withPassword bool) string {
	pwBlock := ""
	if withPassword {
		pwBlock = `
  fields {
    fieldname           = "Password"
    password_value      = "MixedSshPw-uniq1"
    password_wo_version = 1
  }`
	}
	return fmt.Sprintf(`
terraform {
  required_version = ">= 1.11.0"
}
%s
resource "tss_resource_secret" "test" {
  name             = %q
  folderid         = %q
  siteid           = %q
  secrettemplateid = %q

  fields {
    fieldname = "Machine"
    itemvalue = "ssh-acc-host"
  }
  fields {
    fieldname = "Username"
    itemvalue = "testuser"
  }%s

  fields {
    fieldname = "Public Key"
  }
  fields {
    fieldname = "Private Key"
  }
  fields {
    fieldname = "Private Key Passphrase"
  }

  sshkeyargs {
    generatesshkeys    = true
    generatepassphrase = true
  }
}
`,
		testAccProviderBlock(),
		name,
		os.Getenv("TSS_TEST_FOLDER_ID"),
		envOr("TSS_TEST_SITE_ID", "1"),
		templateID,
		pwBlock,
	)
}

// Exercises the sshKeyFieldPlanModifier's "mark Unknown" path for SSH key
// generation. The user supplies no itemvalue for the SSH fields; the plan
// modifier marks them Unknown on Create; the server (triggered by
// sshkeyargs.generatesshkeys = true) generates values at apply time;
// flattenSecret stores them in state.
//
// Skipped unless TSS_TEST_SSH_TEMPLATE_ID is set — SSH-key templates are
// tenant-specific.
func TestAccTSSSecret_SshKeyGeneration(t *testing.T) {
	templateID := os.Getenv("TSS_TEST_SSH_TEMPLATE_ID")
	if templateID == "" {
		t.Skip("TSS_TEST_SSH_TEMPLATE_ID not set; skipping SSH key generation test")
	}
	name := acctest.RandomWithPrefix("tf-acc-ssh")
	config := sshSecretConfig(name, templateID, false)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Machine(0), Username(1), Public Key(2), Private Key(3), Passphrase(4)
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.#", "5"),
					resource.TestCheckResourceAttrSet("tss_resource_secret.test", "fields.2.itemvalue"),
					resource.TestCheckResourceAttrSet("tss_resource_secret.test", "fields.3.itemvalue"),
				),
			},
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}

// Exercises a template that contains both a Password field (using
// password_value) and SSH key fields (using sshkeyargs generation). Verifies
// non-interference: password_value stays out of state, SSH fields get
// generated values, re-plan shows no drift.
//
// Skipped unless TSS_TEST_MIXED_TEMPLATE_ID is set — mixed templates (e.g.
// Unix Account with SSH Key) are tenant-specific.
func TestAccTSSSecret_SshKeyAndPasswordMixed(t *testing.T) {
	templateID := os.Getenv("TSS_TEST_MIXED_TEMPLATE_ID")
	if templateID == "" {
		t.Skip("TSS_TEST_MIXED_TEMPLATE_ID not set; skipping mixed SSH+password test")
	}
	name := acctest.RandomWithPrefix("tf-acc-mixed")
	config := sshSecretConfig(name, templateID, true)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					// Machine(0), Username(1), Password(2), Public Key(3), Private Key(4), Passphrase(5)
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.#", "6"),
					// Password field's itemvalue is "" (never populated in state)
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.2.itemvalue", ""),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.2.password_wo_version", "1"),
					// SSH key fields get server-generated values
					resource.TestCheckResourceAttrSet("tss_resource_secret.test", "fields.3.itemvalue"),
					resource.TestCheckResourceAttrSet("tss_resource_secret.test", "fields.4.itemvalue"),
				),
			},
			{
				Config:             config,
				PlanOnly:           true,
				ExpectNonEmptyPlan: false,
			},
		},
	})
}
