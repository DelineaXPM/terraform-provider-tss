package delinea

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"tss": providerserver.NewProtocol6WithError(New()),
}

var (
	testAccLiveClientOnce sync.Once
	testAccLiveClient     *server.Server
	testAccLiveClientErr  error
)

func testAccClient() (*server.Server, error) {
	testAccLiveClientOnce.Do(func() {
		allowInsecureHTTP := false
		if raw := os.Getenv("TSS_ALLOW_INSECURE_HTTP"); raw != "" {
			allowInsecureHTTP, testAccLiveClientErr = strconv.ParseBool(raw)
			if testAccLiveClientErr != nil {
				testAccLiveClientErr = fmt.Errorf("parsing TSS_ALLOW_INSECURE_HTTP: %w", testAccLiveClientErr)
				return
			}
		}
		testAccLiveClient, testAccLiveClientErr = server.New(server.Configuration{
			ServerURL:         os.Getenv("TSS_SERVER_URL"),
			AllowInsecureHTTP: allowInsecureHTTP,
			Credentials: server.UserCredential{
				Username: os.Getenv("TSS_USERNAME"),
				Password: os.Getenv("TSS_PASSWORD"),
				Token:    os.Getenv("TSS_TOKEN"),
				Domain:   os.Getenv("TSS_DOMAIN"),
			},
		})
	})
	return testAccLiveClient, testAccLiveClientErr
}

func testAccVerifySecretAbsent(client *server.Server, id int, name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		secret, err := client.SecretContext(ctx, id)
		if err != nil {
			if status, ok := httpStatus(err); ok {
				switch status {
				case http.StatusBadRequest:
					if name == "" {
						return fmt.Errorf("verifying deletion of secret %d requires its prior name to corroborate the ambiguous HTTP 400 response", id)
					}
					matches, searchErr := client.SecretsContext(ctx, name, "Name")
					if searchErr != nil {
						return fmt.Errorf("corroborating deletion of secret %d with search: %w", id, searchErr)
					}
					for _, match := range matches {
						if match.ID == id {
							return fmt.Errorf("secret %d remains discoverable after deletion", id)
						}
					}
					return nil
				case http.StatusNotFound:
					return fmt.Errorf("verifying deletion of secret %d returned ambiguous HTTP 404; the SDK cannot distinguish a missing secret from a missing attachment: %w", id, err)
				}
			}
			return fmt.Errorf("verifying deletion of secret %d: %w", id, err)
		}
		if !secret.Active {
			return fmt.Errorf("secret %d remains readable with active=false, which does not prove deletion", id)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("secret %d still exists and is active after deletion verification: %w", id, ctx.Err())
		case <-ticker.C:
		}
	}
}

func testAccCheckDestroy(state *terraform.State) error {
	client, err := testAccClient()
	if err != nil {
		return err
	}
	for address, resourceState := range state.RootModule().Resources {
		if resourceState.Type != "tss_resource_secret" || resourceState.Primary == nil {
			continue
		}
		rawID, ok := resourceState.Primary.Attributes["id"]
		if !ok {
			return fmt.Errorf("%s has no id in pre-destroy state", address)
		}
		id, err := strconv.Atoi(rawID)
		if err != nil {
			return fmt.Errorf("%s has invalid id %q: %w", address, rawID, err)
		}
		name, ok := resourceState.Primary.Attributes["name"]
		if !ok || name == "" {
			return fmt.Errorf("%s has no name in pre-destroy state", address)
		}
		if err := testAccVerifySecretAbsent(client, id, name); err != nil {
			return fmt.Errorf("%s: %w", address, err)
		}
	}
	return nil
}

func testAccCheckResourceStateExcludes(address, value string) resource.TestCheckFunc {
	return func(state *terraform.State) error {
		resourceState, ok := state.RootModule().Resources[address]
		if !ok || resourceState.Primary == nil {
			return fmt.Errorf("%s has no state", address)
		}
		for attribute, stored := range resourceState.Primary.Attributes {
			if strings.Contains(stored, value) {
				return fmt.Errorf("%s.%s contains the write-only value", address, attribute)
			}
		}
		return nil
	}
}

