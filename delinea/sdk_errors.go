package delinea

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/DelineaXPM/delinea-common/api"
	"github.com/DelineaXPM/tss-sdk-go/v3/server"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"golang.org/x/sync/errgroup"
)

// httpStatus extracts the status code when err is the SDK's *server.HTTPError.
func httpStatus(err error) (int, bool) {
	he, ok := errors.AsType[*server.HTTPError](err)
	if !ok {
		return 0, false
	}
	return he.StatusCode, true
}

// isServerRejection reports whether err proves Secret Server received and
// rejected the request — a non-timeout 4xx it sent back, or a failure the SDK
// classifies as configuration/auth/authorization, all of which happen before
// or instead of a committed write. Anything else (timeout, cancellation,
// connection loss) leaves a write's outcome unknown.
func isServerRejection(err error) bool {
	if status, ok := httpStatus(err); ok {
		return status >= 400 && status < 500 && status != http.StatusRequestTimeout && status != 499
	}
	return errors.Is(err, api.ErrConfig) || errors.Is(err, api.ErrAuth) || errors.Is(err, api.ErrAccessDenied)
}

// duplicateCheckHint tells the operator how to avoid duplicating a secret
// whose creation outcome is unknown.
func duplicateCheckHint(name string) string {
	return fmt.Sprintf("Before re-applying, check Secret Server for a secret named %q and delete it, or the next apply may create a duplicate.", name)
}

// deleteOutcome classifies a failed DELETE after disambiguating the server's
// deliberately ambiguous response with a follow-up GET.
type deleteOutcome int

const (
	// deleteFailed: an unrelated failure (auth, network, server error).
	deleteFailed deleteOutcome = iota
	// deleteRefused: the secret is demonstrably alive (readable and active);
	// Secret Server refused the DELETE.
	deleteRefused
	// deleteAmbiguous: the DELETE and the follow-up GET both returned the
	// ambiguous access-denied response — already-deleted and
	// permission-denied are indistinguishable by the server's design.
	deleteAmbiguous
)

func classifyDeleteFailure(ctx context.Context, client *server.Server, id int, delErr error) deleteOutcome {
	if !isSecretGone(delErr) {
		return deleteFailed
	}
	secret, getErr := client.SecretContext(ctx, id)
	switch {
	case getErr == nil && secret.Active:
		return deleteRefused
	case getErr == nil:
		// Active=false is not proof of deletion: active is configurable, so a
		// failed DELETE of an intentionally inactive secret has the same shape
		// as a readable recycled secret.
		return deleteAmbiguous
	case isSecretNotFound(getErr):
		// A 404 from the verification GET can be the SDK's attachment
		// sub-request rather than the secret itself. Terraform's stored template
		// ID cannot disambiguate it because the secret may have changed template
		// outside Terraform.
		return deleteAmbiguous
	case isSecretGone(getErr):
		return deleteAmbiguous
	default:
		return deleteFailed
	}
}

// deleteRefusedDetail and deleteAmbiguousDetail single-source the delete
// diagnostic prose for both resources that classify delete failures.
func deleteRefusedDetail(id int, delErr error) string {
	return fmt.Sprintf("Secret Server refused to delete secret %d (%s) but the secret still exists and is active — it may be checked out, DoubleLocked, or the account may lack delete permission.", id, delErr)
}

func deleteAmbiguousDetail(id int, delErr error, stateRmHint bool) string {
	detail := fmt.Sprintf("Deleting secret %d failed (%s) and reading it back could not establish whether it still exists — the responses are consistent with both an already-deleted secret and missing permission.", id, delErr)
	if stateRmHint {
		detail += " If the secret is confirmed gone, remove it with `terraform state rm`; otherwise restore view and delete permission and re-run."
	}
	return detail
}

// Classic Secret Server reports both deleted and never-existent secrets as
// HTTP 400 with errorCode API_AccessDenied (deliberate anti-enumeration), and
// the SDK withholds response bodies for the secrets resource, so only the
// status code is observable; 404 comes from Platform vaults and proxies.
// Verified against a live Secret Server Cloud tenant: GET and DELETE of a
// deleted secret return 400, as does GET of an id that never existed.

// Managed-secret reads and failed-delete verification fail closed on 404
// because the SDK returns the same status when an attachment download fails
// after a successful secret GET. The classic 400 ambiguity is also never proof
// of absence.

