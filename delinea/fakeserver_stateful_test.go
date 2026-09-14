package delinea

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type statefulFakeServer struct {
	t   *testing.T
	srv *httptest.Server

	mu           sync.Mutex
	secrets      map[int]map[string]any
	puts         []map[string]any
	requests     []string
	nextID       int
	failGets     int
	deleteStatus int
	onCreate     func(secret map[string]any)
	onPut        func(updated, current map[string]any) bool
}

const fakeTemplateJSON = `{"ID":2,"Name":"tmpl","Fields":[
  {"SecretTemplateFieldID":7,"FieldSlugName":"password","Name":"Password","IsPassword":true,"IsRequired":true},
  {"SecretTemplateFieldID":8,"FieldSlugName":"username","Name":"Username"},
  {"SecretTemplateFieldID":9,"FieldSlugName":"notes","Name":"Notes"}]}`

const fakeReplacementTemplateJSON = `{"ID":3,"Name":"replacement","Fields":[
  {"SecretTemplateFieldID":18,"FieldSlugName":"username","Name":"Username"}]}`

var fakeSecretPath = regexp.MustCompile(`^/api/v1/secrets/(\d+)$`)

func newStatefulFakeServer(t *testing.T) *statefulFakeServer {
	t.Helper()
	f := &statefulFakeServer{t: t, secrets: map[int]map[string]any{}, nextID: 100}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handle))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *statefulFakeServer) handle(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.Method+" "+r.URL.Path)
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/oauth2/token":
		mustWriteResponse(w, `{"access_token":"fake-token","token_type":"bearer","expires_in":3600}`)
	case r.URL.Path == "/api/v1/healthcheck":
		mustWriteResponse(w, `{"healthy":true}`)
	case r.URL.Path == "/api/v1/secret-templates/2":
		mustWriteResponse(w, fakeTemplateJSON)
	case r.URL.Path == "/api/v1/secret-templates/3":
		mustWriteResponse(w, fakeReplacementTemplateJSON)
	case strings.TrimSuffix(r.URL.Path, "/") == "/api/v1/secrets" && r.Method == http.MethodPost:
		secret := decodeSecret(r.Body)
		f.nextID++
		secret["ID"] = f.nextID
		assignItemIDs(secret, f.nextID)
		if f.onCreate != nil {
			f.onCreate(secret)
		}
		f.secrets[f.nextID] = secret
		_ = json.NewEncoder(w).Encode(secret)
	case fakeSecretPath.MatchString(r.URL.Path):
		id, _ := strconv.Atoi(fakeSecretPath.FindStringSubmatch(r.URL.Path)[1])
		secret, ok := f.secrets[id]
		switch {
		case r.Method == http.MethodGet && f.failGets > 0:
			f.failGets--
			respondStatus(w, http.StatusForbidden)
		case r.Method == http.MethodDelete && f.deleteStatus != 0:
			if f.deleteStatus == http.StatusBadRequest {
				respondAccessDenied(w)
			} else {
				respondStatus(w, f.deleteStatus)
			}
		case !ok:
			respondAccessDenied(w)
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(secret)
		case r.Method == http.MethodPut:
			updated := decodeSecret(r.Body)
			f.puts = append(f.puts, cloneSecret(updated))
			updated["ID"] = id
			if f.onPut == nil || !f.onPut(updated, secret) {
				mergeOmittedItems(updated, secret)
			}
			assignItemIDs(updated, id)
			f.secrets[id] = updated
			_ = json.NewEncoder(w).Encode(updated)
		case r.Method == http.MethodDelete:
			delete(f.secrets, id)
			mustWriteResponse(w, `{}`)
		default:
			http.NotFound(w, r)
		}
	default:
		http.NotFound(w, r)
	}
}

func decodeSecret(body io.Reader) map[string]any {
	raw, _ := io.ReadAll(body)
	secret := map[string]any{}
	_ = json.Unmarshal(raw, &secret)
	return secret
}

func cloneSecret(secret map[string]any) map[string]any {
	raw, _ := json.Marshal(secret)
	cloned := map[string]any{}
	_ = json.Unmarshal(raw, &cloned)
	return cloned
}

