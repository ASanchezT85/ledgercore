// Package obs centralizes observability bootstrap for all services.
//
// Today it is a deliberate no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set,
// and even then the OpenTelemetry wiring is intentionally NOT implemented yet:
// pulling the OTel SDK would add a large dependency tree to libs/go, and we
// want services to depend on obs.Setup from day one so enabling tracing later
// is a change in exactly one place.
package obs

import (
	"context"
	"log/slog"
	"os"
)

// Setup initializes observability for a service and returns a shutdown
// function that must be deferred by the caller.
//
// Behavior:
//   - OTEL_EXPORTER_OTLP_ENDPOINT unset/empty: returns a no-op shutdown.
//   - OTEL_EXPORTER_OTLP_ENDPOINT set: still a no-op for now (see TODO), but
//     it logs that the endpoint was detected so operators are not surprised.
//
// TODO(obs): wire the OpenTelemetry SDK here — trace + metric providers with
// OTLP exporters honoring the standard OTEL_* env vars, resource attributes
// (service.name = serviceName), and propagation. Return a shutdown that
// flushes both providers. Do NOT add the OTel dependencies to libs/go until
// this TODO is executed, to keep the dependency tree clean.
func Setup(ctx context.Context, serviceName string) (func(), error) {
	_ = ctx

	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		return func() {}, nil
	}

	slog.Info("otel endpoint detected but instrumentation is not wired yet; running with no-op telemetry",
		"service", serviceName,
		"endpoint", endpoint,
	)
	return func() {}, nil
}
