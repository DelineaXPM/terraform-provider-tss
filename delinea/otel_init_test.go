package delinea

import "os"

// Disable the OpenTelemetry trace exporter in tests. Terraform 1.12+ attempts
// to export traces to localhost:4317 on every CLI invocation and waits ~10
// seconds for the connection to time out when no endpoint is listening.
// terraform-plugin-testing (used by the acceptance tests added in a later
// commit) spawns several terraform subprocesses per TestAcc step, and those
// timeouts compound into minutes — enough to blow the default test timeout.
//
// terraform-plugin-testing (or one of its deps) sets OTEL_TRACES_EXPORTER=otlp
// by the time this init() runs, so the override must be unconditional rather
// than gated on an empty-env check. Lives in its own file so the fix is
// independent of the acceptance test file and can land as its own commit.
func init() {
	os.Setenv("OTEL_TRACES_EXPORTER", "none")
	os.Setenv("OTEL_SDK_DISABLED", "true")
}