func mergeOmittedItems(updated, current map[string]any) {
	currentItems, _ := current["Items"].([]any)
	newItems, _ := updated["Items"].([]any)
	present := map[any]bool{}
	for _, item := range newItems {
		present[item.(map[string]any)["FieldID"]] = true
	}
	for _, item := range currentItems {
		if !present[item.(map[string]any)["FieldID"]] {
			newItems = append(newItems, item)
		}
	}
	updated["Items"] = newItems
}

func assignItemIDs(secret map[string]any, secretID int) {
	items, _ := secret["Items"].([]any)
	for i, item := range items {
		item.(map[string]any)["ItemID"] = secretID*10 + i
	}
}

func (f *statefulFakeServer) lastPut() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.puts) == 0 {
		f.t.Fatalf("no PUT observed; requests: %v", f.requests)
	}
	return f.puts[len(f.puts)-1]
}

func (f *statefulFakeServer) set(fn func(f *statefulFakeServer)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *statefulFakeServer) providerBlock() string {
	return fmt.Sprintf(`
provider "tss" {
  server_url = %q
  username   = "u"
  password   = "p"
}
`, f.srv.URL)
}

func (f *statefulFakeServer) secretConfig(name, fieldsHCL string) string {
	return f.secretConfigWithTemplate(name, "2", fieldsHCL)
}

func (f *statefulFakeServer) secretConfigWithTemplate(name, templateID, fieldsHCL string) string {
	return f.providerBlock() + fmt.Sprintf(`
resource "tss_resource_secret" "s" {
  name             = %q
  folderid         = "7"
  siteid           = "1"
  secrettemplateid = %q
%s
}
`, name, templateID, fieldsHCL)
}

const fakePasswordField = `
  fields {
    fieldname           = "Password"
    password_value      = "pw1"
    password_wo_version = 1
  }`

func TestUnitSecretResource_UpdatePreservesUnconfiguredServerSettings(t *testing.T) {
	f := newStatefulFakeServer(t)
	f.onCreate = func(secret map[string]any) {
		secret["CheckOutEnabled"] = true
		secret["RequiresComment"] = true
		secret["EnableInheritPermissions"] = true
		secret["LauncherConnectAsSecretID"] = 55
		secret["CheckOutIntervalMinutes"] = 30
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.secretConfig("first", fakePasswordField),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.s", "checkoutenabled", "true"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "requirescomment", "true"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "enableinheritpermissions", "true"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "launcherconnectassecretid", "55"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "checkoutintervalminutes", "30"),
				),
			},
			{
				Config: f.secretConfig("renamed", fakePasswordField),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.s", "name", "renamed"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "checkoutenabled", "true"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "requirescomment", "true"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "enableinheritpermissions", "true"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "launcherconnectassecretid", "55"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "checkoutintervalminutes", "30"),
				),
			},
		},
	})
	put := f.lastPut()
	for key, want := range map[string]any{
		"Name":                      "renamed",
		"CheckOutEnabled":           true,
		"RequiresComment":           true,
		"EnableInheritPermissions":  true,
		"LauncherConnectAsSecretID": float64(55),
		"CheckOutIntervalMinutes":   float64(30),
	} {
		if put[key] != want {
			t.Errorf("PUT %s = %v, want %v", key, put[key], want)
		}
	}
}

func TestUnitSecretResource_ReorderedFieldsPreserveValuesByIdentity(t *testing.T) {
	f := newStatefulFakeServer(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: f.secretConfig("ordered", `
  fields {
    fieldname = "Username"
    itemvalue = "alice"
  }
  fields {
    fieldname = "Notes"
    itemvalue = "memo"
  }`)},
			{
				Config: f.secretConfig("reordered", `
  fields {
    fieldname = "Notes"
  }
  fields {
    fieldname = "Username"
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.0.fieldname", "Notes"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.0.itemvalue", "memo"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.1.fieldname", "Username"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.1.itemvalue", "alice"),
				),
			},
		},
	})
	put := f.lastPut()
	items, _ := put["Items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["ItemValue"] != "memo" || items[1].(map[string]any)["ItemValue"] != "alice" {
		t.Fatalf("reordered PUT items = %#v", items)
	}
}

func TestUnitSecretResource_RepointedFieldDoesNotInheritValueByPosition(t *testing.T) {
	f := newStatefulFakeServer(t)
	f.onCreate = func(secret map[string]any) {
		items, _ := secret["Items"].([]any)
		secret["Items"] = append(items, map[string]any{"FieldID": 9, "FieldName": "Notes", "Slug": "notes", "ItemValue": "server-note"})
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: f.secretConfig("before", `
  fields {
    fieldname = "Username"
    itemvalue = "alice"
  }`)},
			{
				Config: f.secretConfig("after", `
  fields {
    fieldname = "Notes"
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.0.fieldname", "Notes"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.0.itemvalue", "server-note"),
				),
			},
		},
	})
	put := f.lastPut()
	items, _ := put["Items"].([]any)
	if len(items) != 0 {
		t.Fatalf("repointed PUT items = %#v, want no write for an omitted value", items)
	}
}