// isSecretNotFound reports whether err carries HTTP 404. Callers must decide
// whether an attachment sub-request could have produced it.
func isSecretNotFound(err error) bool {
	status, ok := httpStatus(err)
	return ok && status == http.StatusNotFound
}

func isAuthenticationFailure(err error) bool {
	if errors.Is(err, api.ErrAuth) || errors.Is(err, api.ErrAccessDenied) {
		return true
	}
	status, ok := httpStatus(err)
	return ok && (status == http.StatusUnauthorized || status == http.StatusForbidden)
}

func authenticationFailureDetail(action string, id int, err error) string {
	return fmt.Sprintf("Secret Server rejected the request to %s secret %d: %s. The configured token may be expired or invalid, or the identity may lack permission. Refresh the token or restore the required permission and retry; the response does not establish that the secret is absent.", action, id, err)
}

// fetchSlots bounds concurrent fan-out from multi-secret data sources and
// ephemeral resources across the provider process. The single-secret paths do
// not introduce their own fan-out and therefore do not acquire a slot.
var fetchSlots = make(chan struct{}, 8)

// addSecretFetchErrorDiag reports a fatal fetch failure with the
// classification shared by the single- and multi-secret paths.
func addSecretFetchErrorDiag(diags *diag.Diagnostics, id int, err error) {
	if isAuthenticationFailure(err) {
		diags.AddError("Secret Authentication or Authorization Failed", authenticationFailureDetail("read", id, err))
		return
	}
	if status, hasStatus := httpStatus(err); hasStatus {
		switch status {
		case http.StatusBadRequest:
			diags.AddError("Secret Access Ambiguous",
				fmt.Sprintf("Secret %d returned HTTP 400. Classic Secret Server uses this anti-enumeration response for both deleted and inaccessible secrets, while a proxy, WAF, or incompatible request can return the same status: %s. Verify the ID and read permission; if the secret was deleted, remove its ID from the configuration.", id, err))
			return
		case http.StatusNotFound:
			diags.AddError("Secret Fetch Unverified",
				fmt.Sprintf("Secret %d returned HTTP 404, but the SDK cannot distinguish a missing secret from a missing attachment on a live secret: %s. Verify the secret and its attachments; if the secret was deleted, remove its ID from the configuration.", id, err))
			return
		case http.StatusTooManyRequests:
			diags.AddError("Secret Server Rate Limited",
				fmt.Sprintf("Secret Server rate-limited the read of secret %d after the SDK's retries: %s. Reduce the number of secrets read at once or the run's overall parallelism.", id, err))
			return
		}
	}
	diags.AddError("Secret Fetch Error", fmt.Sprintf("Failed to fetch secret with ID %d: %s", id, err))
}

// fetchSecrets retrieves ids concurrently (bounded provider-wide), preserving
// order. Any failure is fatal because the SDK does not expose whether a 404
// came from the primary secret GET or a later attachment download.
func fetchSecrets(ctx context.Context, client *server.Server, ids []int, diags *diag.Diagnostics) (secrets []*server.Secret, ok bool) {
	secrets = make([]*server.Secret, len(ids))
	fetchErrs := make([]error, len(ids))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(cap(fetchSlots))
	for i, id := range ids {
		g.Go(func() error {
			select {
			case fetchSlots <- struct{}{}:
				defer func() { <-fetchSlots }()
			case <-gctx.Done():
				fetchErrs[i] = gctx.Err()
				return nil
			}
			secrets[i], fetchErrs[i] = client.SecretContext(gctx, id)
			if err := fetchErrs[i]; err != nil {
				// Returning the fatal error cancels gctx so queued and
				// in-flight fetches stop instead of riding out their full
				// retry/timeout budgets on an already-decided outcome.
				return err
			}
			return nil
		})
	}
	_ = g.Wait()
	ok = true
	emitted := false
	cancelNoted := false
	for i, err := range fetchErrs {
		if err == nil {
			continue
		}
		ok = false
		if errors.Is(err, context.Canceled) {
			// One summary when the whole run was cancelled; silence for the
			// collateral of our own first-fatal-error cancellation, whose
			// causal error carries the diagnostic.
			if ctx.Err() != nil && !cancelNoted {
				cancelNoted = true
				emitted = true
				diags.AddError("Secret Read Cancelled", "The operation was cancelled before all secrets were fetched.")
			}
			continue
		}
		emitted = true
		addSecretFetchErrorDiag(diags, ids[i], err)
	}
	if !ok && !emitted {
		// Every fatal error was a suppressed cancellation (e.g. the causal
		// error itself wrapped context.Canceled); never fail silently.
		diags.AddError("Secret Fetch Error", "The read failed before all secrets were fetched.")
	}
	return secrets, ok
}

