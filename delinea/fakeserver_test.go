package delinea

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func newFakeSecretServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request) bool) *server.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/oauth2/token":
			w.Header().Set("Content-Type", "application/json")
			mustWriteResponse(w, `{"access_token":"fake-token","token_type":"bearer","expires_in":3600}`)
		case r.URL.Path == "/api/v1/healthcheck":
			w.Header().Set("Content-Type", "application/json")
			mustWriteResponse(w, `{"healthy":true}`)
		case handle != nil && handle(w, r):
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	client, err := server.New(server.Configuration{
		ServerURL:      srv.URL,
		Credentials:    server.UserCredential{Username: "u", Password: "p"},
		DisableRetries: true,
	})
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	return client
}

func mustWriteResponse(w io.Writer, value string) {
	if _, err := io.WriteString(w, value); err != nil {
		panic(err)
	}
}

func respondAccessDenied(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	mustWriteResponse(w, `{"errorCode":"API_AccessDenied","message":"Access denied"}`)
}

func respondStatus(w http.ResponseWriter, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	mustWriteResponse(w, `{"message":"error"}`)
}

func respondSecret(w http.ResponseWriter, id int, active bool) {
	w.Header().Set("Content-Type", "application/json")
	mustWriteResponse(w, fmt.Sprintf(`{"ID":%d,"Name":"s%d","Active":%t,"SecretTemplateID":2,"FolderID":7,"SiteID":1,
		"Items":[{"ItemID":1,"FieldID":7,"FieldName":"Password","Slug":"password","ItemValue":"pw-%d","IsPassword":true}]}`,
		id, id, active, id))
}

func respondTemplate(w http.ResponseWriter, id int, withFileField bool) {
	w.Header().Set("Content-Type", "application/json")
	fileField := ""
	if withFileField {
		fileField = `,{"SecretTemplateFieldID":9,"FieldSlugName":"attachment","Name":"Attachment","IsFile":true}`
	}
	mustWriteResponse(w, fmt.Sprintf(`{"ID":%d,"Name":"tmpl","Fields":[{"SecretTemplateFieldID":7,"FieldSlugName":"password","Name":"Password","IsPassword":true,"IsRequired":true}%s]}`,
		id, fileField))
}

func TestClassifyDeleteFailure_Outcomes(t *testing.T) {
	cases := []struct {
		name      string
		delErr    error
		getStatus int
		active    bool
		want      deleteOutcome
	}{
		{"unrelated failure", httpErr(500), 0, false, deleteFailed},
		{"refused: readable and active", httpErr(400), 200, true, deleteRefused},
		{"ambiguous: readable but inactive", httpErr(400), 200, false, deleteAmbiguous},
		{"ambiguous: double access-denied", httpErr(400), 400, false, deleteAmbiguous},
		{"failed: get forbidden", httpErr(400), 403, false, deleteFailed},
		{"failed: get server error", httpErr(400), 500, false, deleteFailed},
		{"ambiguous: get 404", httpErr(400), 404, false, deleteAmbiguous},
		{"ambiguous: delete 404 then access denied", httpErr(404), 400, false, deleteAmbiguous},
		{"refused: delete 404 but readable active", httpErr(404), 200, true, deleteRefused},
		{"ambiguous: delete 404, readable inactive", httpErr(404), 200, false, deleteAmbiguous},
		{"failed: delete 404 then get server error", httpErr(404), 500, false, deleteFailed},
		{"ambiguous: delete and get 404", httpErr(404), 404, false, deleteAmbiguous},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
				switch r.URL.Path {
				case "/api/v1/secrets/1":
					switch c.getStatus {
					case 200:
						respondSecret(w, 1, c.active)
					case 400:
						respondAccessDenied(w)
					default:
						respondStatus(w, c.getStatus)
					}
					return true
				}
				return false
			})
			if got := classifyDeleteFailure(context.Background(), client, 1, c.delErr); got != c.want {
				t.Errorf("classifyDeleteFailure = %v, want %v", got, c.want)
			}
		})
	}
}

