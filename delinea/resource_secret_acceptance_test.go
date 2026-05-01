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
//
// Example invocation:
//
//	TF_ACC=1 \
//	  TSS_SERVER_URL=https://dbinger.secureplatform.io \
//	  TSS_USERNAME=... \
//	  TSS_PASSWORD=... \
//	  TSS_TEST_FOLDER_ID=104 \
//	  go test -vet=off ./delinea/ -run TestAccTSSSecret -v
//
// Each test creates one secret and relies on the framework's automatic
// CheckDestroy path to delete it at the end of the run. If a test is
// interrupted, residual secrets may need manual cleanup in the tenant.
//
// Caveat: if ~/.terraformrc contains a `dev_overrides` block for
// "DelineaXPM/tss" (common during local provider development), the tests hang.
// Dev overrides tell terraform to use an installed binary and ignore the
// reattach env terraform-plugin-testing depends on, which breaks the in-process
// provider wiring. Workarounds: either remove the dev_overrides entry, or run
// the tests with TF_CLI_CONFIG_FILE pointing to a config without dev_overrides.
// When dev overrides are active, manual reproduction via `terraform apply`
// against the installed binary is the verification path — see the repro
// sections in _ticket-700142.md and _ticket-new-inconsistent-fields.md.

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

func testAccSecretConfig(name string, fields ...[2]string) string {
	var fieldBlocks string
	for _, f := range fields {
		fieldBlocks += fmt.Sprintf(`
  fields {
    fieldname = %q
    itemvalue = %q
  }`, f[0], f[1])
	}
	return fmt.Sprintf(`%s
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
// the two fields the user configured.
func TestAccTSSSecret_PartialFields(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-partial")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSecretConfig(name,
					[2]string{"Username", "testuser"},
					[2]string{"Password", "TestPassword123!"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.#", "2"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.0.fieldname", "Username"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.fieldname", "Password"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.0.itemvalue", "testuser"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.itemvalue", "TestPassword123!"),
				),
			},
		},
	})
}

// Regression guard: listing every field the template defines must still work.
func TestAccTSSSecret_AllFields(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-all")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSecretConfig(name,
					[2]string{"Resource", "srv-acc"},
					[2]string{"Username", "testuser"},
					[2]string{"Password", "TestPassword123!"},
					[2]string{"Notes", "acceptance-test"},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.#", "4"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.0.fieldname", "Resource"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.3.fieldname", "Notes"),
				),
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
		[2]string{"Username", "testuser"},
		[2]string{"Password", "TestPassword123!"},
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