func testAccResourceTest(t *testing.T, testCase resource.TestCase) {
	t.Helper()
	if testCase.CheckDestroy == nil {
		testCase.CheckDestroy = testAccCheckDestroy
	}
	resource.Test(t, testCase)
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
	return fmt.Sprintf(`
provider "tss" {
  server_url = %q
}
`, os.Getenv("TSS_SERVER_URL"))
}

func TestTestAccProviderBlockDoesNotEmbedCredentials(t *testing.T) {
	t.Setenv("TSS_SERVER_URL", "https://secret-server.example.test")
	t.Setenv("TSS_USERNAME", "acceptance-user")
	t.Setenv("TSS_PASSWORD", "acceptance-password")
	t.Setenv("TSS_TOKEN", "acceptance-token")
	block := testAccProviderBlock()
	for _, credential := range []string{"acceptance-user", "acceptance-password", "acceptance-token"} {
		if strings.Contains(block, credential) {
			t.Fatalf("provider block contains credential %q", credential)
		}
	}
}

type fieldSpec struct {
	Name              string
	ItemValue         string
	PasswordValue     string
	PasswordWoVersion int
	Generate          bool
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

func testAccAdditionalPasswordSecretConfig(resourceName, name, password string) string {
	return fmt.Sprintf(`
resource "tss_resource_secret" %q {
  name             = %q
  folderid         = %q
  siteid           = %q
  secrettemplateid = %q
  fields {
    fieldname = "Username"
    itemvalue = "testuser"
  }
  fields {
    fieldname           = "Password"
    password_value      = %q
    password_wo_version = 1
  }
}
`, resourceName, name, os.Getenv("TSS_TEST_FOLDER_ID"), envOr("TSS_TEST_SITE_ID", "1"), envOr("TSS_TEST_TEMPLATE_ID", "2"), password)
}

func TestAccTSSSecret_PartialFields(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-partial")
	testAccResourceTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSecretConfig(name,
					fieldSpec{Name: "Username", ItemValue: "testuser"},
					fieldSpec{Name: "Password", PasswordValue: "TestPassword123!", PasswordWoVersion: 1},
				),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "active", "true"),
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

func TestAccTSSSecret_AllFields(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-all")
	testAccResourceTest(t, resource.TestCase{
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

func TestAccTSSSecret_PasswordValueNotInState(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-pw-state")
	const pw = "SuperSecret-DoNotStore-xyz9"
	testAccResourceTest(t, resource.TestCase{
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
					testAccCheckResourceStateExcludes("tss_resource_secret.test", pw),
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

func TestAccTSSSecret_PasswordRotation(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-rotate")
	configFor := func(password string, version int) string {
		return testAccSecretConfig(name,
			fieldSpec{Name: "Username", ItemValue: "testuser"},
			fieldSpec{Name: "Password", PasswordValue: password, PasswordWoVersion: version},
		) + `
data "tss_secret" "pw" {
  id         = tostring(tss_resource_secret.test.id)
  field      = "password"
  depends_on = [tss_resource_secret.test]
}
`
	}
	testAccResourceTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configFor("InitialPassword-1", 1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.password_wo_version", "1"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.itemvalue", ""),
					resource.TestCheckResourceAttr("data.tss_secret.pw", "value", "InitialPassword-1"),
				),
			},
			{
				Config: configFor("RotatedPassword-2", 2),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.password_wo_version", "2"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.1.itemvalue", ""),
					resource.TestCheckResourceAttr("data.tss_secret.pw", "value", "RotatedPassword-2"),
				),
			},
		},
	})
}