// fetchSecretField fetches one secret and extracts a field with the same error
// classification as the multi-secret path.
func fetchSecretField(ctx context.Context, client *server.Server, id int, field string, diags *diag.Diagnostics) (value string, ok bool) {
	secret, err := client.SecretContext(ctx, id)
	if err != nil {
		addSecretFetchErrorDiag(diags, id, err)
		return "", false
	}
	value, matches := secretFieldValue(secret, field)
	if matches == 0 {
		diags.AddError("Field Not Found", fmt.Sprintf("The secret with ID %d does not contain the field %q", id, field))
		return "", false
	}
	if matches > 1 {
		diags.AddError("Field Ambiguous", fmt.Sprintf("The field %q matches more than one field name or slug on secret %d; use a unique field alias or change the template.", field, id))
		return "", false
	}
	return value, true
}

func secretFieldValue(secret *server.Secret, name string) (string, int) {
	value := ""
	matches := 0
	for _, field := range secret.Fields {
		if fieldNameMatches(name, field.FieldName, field.Slug) {
			value = field.ItemValue
			matches++
		}
	}
	return value, matches
}

func fieldNameMatches(name, canonical, slug string) bool {
	return strings.EqualFold(name, canonical) || slug != "" && strings.EqualFold(name, slug)
}

type SecretModel struct {
	ID    types.Int64  `tfsdk:"id"`
	Value types.String `tfsdk:"value"`
}

func populateSecretModelValues(values []SecretModel, ids []int, secrets []*server.Secret, field string, diags *diag.Diagnostics) bool {
	if len(values) != len(ids) || len(secrets) != len(ids) {
		diags.AddError("Secret Fetch Error", "The secret fetch returned an unexpected number of results.")
		return false
	}
	for i, secret := range secrets {
		if secret == nil {
			diags.AddError("Secret Fetch Error", fmt.Sprintf("The secret fetch for ID %d returned no secret data.", ids[i]))
			continue
		}
		value, matches := secretFieldValue(secret, field)
		if matches == 0 {
			diags.AddError("Field Not Found", fmt.Sprintf("The secret with ID %d does not contain the field %q.", ids[i], field))
			continue
		}
		if matches > 1 {
			diags.AddError("Field Ambiguous", fmt.Sprintf("The field %q matches more than one field name or slug on secret %d; use a unique field alias or change the template.", field, ids[i]))
			continue
		}
		values[i].Value = types.StringValue(value)
	}
	return !diags.HasError()
}

// fetchSecretModels fetches ids concurrently and returns one result per input
// id in the same position. A missing field or secret-fetch failure aborts the
// read.
func fetchSecretModels(ctx context.Context, client *server.Server, ids []types.Int64, field string, diags *diag.Diagnostics) (values []SecretModel, ok bool) {
	intIDs := make([]int, len(ids))
	values = make([]SecretModel, len(ids))
	for i, id := range ids {
		converted, err := toPositiveServerInt(id.ValueInt64(), fmt.Sprintf("ids[%d]", i))
		if err != nil {
			diags.AddError("Invalid Secret ID", err.Error())
			return nil, false
		}
		intIDs[i] = converted
		values[i] = SecretModel{ID: id, Value: types.StringValue("")}
	}
	secrets, ok := fetchSecrets(ctx, client, intIDs, diags)
	if !ok {
		return nil, false
	}
	if !populateSecretModelValues(values, intIDs, secrets, field, diags) {
		return nil, false
	}
	return values, true
}

// isSecretGone reports whether err is consistent with the secret being
// absent: 404 or classic Secret Server's ambiguous 400. Because missing
// permission can have the same statuses, use this only where absence is the
// expected outcome; auth, network, and server failures never match.
func isSecretGone(err error) bool {
	if isSecretNotFound(err) {
		return true
	}
	status, ok := httpStatus(err)
	return ok && status == http.StatusBadRequest
}