func TestFetchSecrets_AbortsOnNotFound(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/api/v1/secrets/1":
			respondSecret(w, 1, true)
		case "/api/v1/secrets/2":
			respondStatus(w, http.StatusNotFound)
		case "/api/v1/secrets/3":
			respondSecret(w, 3, true)
		default:
			return false
		}
		return true
	})
	var diags diag.Diagnostics
	if _, ok := fetchSecrets(context.Background(), client, []int{1, 2, 3}, &diags); ok || diags.ErrorsCount() == 0 {
		t.Fatalf("404 must abort: ok=%v diags=%v", ok, diags)
	}
	if diags[0].Summary() != "Secret Fetch Unverified" {
		t.Fatalf("diagnostic summary = %q", diags[0].Summary())
	}
}

func TestFetchSecrets_AbortsOnBareBadRequest(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/api/v1/secrets/1" {
			respondAccessDenied(w)
			return true
		}
		return false
	})
	var diags diag.Diagnostics
	if _, ok := fetchSecrets(context.Background(), client, []int{1}, &diags); ok || diags.ErrorsCount() == 0 {
		t.Fatalf("bare HTTP 400 must abort: ok=%v diags=%v", ok, diags)
	}
	if diags[0].Summary() != "Secret Access Ambiguous" {
		t.Fatalf("diagnostic summary = %q", diags[0].Summary())
	}
}

func TestFetchSecretModels_PreservesInputPositions(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/api/v1/secrets/1":
			respondSecret(w, 1, true)
		case "/api/v1/secrets/2":
			respondSecret(w, 2, true)
		case "/api/v1/secrets/3":
			respondSecret(w, 3, true)
		default:
			return false
		}
		return true
	})
	var diags diag.Diagnostics
	ids := []types.Int64{types.Int64Value(1), types.Int64Value(2), types.Int64Value(3)}
	values, ok := fetchSecretModels(context.Background(), client, ids, "PASSWORD", &diags)
	if !ok || len(values) != 3 {
		t.Fatalf("values=%v ok=%v diags=%v", values, ok, diags)
	}
	if values[0].ID.ValueInt64() != 1 || values[0].Value.ValueString() != "pw-1" ||
		values[1].ID.ValueInt64() != 2 || values[1].Value.ValueString() != "pw-2" ||
		values[2].ID.ValueInt64() != 3 || values[2].Value.ValueString() != "pw-3" {
		t.Fatalf("positions were not preserved: %v", values)
	}
}

func TestFetchSecretModels_MissingFieldIsError(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/secrets/1" {
			return false
		}
		respondSecret(w, 1, true)
		return true
	})
	var diags diag.Diagnostics
	values, ok := fetchSecretModels(context.Background(), client, []types.Int64{types.Int64Value(1)}, "missing", &diags)
	if ok || values != nil || diags.ErrorsCount() != 1 || diags[0].Summary() != "Field Not Found" {
		t.Fatalf("values=%v ok=%v diags=%v", values, ok, diags)
	}
}

func TestAcceptanceDeletionVerifier_CorroboratesBadRequestWithSearch(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/api/v1/secrets/1":
			respondAccessDenied(w)
		case "/api/v1/secrets":
			w.Header().Set("Content-Type", "application/json")
			mustWriteResponse(w, `{"Records":[]}`)
		default:
			return false
		}
		return true
	})
	if err := testAccVerifySecretAbsent(client, 1, "unique-name"); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptanceDeleteFixture_DoesNotAcceptUncorroboratedBadRequest(t *testing.T) {
	var mu sync.Mutex
	directGets := 0
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/api/v1/secrets/1":
			if r.Method == http.MethodDelete {
				respondAccessDenied(w)
				return true
			}
			mu.Lock()
			directGets++
			first := directGets == 1
			mu.Unlock()
			if first {
				respondAccessDenied(w)
			} else {
				respondSecret(w, 1, true)
			}
		case "/api/v1/secrets":
			w.Header().Set("Content-Type", "application/json")
			mustWriteResponse(w, `{"Records":[{"ID":1,"Name":"unique-name"}]}`)
		default:
			return false
		}
		return true
	})
	err := testAccDeleteFixture(client, 1, "unique-name")
	if err == nil || !strings.Contains(err.Error(), "remains discoverable") {
		t.Fatalf("err = %v, want the secret to be reported as still discoverable", err)
	}
}