func TestAccTSSSecret_PasswordValueRefreshNoDrift(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-pw-no-drift")
	config := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "SteadyPassword-abc1", PasswordWoVersion: 1},
	)
	testAccResourceTest(t, resource.TestCase{
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

func TestAccTSSSecret_PasswordValueChangeWithoutVersionBumpIsNoOp(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-pw-noversion")
	step1 := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "OriginalPassword-1", PasswordWoVersion: 1},
	)
	step2 := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "DifferentPassword-2", PasswordWoVersion: 1},
	)
	testAccResourceTest(t, resource.TestCase{
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

func TestAccTSSSecret_PasswordValueChangeWithUnrelatedUpdateDoesNotRotate(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-pw-unrelated")
	configFor := func(secretName, password string) string {
		return testAccSecretConfig(secretName,
			fieldSpec{Name: "Username", ItemValue: "testuser"},
			fieldSpec{Name: "Password", PasswordValue: password, PasswordWoVersion: 1},
		) + `
data "tss_secret" "pw" {
  id         = tostring(tss_resource_secret.test.id)
  field      = "password"
  depends_on = [tss_resource_secret.test]
}
`
	}
	testAccResourceTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configFor(name, "OriginalPassword-1"),
				Check:  resource.TestCheckResourceAttr("data.tss_secret.pw", "value", "OriginalPassword-1"),
			},
			{
				Config: configFor(name+"-renamed", "DifferentPassword-2"),
				Check:  resource.TestCheckResourceAttr("data.tss_secret.pw", "value", "OriginalPassword-1"),
			},
		},
	})
}

func TestAccTSSSecret_GeneratePasswordFromTemplatePolicy(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-generate")
	testAccResourceTest(t, resource.TestCase{
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
	testAccResourceTest(t, resource.TestCase{
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

func TestAccTSSSecret_GenerateNoBumpIsNoOp(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-gen-noop")
	config := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", Generate: true, PasswordWoVersion: 1},
	)
	testAccResourceTest(t, resource.TestCase{
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

func TestAccTSSSecret_PartialFieldsRefreshNoDrift(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-refresh")
	config := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "TestPassword123!", PasswordWoVersion: 1},
	)
	testAccResourceTest(t, resource.TestCase{
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

func testAccOptionalTemplateID(t *testing.T, key string) string {
	t.Helper()
	templateID := os.Getenv(key)
	if templateID != "" {
		return templateID
	}
	if os.Getenv("TF_ACC_REQUIRE_ALL_TESTS") != "" {
		t.Fatalf("%s must be set when TF_ACC_REQUIRE_ALL_TESTS is set", key)
	}
	t.Skipf("%s not set; skipping", key)
	return ""
}

func TestAccTSSSecret_SshKeyGeneration(t *testing.T) {
	templateID := testAccOptionalTemplateID(t, "TSS_TEST_SSH_TEMPLATE_ID")
	name := acctest.RandomWithPrefix("tf-acc-ssh")
	config := sshSecretConfig(name, templateID, false)
	testAccResourceTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
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

func TestAccTSSSecret_SshKeyAndPasswordMixed(t *testing.T) {
	templateID := testAccOptionalTemplateID(t, "TSS_TEST_MIXED_TEMPLATE_ID")
	name := acctest.RandomWithPrefix("tf-acc-mixed")
	config := sshSecretConfig(name, templateID, true)
	testAccResourceTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.#", "6"),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.2.itemvalue", ""),
					resource.TestCheckResourceAttr("tss_resource_secret.test", "fields.2.password_wo_version", "1"),
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

func TestAccTSSSecret_GenerateUnrelatedUpdateNoRotate(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-gen-stable")
	configFor := func(secretName string, woVersion int) string {
		return testAccSecretConfig(secretName,
			fieldSpec{Name: "Username", ItemValue: "testuser"},
			fieldSpec{Name: "Password", Generate: true, PasswordWoVersion: woVersion},
		) + `
data "tss_secret" "pw" {
  id         = tostring(tss_resource_secret.test.id)
  field      = "password"
  depends_on = [tss_resource_secret.test]
}
`
	}
	explicitConfigFor := func(secretName, password string, woVersion int) string {
		return testAccSecretConfig(secretName,
			fieldSpec{Name: "Username", ItemValue: "testuser"},
			fieldSpec{Name: "Password", PasswordValue: password, PasswordWoVersion: woVersion},
		) + `
data "tss_secret" "pw" {
  id         = tostring(tss_resource_secret.test.id)
  field      = "password"
  depends_on = [tss_resource_secret.test]
}
`
	}
	var initial string
	explicit := "Exp1icitAfterGenerated!"
	testAccResourceTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configFor(name, 1),
				Check: resource.TestCheckResourceAttrWith("data.tss_secret.pw", "value", func(v string) error {
					if v == "" {
						return fmt.Errorf("generated password is empty")
					}
					initial = v
					return nil
				}),
			},
			{
				Config: configFor(name+"-renamed", 1),
				Check: resource.TestCheckResourceAttrWith("data.tss_secret.pw", "value", func(v string) error {
					if v != initial {
						return fmt.Errorf("renaming the secret rotated the generated password")
					}
					return nil
				}),
			},
			{
				Config: explicitConfigFor(name+"-renamed", explicit, 1),
				Check:  resource.TestCheckResourceAttr("data.tss_secret.pw", "value", explicit),
			},
			{
				Config: configFor(name+"-renamed", 2),
				Check: resource.TestCheckResourceAttrWith("data.tss_secret.pw", "value", func(v string) error {
					if v == initial {
						return fmt.Errorf("password_wo_version bump did not rotate the password")
					}
					return nil
				}),
			},
		},
	})
}

func TestAccTSSSecretEphemeral_ReadsPassword(t *testing.T) {
	name := acctest.RandomWithPrefix("tf-acc-ephemeral")
	config := testAccSecretConfig(name,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: "Eph3meral!pw1", PasswordWoVersion: 1},
	) + `
ephemeral "tss_secret" "pw" {
  id    = tostring(tss_resource_secret.test.id)
  field = "password"
}
provider "echo" {
  data = ephemeral.tss_secret.pw.value
}
resource "echo" "pw" {}
`
	factories := map[string]func() (tfprotov6.ProviderServer, error){
		"echo": echoprovider.NewProviderServer(),
	}
	for k, v := range testAccProtoV6ProviderFactories {
		factories[k] = v
	}
	testAccResourceTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("echo.pw", "data", "Eph3meral!pw1"),
			},
		},
	})
}

func TestAccTSSSecretsDataSource_ReadsPasswords(t *testing.T) {
	firstName := acctest.RandomWithPrefix("tf-acc-multi-data-a")
	secondName := acctest.RandomWithPrefix("tf-acc-multi-data-b")
	const firstPassword = "MultiData!pw1"
	const secondPassword = "MultiData!pw2"
	config := testAccSecretConfig(firstName,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: firstPassword, PasswordWoVersion: 1},
	) + testAccAdditionalPasswordSecretConfig("second", secondName, secondPassword) + `
data "tss_secrets" "pw" {
  ids        = [tss_resource_secret.test.id, tss_resource_secret.second.id]
  field      = "PASSWORD"
  depends_on = [tss_resource_secret.test, tss_resource_secret.second]
}
`
	testAccResourceTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.tss_secrets.pw", "secrets.#", "2"),
					resource.TestCheckResourceAttr("data.tss_secrets.pw", "secrets.0.value", firstPassword),
					resource.TestCheckResourceAttr("data.tss_secrets.pw", "secrets.1.value", secondPassword),
				),
			},
		},
	})
}