func TestUnitSecretResource_TemplateChangeDoesNotInheritSameNamedFieldValue(t *testing.T) {
	f := newStatefulFakeServer(t)
	f.onPut = func(updated, _ map[string]any) bool {
		if updated["SecretTemplateID"] != float64(3) {
			return false
		}
		updated["Items"] = []any{map[string]any{
			"FieldID":   float64(18),
			"FieldName": "Username",
			"Slug":      "username",
			"ItemValue": "replacement-value",
		}}
		return true
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: f.secretConfig("before", `
  fields {
    fieldname = "Username"
    itemvalue = "old-value"
  }`)},
			{
				Config: f.secretConfigWithTemplate("after", "3", `
  fields {
    fieldname = "Username"
  }`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.s", "secrettemplateid", "3"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.0.itemvalue", "replacement-value"),
				),
			},
		},
	})
	put := f.lastPut()
	items, _ := put["Items"].([]any)
	if len(items) != 0 {
		t.Fatalf("template-change PUT items = %#v, want no inherited field value", items)
	}
}

func TestUnitSecretResource_ExplicitSettingsAreSentOnUpdate(t *testing.T) {
	f := newStatefulFakeServer(t)
	f.onCreate = func(secret map[string]any) { secret["CheckOutEnabled"] = true }
	explicit := fakePasswordField + `
  checkoutenabled = false
  requirescomment = true`
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{Config: f.secretConfig("first", fakePasswordField)},
			{
				Config: f.secretConfig("first", explicit),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.s", "checkoutenabled", "false"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "requirescomment", "true"),
				),
			},
		},
	})
	put := f.lastPut()
	if put["CheckOutEnabled"] != false || put["RequiresComment"] != true {
		t.Errorf("PUT CheckOutEnabled=%v RequiresComment=%v, want false/true", put["CheckOutEnabled"], put["RequiresComment"])
	}
}

func TestUnitSecretResource_ItemValueOnPasswordFieldIsRejected(t *testing.T) {
	f := newStatefulFakeServer(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.secretConfig("legacy", `
  fields {
    fieldname = "Password"
    itemvalue = "legacy-pw"
  }`),
				ExpectError: regexp.MustCompile(`(?s)password fields.*do not accept itemvalue`),
			},
		},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, request := range f.requests {
		if strings.HasPrefix(request, "POST /api/v1/secrets") {
			t.Fatalf("a rejected configuration must not create a secret; requests: %v", f.requests)
		}
	}
}

func TestUnitSecretResource_EmptyItemValueOnPasswordFieldIsRejected(t *testing.T) {
	f := newStatefulFakeServer(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.secretConfig("legacy-empty", `
  fields {
    fieldname = "Password"
    itemvalue = ""
  }`),
				ExpectError: regexp.MustCompile(`(?s)password fields.*do not accept itemvalue`),
			},
		},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, request := range f.requests {
		if strings.HasPrefix(request, "POST /api/v1/secrets") {
			t.Fatalf("a rejected configuration must not create a secret; requests: %v", f.requests)
		}
	}
}

func TestUnitSecretResource_PartialCreateIsRecordedAsTainted(t *testing.T) {
	f := newStatefulFakeServer(t)
	f.failGets = 1
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      f.secretConfig("partial", fakePasswordField),
				ExpectError: regexp.MustCompile(`Secret Partially Created`),
			},
		},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.secrets) != 0 {
		t.Fatalf("the tainted secret was not destroyed at the end of the test; remaining %v", f.secrets)
	}
	var deletes int
	for _, request := range f.requests {
		if strings.HasPrefix(request, "DELETE ") {
			deletes++
		}
	}
	if deletes != 1 {
		t.Fatalf("DELETE count = %d, want exactly one for the tainted secret; requests: %v", deletes, f.requests)
	}
}