func TestAcceptanceDeletionVerifier_RejectsAmbiguousSignals(t *testing.T) {
	for _, response := range []func(http.ResponseWriter){
		func(w http.ResponseWriter) { respondStatus(w, http.StatusNotFound) },
		func(w http.ResponseWriter) { respondSecret(w, 1, false) },
	} {
		client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
			if r.URL.Path != "/api/v1/secrets/1" {
				return false
			}
			response(w)
			return true
		})
		if err := testAccVerifySecretAbsent(client, 1, "unique-name"); err == nil {
			t.Fatal("ambiguous deletion signal was accepted")
		}
	}
}

func TestFetchSecrets_AbortsOnServerError(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/api/v1/secrets/1":
			respondSecret(w, 1, true)
		case "/api/v1/secrets/2":
			respondStatus(w, http.StatusInternalServerError)
		default:
			return false
		}
		return true
	})
	var diags diag.Diagnostics
	_, ok := fetchSecrets(context.Background(), client, []int{1, 2}, &diags)
	if ok {
		t.Fatal("ok = true, want abort on a non-skippable failure")
	}
	if diags.ErrorsCount() == 0 {
		t.Error("an aborting read must carry at least one error diagnostic")
	}
}

func TestFetchSecrets_ReportsCallerCancellation(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/api/v1/secrets/1" {
			return false
		}
		respondSecret(w, 1, true)
		return true
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var diags diag.Diagnostics
	_, ok := fetchSecrets(ctx, client, []int{1}, &diags)
	if ok {
		t.Fatal("ok = true, want cancellation to abort")
	}
	if diags.ErrorsCount() != 1 || diags[0].Summary() != "Secret Read Cancelled" {
		t.Fatalf("cancellation diagnostics = %v", diags)
	}
}

func TestPopulateSecretModelValues_RejectsNilSecret(t *testing.T) {
	values := []SecretModel{{ID: types.Int64Value(7), Value: types.StringValue("")}}
	var diags diag.Diagnostics
	if populateSecretModelValues(values, []int{7}, []*server.Secret{nil}, "password", &diags) {
		t.Fatal("nil secret was accepted")
	}
	if diags.ErrorsCount() != 1 || diags[0].Summary() != "Secret Fetch Error" {
		t.Fatalf("nil-secret diagnostics = %v", diags)
	}
}

func TestFetchSecretField(t *testing.T) {
	client := newFakeSecretServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/api/v1/secrets/1":
			respondSecret(w, 1, true)
		case "/api/v1/secrets/2":
			respondStatus(w, http.StatusNotFound)
		case "/api/v1/secrets/3":
			respondStatus(w, http.StatusInternalServerError)
		case "/api/v1/secrets/4":
			respondStatus(w, http.StatusForbidden)
		case "/api/v1/secrets/5":
			respondAccessDenied(w)
		default:
			return false
		}
		return true
	})

	var diags diag.Diagnostics
	value, ok := fetchSecretField(context.Background(), client, 1, "PASSWORD", &diags)
	if !ok || value != "pw-1" {
		t.Errorf("fetch existing field: value=%q ok=%v, diags %v", value, ok, diags)
	}

	diags = nil
	if _, ok := fetchSecretField(context.Background(), client, 1, "nosuch", &diags); ok || diags.ErrorsCount() != 1 || diags[0].Summary() != "Field Not Found" {
		t.Errorf("missing field: ok=%v diags=%v", ok, diags)
	}

	diags = nil
	if _, ok := fetchSecretField(context.Background(), client, 2, "password", &diags); ok || diags[0].Summary() != "Secret Fetch Unverified" {
		t.Errorf("absent secret: ok=%v diags=%v", ok, diags)
	}

	diags = nil
	if _, ok := fetchSecretField(context.Background(), client, 3, "password", &diags); ok || diags[0].Summary() != "Secret Fetch Error" {
		t.Errorf("server failure: ok=%v diags=%v", ok, diags)
	}

	diags = nil
	if _, ok := fetchSecretField(context.Background(), client, 4, "password", &diags); ok || diags[0].Summary() != "Secret Authentication or Authorization Failed" {
		t.Errorf("forbidden secret: ok=%v diags=%v", ok, diags)
	}

	diags = nil
	if _, ok := fetchSecretField(context.Background(), client, 5, "password", &diags); ok || diags[0].Summary() != "Secret Access Ambiguous" {
		t.Errorf("bare bad request: ok=%v diags=%v", ok, diags)
	}
}