func TestAccTSSSecretsEphemeral_ReadsPasswords(t *testing.T) {
	firstName := acctest.RandomWithPrefix("tf-acc-multi-ephemeral-a")
	secondName := acctest.RandomWithPrefix("tf-acc-multi-ephemeral-b")
	const firstPassword = "MultiEph3meral!pw1"
	const secondPassword = "MultiEph3meral!pw2"
	config := testAccSecretConfig(firstName,
		fieldSpec{Name: "Username", ItemValue: "testuser"},
		fieldSpec{Name: "Password", PasswordValue: firstPassword, PasswordWoVersion: 1},
	) + testAccAdditionalPasswordSecretConfig("second", secondName, secondPassword) + `
ephemeral "tss_secrets" "pw" {
  ids   = [tss_resource_secret.test.id, tss_resource_secret.second.id]
  field = "PASSWORD"
}
provider "echo" {
  data = join(",", [for secret in ephemeral.tss_secrets.pw.secrets : secret.value])
}
resource "echo" "pw" {}
`
	factories := map[string]func() (tfprotov6.ProviderServer, error){
		"echo": echoprovider.NewProviderServer(),
	}
	for key, factory := range testAccProtoV6ProviderFactories {
		factories[key] = factory
	}
	testAccResourceTest(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) },
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  resource.TestCheckResourceAttr("echo.pw", "data", firstPassword+","+secondPassword),
			},
		},
	})
}