func TestUnitSecretResource_CreateWithoutActiveIsActive(t *testing.T) {
	f := newStatefulFakeServer(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.secretConfig("active-default", fakePasswordField),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_resource_secret.s", "active", "true"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.#", "1"),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.0.itemvalue", ""),
					resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.0.slug", "password"),
					testAccCheckResourceStateExcludes("tss_resource_secret.s", "pw1"),
				),
			},
		},
	})
}

func TestUnitSecretResource_CreateWithoutFieldsDoesNotAdoptTemplateFields(t *testing.T) {
	f := newStatefulFakeServer(t)
	f.onCreate = func(secret map[string]any) {
		secret["Items"] = []any{
			map[string]any{"FieldID": 7, "FieldName": "Password", "Slug": "password", "ItemValue": "generated", "IsPassword": true},
			map[string]any{"FieldID": 8, "FieldName": "Username", "Slug": "username", "ItemValue": "default-user"},
		}
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: f.secretConfig("no-fields", ""),
			Check:  resource.TestCheckResourceAttr("tss_resource_secret.s", "fields.#", "0"),
		}},
	})
}

func TestUnitSecretResource_DuplicateFieldAliasesAreRejected(t *testing.T) {
	f := newStatefulFakeServer(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: f.secretConfig("duplicates", `
  fields {
    fieldname           = "Password"
    password_value      = "first"
    password_wo_version = 1
  }
  fields {
    fieldname           = "password"
    password_value      = "second"
    password_wo_version = 1
  }`),
			ExpectError: regexp.MustCompile(`both resolve\s+to template field`),
		}},
	})
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, request := range f.requests {
		if strings.HasPrefix(request, "POST /api/v1/secrets") {
			t.Fatalf("a duplicate field configuration must not create a secret; requests: %v", f.requests)
		}
	}
}

func TestUnitSecretResource_PartialSSHKeyArgsUseFalseDefaults(t *testing.T) {
	f := newStatefulFakeServer(t)
	config := f.secretConfig("ssh-defaults", fakePasswordField+`
  sshkeyargs {
    generatesshkeys = true
  }`)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: config,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("tss_resource_secret.s", "sshkeyargs.generatepassphrase", "false"),
				resource.TestCheckResourceAttr("tss_resource_secret.s", "sshkeyargs.generatesshkeys", "true"),
			),
		}},
	})
}

func TestUnitSecretDeletion_CreateDeletesAndRecordsID(t *testing.T) {
	f := newStatefulFakeServer(t)
	f.set(func(f *statefulFakeServer) {
		f.secrets[500] = map[string]any{"ID": 500, "Name": "doomed", "Active": true, "SecretTemplateID": 2, "FolderID": 7, "SiteID": 1, "Items": []any{}}
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.providerBlock() + `
resource "tss_secret_deletion" "d" {
  secret_id = 500
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("tss_secret_deletion.d", "id", "secret_500"),
					func(*terraform.State) error {
						f.mu.Lock()
						defer f.mu.Unlock()
						if _, exists := f.secrets[500]; exists {
							return fmt.Errorf("secret 500 still exists after tss_secret_deletion applied")
						}
						return nil
					},
				),
			},
			{
				Config: f.providerBlock() + `
resource "tss_secret_deletion" "d" {
  secret_id = 500
}`,
				PlanOnly: true,
			},
		},
	})
}

func TestUnitSecretDeletion_RefusedAndAmbiguousDeletes(t *testing.T) {
	cases := map[string]struct {
		keepSecret bool
		active     bool
		wantError  string
	}{
		"refused: secret readable and active": {keepSecret: true, active: true, wantError: "Secret Deletion Refused"},
		"ambiguous: secret readable inactive": {keepSecret: true, active: false, wantError: "Secret Deletion Unverified"},
		"ambiguous: secret unreadable":        {keepSecret: false, wantError: "Secret Deletion Unverified"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newStatefulFakeServer(t)
			f.set(func(f *statefulFakeServer) {
				f.deleteStatus = http.StatusBadRequest
				if c.keepSecret {
					f.secrets[501] = map[string]any{"ID": 501, "Name": "stuck", "Active": c.active, "SecretTemplateID": 2, "FolderID": 7, "SiteID": 1, "Items": []any{}}
				}
			})
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: f.providerBlock() + `
resource "tss_secret_deletion" "d" {
  secret_id = 501
}`,
						ExpectError: regexp.MustCompile(c.wantError),
					},
				},
			})
		})
	}
}

func TestUnitSecretDataSource_ReadsField(t *testing.T) {
	f := newStatefulFakeServer(t)
	f.set(func(f *statefulFakeServer) {
		f.secrets[600] = map[string]any{"ID": 600, "Name": "ds", "Active": true, "SecretTemplateID": 2, "FolderID": 7, "SiteID": 1,
			"Items": []any{map[string]any{"ItemID": 1, "FieldID": 7, "FieldName": "Password", "Slug": "password", "ItemValue": "s3cret", "IsPassword": true}}}
	})
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: f.providerBlock() + `
data "tss_secret" "one" {
  id    = "600"
  field = "PASSWORD"
}
data "tss_secrets" "many" {
  ids   = [600]
  field = "password"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.tss_secret.one", "value", "s3cret"),
					resource.TestCheckResourceAttr("data.tss_secrets.many", "secrets.#", "1"),
					resource.TestCheckResourceAttr("data.tss_secrets.many", "secrets.0.id", "600"),
					resource.TestCheckResourceAttr("data.tss_secrets.many", "secrets.0.value", "s3cret"),
				),
			},
			{
				Config: f.providerBlock() + `
data "tss_secret" "missing" {
  id    = "601"
  field = "password"
}`,
				ExpectError: regexp.MustCompile(`Secret Access Ambiguous`),
			},
			{
				Config: f.providerBlock() + `
data "tss_secret" "nofield" {
  id    = "600"
  field = "nosuch"
}`,
				ExpectError: regexp.MustCompile(`Field Not Found`),
			},
		},
	})
}

