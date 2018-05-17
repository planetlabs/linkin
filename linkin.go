// Package linkin converts between linkerd style and Zipkin style HTTP headers.
//
// Zipkin is a popular distributed tracing system, allowing requests to be
// traced through a distributed system. A request is broken into 'spans', each
// representing one part of the greater request path. Software that wishes to
// leverage Zipkin must be instrumented to do so, for example via Opentracing -
// https://github.com/openzipkin/zipkin-go-opentracing.
//
// linkerd is a popular service mesh. One of linkerd's selling points is that it
// provides Zipkin request tracing 'for free'. Software need not be 'fully'
// instrumented, and instead need only copy linkerd's l5d-ctx-* HTTP headers
// from incoming HTTP requests to any outgoing HTTP requests they spawn.
//
// Unfortunately linkerd uses a non-standard HTTP header to pass trace metadata
// between systems. This makes it difficult for instrumented code to emit spans
// representing non-HTTP requests such as database calls.
//
// linkin provides a poor man's linkerd compatibility layer at the
// instrumentation level by translating the l5d-ctx-trace HTTP header provided
// by linkerd to the X-B3-* HTTP headers understood by zipkin-go-opentracing,
// and vice versa.
//
// Example usage for server side:
//
//     carrier := opentracing.HTTPHeadersCarrier(linkin.FromL5D(httpReq.Header))
//     clientContext, err := tracer.Extract(opentracing.HTTPHeaders, carrier)
//
// Example usage for client side:
//
//     carrier := opentracing.HTTPHeadersCarrier(linkin.ToL5d(httpReq.Header))
//     err := tracer.Inject(
//         span.Context(),
//         opentracing.HTTPHeaders,
//         carrier)
package linkin

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
)

const (
	l5dHeaderTrace = "l5d-ctx-trace"

	// TODO(negz): Respect the L5D sampled header.
	zipkinHeaderTraceID      = "X-B3-TraceId"
	zipkinHeaderSpanID       = "X-B3-SpanId"
	zipkinHeaderParentSpanID = "X-B3-ParentSpanId"
	zipkinHeaderSampled      = "X-B3-Sampled"
	zipkinHeaderFlags        = "X-B3-Flags"
)

var l5dTraceFlags = []byte{0, 0, 0, 0, 0, 0, 0, 6}

// FromL5D decodes linkerd's l5d-ctx-trace header and populates Zipkin's X-B3-*
// headers. This allows the Zipkin Opentracing tracer to extract trace metadata
// from linkerd, and thus create associated child spans.
func FromL5D(headers http.Header) {
	b, err := base64.StdEncoding.DecodeString(headers.Get(l5dHeaderTrace))
	if err != nil {
		return
	}

	if len(b) != 32 && len(b) != 40 {
		return
	}

	if len(b) == 40 {
		headers.Set(zipkinHeaderTraceID, fmt.Sprintf("%016x%016x", b[32:], b[16:24]))
	} else {
		headers.Set(zipkinHeaderTraceID, fmt.Sprintf("%016x", b[16:24]))
	}
	headers.Set(zipkinHeaderSpanID, hex.EncodeToString(b[0:8]))
	parentID := hex.EncodeToString(b[8:16])
	if parentID != "" {
		headers.Set(zipkinHeaderParentSpanID, parentID)
	}
}

// ToL5D encodes Zipkin's X-B3-* headers as linkerd's l5d-ctx-trace header. This
// allows HTTP request spans sent via linkerd to be children of a Zipkin
// Opentracing span.
func ToL5D(headers http.Header) { // nolint:gocyclo
	traceID, err := hex.DecodeString(headers.Get(zipkinHeaderTraceID))
	if err != nil {
		return
	}
	spanID, err := hex.DecodeString(headers.Get(zipkinHeaderSpanID))
	if err != nil {
		return
	}
	parentID, err := hex.DecodeString(headers.Get(zipkinHeaderParentSpanID))
	if err != nil {
		return
	}

	if !(len(traceID) == 8 || len(traceID) == 16) || len(spanID) != 8 {
		return
	}

	l5dctx := make([]byte, 24+len(traceID))
	for i, b := range spanID {
		l5dctx[i] = b
	}
	if len(parentID) == 8 {
		for i, b := range parentID {
			l5dctx[8+i] = b
		}
	}
	for i, b := range traceID[:8] {
		l5dctx[16+i] = b
	}
	for i, b := range l5dTraceFlags {
		l5dctx[24+i] = b
	}
	if len(traceID) == 16 {
		for i, b := range traceID {
			l5dctx[32+i] = b
		}
	}

	headers.Set(l5dHeaderTrace, base64.StdEncoding.EncodeToString(l5dctx))
}