func testAccCreateSecret(t *testing.T, client *server.Server, name string) int {
	t.Helper()
	plan := SecretResourceState{
		Name:             types.StringValue(name),
		FolderID:         types.StringValue(os.Getenv("TSS_TEST_FOLDER_ID")),
		SiteID:           types.StringValue(envOr("TSS_TEST_SITE_ID", "1")),
		SecretTemplateID: types.StringValue(envOr("TSS_TEST_TEMPLATE_ID", "2")),
		Active:           types.BoolValue(true),
		Fields: []SecretField{
			{FieldName: types.StringValue("Username"), ItemValue: types.StringValue("testuser")},
			{FieldName: types.StringValue("Password"), PasswordValue: types.StringValue("DeleteMe!pw1")},
		},
	}
	prepared, err := (&TSSSecretResource{}).getSecretData(context.Background(), &plan, client)
	if err != nil {
		t.Fatalf("preparing deletion fixture: %v", err)
	}
	created, err := client.CreateSecretContext(context.Background(), *prepared)
	if err != nil {
		if partial, ok := errors.AsType[*server.PartialWriteError](err); ok && partial.SecretID > 0 {
			testAccRegisterFixtureCleanup(t, client, partial.SecretID, name)
		}
		t.Fatalf("creating deletion fixture: %v", err)
	}
	testAccRegisterFixtureCleanup(t, client, created.ID, name)
	return created.ID
}

func testAccRegisterFixtureCleanup(t *testing.T, client *server.Server, id int, name string) {
	t.Helper()
	t.Cleanup(func() {
		if err := testAccDeleteFixture(client, id, name); err != nil {
			t.Errorf("cleaning deletion fixture %d: %v", id, err)
		}
	})
}

func testAccDeleteFixture(client *server.Server, id int, name string) error {
	err := client.DeleteSecretContext(context.Background(), id)
	if err == nil {
		return nil
	}
	if isSecretGone(err) {
		return testAccVerifySecretAbsent(client, id, name)
	}
	return err
}

func TestAccTSSSecretDeletion_DeletesSecret(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC is not set")
	}
	testAccPreCheck(t)
	client, err := testAccClient()
	if err != nil {
		t.Fatal(err)
	}
	secretName := acctest.RandomWithPrefix("tf-acc-delete")
	secretID := testAccCreateSecret(t, client, secretName)
	config := fmt.Sprintf(`
terraform {
  required_version = ">= 1.11.0"
}
%s
resource "tss_secret_deletion" "test" {
  secret_id = %d
}
`, testAccProviderBlock(), secretID)
	testAccResourceTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_secret_deletion.test", "id", fmt.Sprintf("secret_%d", secretID)),
					func(*terraform.State) error {
						return testAccVerifySecretAbsent(client, secretID, secretName)
					},
				),
			},
		},
	})
}

func TestAccTSSSecretDeletion_ChangingIDDeletesReplacementTarget(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("TF_ACC is not set")
	}
	testAccPreCheck(t)
	client, err := testAccClient()
	if err != nil {
		t.Fatal(err)
	}
	firstName := acctest.RandomWithPrefix("tf-acc-delete-replace-a")
	secondName := acctest.RandomWithPrefix("tf-acc-delete-replace-b")
	firstID := testAccCreateSecret(t, client, firstName)
	secondID := testAccCreateSecret(t, client, secondName)
	configFor := func(id int) string {
		return fmt.Sprintf(`
terraform {
  required_version = ">= 1.11.0"
}
%s
resource "tss_secret_deletion" "test" {
  secret_id = %d
}
`, testAccProviderBlock(), id)
	}
	testAccResourceTest(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configFor(firstID),
				Check: func(*terraform.State) error {
					return testAccVerifySecretAbsent(client, firstID, firstName)
				},
			},
			{
				Config: configFor(secondID),
				Check: func(*terraform.State) error {
					return testAccVerifySecretAbsent(client, secondID, secondName)
				},
			},
		},
	})
}
