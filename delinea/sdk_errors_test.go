package delinea

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/DelineaXPM/delinea-common/api"
	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func httpErr(status int) error {
	return &server.HTTPError{StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status))}
}

func TestAbsencePredicates_NotFoundIsGone(t *testing.T) {
	cases := []struct {
		status         int
		notFound, gone bool
	}{
		{http.StatusNotFound, true, true},
		{http.StatusBadRequest, false, true},
		{http.StatusForbidden, false, false},
		{http.StatusUnauthorized, false, false},
		{http.StatusTooManyRequests, false, false},
		{http.StatusInternalServerError, false, false},
		{http.StatusBadGateway, false, false},
	}
	for _, c := range cases {
		err := httpErr(c.status)
		if got := isSecretNotFound(err); got != c.notFound {
			t.Errorf("isSecretNotFound(%d) = %v, want %v", c.status, got, c.notFound)
		}
		if got := isSecretGone(err); got != c.gone {
			t.Errorf("isSecretGone(%d) = %v, want %v", c.status, got, c.gone)
		}
		if c.notFound && !c.gone {
			t.Errorf("status %d breaks the subset relation notFound ⊆ gone", c.status)
		}
	}
}

func TestAbsencePredicates_WrappedHTTPErrorMatches(t *testing.T) {
	err := fmt.Errorf("retrieving secret: %w", httpErr(http.StatusNotFound))
	if !isSecretNotFound(err) {
		t.Error("a wrapped 404 HTTPError must still classify as not found")
	}
}

func TestAbsencePredicates_NonHTTPErrorsNeverMatch(t *testing.T) {
	for _, err := range []error{
		errors.New("boom"),
		context.Canceled,
		fmt.Errorf("wrapped: %w", context.DeadlineExceeded),
		fmt.Errorf("auth: %w", api.ErrAccessDenied),
	} {
		if isSecretNotFound(err) || isSecretGone(err) {
			t.Errorf("non-HTTPError %v must never classify as absent", err)
		}
	}
}

func TestIsServerRejection(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{httpErr(http.StatusBadRequest), true},
		{httpErr(http.StatusNotFound), true},
		{httpErr(http.StatusRequestTimeout), false},
		{httpErr(499), false},
		{httpErr(http.StatusInternalServerError), false},
		{httpErr(http.StatusMovedPermanently), false},
		{fmt.Errorf("init: %w", api.ErrConfig), true},
		{fmt.Errorf("grant: %w", api.ErrAuth), true},
		{fmt.Errorf("read: %w", api.ErrAccessDenied), true},
		{errors.New("connection reset"), false},
		{context.Canceled, false},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), false},
	}
	for _, c := range cases {
		if got := isServerRejection(c.err); got != c.want {
			t.Errorf("isServerRejection(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestAddSecretFetchErrorDiag_RateLimitedGetsGuidance(t *testing.T) {
	var diags diag.Diagnostics
	addSecretFetchErrorDiag(&diags, 7, httpErr(http.StatusTooManyRequests))
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}
	if diags[0].Summary() != "Secret Server Rate Limited" {
		t.Errorf("summary = %q, want the rate-limit classification", diags[0].Summary())
	}
}

func TestAddSecretFetchErrorDiag_OtherFailuresAreGeneric(t *testing.T) {
	var diags diag.Diagnostics
	addSecretFetchErrorDiag(&diags, 7, httpErr(http.StatusInternalServerError))
	if len(diags) != 1 {
		t.Fatalf("got %d diagnostics, want 1", len(diags))
	}
	if diags[0].Summary() != "Secret Fetch Error" {
		t.Errorf("summary = %q, want the generic classification", diags[0].Summary())
	}
}

func TestAddSecretFetchErrorDiag_AmbiguousStatusesGetGuidance(t *testing.T) {
	cases := []struct {
		status  int
		summary string
		detail  string
	}{
		{http.StatusBadRequest, "Secret Access Ambiguous", "remove its ID"},
		{http.StatusNotFound, "Secret Fetch Unverified", "missing attachment"},
	}
	for _, c := range cases {
		var diags diag.Diagnostics
		addSecretFetchErrorDiag(&diags, 7, httpErr(c.status))
		if len(diags) != 1 || diags[0].Summary() != c.summary || !strings.Contains(diags[0].Detail(), c.detail) {
			t.Errorf("status %d diagnostics = %v", c.status, diags)
		}
	}
}

func TestAddSecretFetchErrorDiag_ForbiddenNamesAuthentication(t *testing.T) {
	var diags diag.Diagnostics
	addSecretFetchErrorDiag(&diags, 7, httpErr(http.StatusForbidden))
	if len(diags) != 1 || diags[0].Summary() != "Secret Authentication or Authorization Failed" ||
		!strings.Contains(diags[0].Detail(), "expired or invalid") || strings.Contains(diags[0].Detail(), "state rm") {
		t.Fatalf("diagnostics = %v", diags)
	}
}

func TestAuthenticationFailureIncludesSDKSentinels(t *testing.T) {
	for _, err := range []error{
		api.ErrAuth,
		fmt.Errorf("token grant: %w", api.ErrAuth),
		api.ErrAccessDenied,
		fmt.Errorf("read: %w", api.ErrAccessDenied),
		httpErr(http.StatusUnauthorized),
		httpErr(http.StatusForbidden),
	} {
		if !isAuthenticationFailure(err) {
			t.Errorf("isAuthenticationFailure(%v) = false", err)
		}
	}
	for _, err := range []error{api.ErrConfig, httpErr(http.StatusBadRequest), errors.New("connection reset")} {
		if isAuthenticationFailure(err) {
			t.Errorf("isAuthenticationFailure(%v) = true", err)
		}
	}
}

func TestAddSecretFetchErrorDiag_SDKAuthenticationSentinelNamesAuthentication(t *testing.T) {
	var diags diag.Diagnostics
	addSecretFetchErrorDiag(&diags, 7, fmt.Errorf("token grant: %w", api.ErrAuth))
	if len(diags) != 1 || diags[0].Summary() != "Secret Authentication or Authorization Failed" ||
		!strings.Contains(diags[0].Detail(), "expired or invalid") {
		t.Fatalf("diagnostics = %v", diags)
	}
}

func TestSecretFieldValue_EmptyNameDoesNotMatchEmptySlug(t *testing.T) {
	secret := &server.Secret{Fields: []server.SecretField{{FieldName: "Password", ItemValue: "value"}}}
	if value, matches := secretFieldValue(secret, ""); matches != 0 || value != "" {
		t.Fatalf("secretFieldValue returned %q with %d matches", value, matches)
	}
}

func TestSecretFieldValue_RejectsNameSlugCollision(t *testing.T) {
	secret := &server.Secret{Fields: []server.SecretField{
		{FieldName: "Account", Slug: "account-name", ItemValue: "first"},
		{FieldName: "Account Name", Slug: "account", ItemValue: "second"},
	}}
	if _, matches := secretFieldValue(secret, "account"); matches != 2 {
		t.Fatalf("secretFieldValue found %d matches, want 2", matches)
	}
}

func TestFetchSecretModels_RejectsOversizedIDBeforeFetch(t *testing.T) {
	var diags diag.Diagnostics
	values, ok := fetchSecretModels(context.Background(), nil, []types.Int64{types.Int64Value(1<<32 + 1)}, "password", &diags)
	if ok || values != nil || !diags.HasError() || diags[0].Summary() != "Invalid Secret ID" {
		t.Fatalf("values=%v ok=%v diagnostics=%v", values, ok, diags)
	}
}
