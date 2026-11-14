package tracing

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	common_tracing "github.com/soulteary/tracing-kit/v2"
	otlp "github.com/soulteary/tracing-kit/v2/otlp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// defaultTracesPath is the OTLP/HTTP path spans are posted to when the
// endpoint names none.
const defaultTracesPath = "/v1/traces"

// Endpoint is a parsed OTLP_ENDPOINT: where spans are sent and how.
type Endpoint struct {
	// Host is the collector's host, with the port when one was given.
	Host string
	// Path is the request path.
	Path string
	// Insecure sends spans over plain HTTP. It is false only for an
	// https:// endpoint.
	Insecure bool
}

// String returns the URL spans are posted to. It holds no credentials: an
// endpoint with user information is refused by ParseEndpoint.
func (e Endpoint) String() string {
	scheme := "https"
	if e.Insecure {
		scheme = "http"
	}
	return scheme + "://" + e.Host + e.Path
}

// ParseEndpoint accepts the two forms OTLP_ENDPOINT may take:
//
//   - host:port (or a bare host), exported over plain HTTP to /v1/traces;
//   - http://host[:port][/path] or https://host[:port][/path]. https uses TLS
//     with the system roots. A path other than "/" is used as the request
//     path as it stands; without one spans go to /v1/traces.
//
// Anything else is an error. The error never contains the value, which could
// hold credentials.
func ParseEndpoint(raw string) (Endpoint, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return Endpoint{}, errors.New("endpoint is empty")
	}
	if !strings.Contains(value, "://") {
		if err := validateHostPort(value); err != nil {
			return Endpoint{}, err
		}
		return Endpoint{Host: value, Path: defaultTracesPath, Insecure: true}, nil
	}

	u, err := url.Parse(value)
	if err != nil {
		return Endpoint{}, errors.New("endpoint is not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Endpoint{}, errors.New("endpoint URL scheme must be http or https")
	}
	if u.User != nil {
		return Endpoint{}, errors.New("endpoint URL must not contain user information")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return Endpoint{}, errors.New("endpoint URL must not contain a query or fragment")
	}
	if u.Host == "" || u.Hostname() == "" {
		return Endpoint{}, errors.New("endpoint URL has no host")
	}
	if err := validateHostPort(u.Host); err != nil {
		return Endpoint{}, err
	}
	path := u.EscapedPath()
	if path == "" || path == "/" {
		path = defaultTracesPath
	}
	return Endpoint{Host: u.Host, Path: path, Insecure: u.Scheme == "http"}, nil
}

// validateHostPort checks a host with an optional port.
func validateHostPort(value string) error {
	if strings.ContainsAny(value, "/?#@ \t\r\n") {
		return errors.New("endpoint must be host:port or an http(s):// URL")
	}
	host, port := value, ""
	if h, p, err := net.SplitHostPort(value); err == nil {
		if p == "" {
			return errors.New("endpoint has a colon but no port")
		}
		host, port = h, p
	} else if strings.Contains(value, ":") && !strings.HasPrefix(value, "[") {
		// A colon without a usable port, or an unbracketed IPv6 address.
		return errors.New("endpoint host and port are not valid")
	}
	if strings.Trim(host, "[]") == "" {
		return errors.New("endpoint has no host")
	}
	if port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return errors.New("endpoint port must be a number from 1 to 65535")
		}
	}
	return nil
}

// InitTracer builds the OTLP/HTTP exporter for the endpoint, installs a
// tracer provider that exports through it and sets the W3C propagators. It
// samples and describes the service as tracing-kit's InitTracer does; it
// exists because that function takes a bare host:port only, always exports
// in plain text and cannot set a request path.
//
// On error nothing is installed.
func InitTracer(serviceName, serviceVersion string, endpoint Endpoint) (*sdktrace.TracerProvider, error) {
	if endpoint.Host == "" {
		return nil, errors.New("endpoint has no host")
	}
	cfg := otlp.Config{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		Endpoint:       endpoint.Host,
		Insecure:       endpoint.Insecure,
		SampleRatio:    1,
	}
	ctx := context.Background()
	res, err := cfg.Resource(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint.Host),
		otlptracehttp.WithURLPath(endpoint.Path),
	}
	if endpoint.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptrace.New(ctx, otlptracehttp.NewClient(opts...))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(cfg.Sampler()),
	)
	common_tracing.Install(tp, serviceName)
	common_tracing.SetDefaultPropagator()
	return tp, nil
}