func TestUnitProvider_ConfigureRejectsBadURL(t *testing.T) {
	f := newStatefulFakeServer(t)
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
provider "tss" {
  server_url = "http://secret-server.example.com/SecretServer"
  username   = "u"
  password   = "p"
}
data "tss_secret" "x" {
  id    = "1"
  field = "password"
}`,
				ExpectError: regexp.MustCompile(`allow_insecure_http`),
			},
			{
				Config: f.providerBlock() + `
data "tss_secret" "x" {
  id    = "not-a-number"
  field = "password"
}`,
				ExpectError: regexp.MustCompile(`Secret ID must be an integer`),
			},
		},
	})
}

func TestUnitEphemeralResources_ReadFields(t *testing.T) {
	f := newStatefulFakeServer(t)
	f.set(func(f *statefulFakeServer) {
		for _, id := range []int{700, 701} {
			f.secrets[id] = map[string]any{"ID": id, "Name": fmt.Sprintf("e%d", id), "Active": true, "SecretTemplateID": 2, "FolderID": 7, "SiteID": 1,
				"Items": []any{map[string]any{"ItemID": 1, "FieldID": 7, "FieldName": "Password", "Slug": "password", "ItemValue": fmt.Sprintf("pw-%d", id), "IsPassword": true}}}
		}
	})
	factories := map[string]func() (tfprotov6.ProviderServer, error){
		"echo": echoprovider.NewProviderServer(),
	}
	for key, factory := range testAccProtoV6ProviderFactories {
		factories[key] = factory
	}
	resource.UnitTest(t, resource.TestCase{
		ProtoV6ProviderFactories: factories,
		Steps: []resource.TestStep{
			{
				Config: f.providerBlock() + `
ephemeral "tss_secrets" "many" {
  ids   = [700, 999]
  field = "password"
}
provider "echo" {
  data = join(",", [for s in ephemeral.tss_secrets.many.secrets : s.value])
}
resource "echo" "pw" {}
`,
				ExpectError: regexp.MustCompile(`Secret Access Ambiguous`),
			},
			{
				Config: f.providerBlock() + `
ephemeral "tss_secret" "one" {
  id    = "700"
  field = "password"
}
ephemeral "tss_secrets" "many" {
  ids   = [700, 701]
  field = "PASSWORD"
}
provider "echo" {
  data = join(",", concat([ephemeral.tss_secret.one.value], [for s in ephemeral.tss_secrets.many.secrets : s.value]))
}
resource "echo" "pw" {}
`,
				Check: resource.TestCheckResourceAttr("echo.pw", "data", "pw-700,pw-700,pw-701"),
			},
		},
	})
}
