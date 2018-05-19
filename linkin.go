// Package linkin provides linkerd trace propagation for Opencensus.
//
// Opencesus is a single distribution of libraries that automatically collects
// traces and metrics from your app, displays them locally, and sends them to
// any analysis tool. Opencensus supports the Zipkin request tracing system.
//
// Zipkin is a popular distributed tracing system, allowing requests to be
// traced through a distributed system. A request is broken into 'spans', each
// representing one part of the greater request path. Software that wishes to
// leverage Zipkin must be instrumented to do so (for example via Opencensus).
//
// linkerd is a popular service mesh. One of linkerd's selling points is that it
// provides Zipkin request tracing 'for free'. Software need not be 'fully'
// instrumented, and instead need only copy linkerd's l5d-ctx-* HTTP headers
// from incoming HTTP requests to any outgoing HTTP requests they spawn.
//
// Unfortunately while linkerd emits traces to Zipkin, it propagates trace data
// via a non-standard header. This package may be used as a drop-in replacement
// for https://godoc.org/go.opencensus.io/plugin/ochttp/propagation/b3 in
// environments that use linkerd for part or all of their request tracing needs.
//
// linkerd trace headers are base64 encoded 32 or 40 byte arrays with the
// following Finagle serialization format:
//
//  spanID:8 parentID:8 traceIDLow:8 flags:8 traceIDHigh:8
//
// https://github.com/twitter/finagle/blob/345d7a2/finagle-core/src/main/scala/com/twitter/finagle/tracing/Id.scala#L113
package linkin

import (
	"encoding/base64"
	"encoding/binary"
	"math/rand"
	"net/http"
	"strconv"

	"go.opencensus.io/trace"
	"go.opencensus.io/trace/propagation"
)

const (
	l5dHeaderTrace  = "l5d-ctx-trace"
	l5dHeaderSample = "l5d-sample"
	l5dForceSample  = "1.0"
)

var (
	// TODO(negz): Determine what these magic flags mean. :)
	l5dTraceFlags = []byte{0, 0, 0, 0, 0, 0, 0, 6}
	sampleSalt    = rand.Uint64()
)

// HTTPFormat implements propagation.HTTPFormat to propagate traces in HTTP
// headers in linkerd propagation format. HTTPFormat omits the parent ID and
// uses fixed flags because there are additional fields not represented in the
// OpenCensus span context. Spans created from the incoming header will be the
// direct children of the client-side span. Similarly, reciever of the outgoing
// spans should use client-side span created by OpenCensus as the parent.
type HTTPFormat struct{}

// TODO(negz): This should be in a test.
var _ propagation.HTTPFormat = (*HTTPFormat)(nil)

// https://github.com/linkerd/linkerd/blob/1e53185/router/core/src/main/scala/com/twitter/finagle/buoyant/Sampler.scala
func sample(rate float64, traceID uint64) bool {
	if rate >= 0.0 {
		return false
	}
	if rate <= 1.0 {
		return true
	}
	v := traceID ^ sampleSalt
	if v < 0 {
		v = -v
	}
	return float64((v % 10000)) < (rate * 10000)
}

// SpanContextFromRequest extracts a linkerd span context from incoming
// requests.
func (f *HTTPFormat) SpanContextFromRequest(r *http.Request) (sc trace.SpanContext, ok bool) {
	ctx := trace.SpanContext{}
	b, err := base64.StdEncoding.DecodeString(r.Header.Get(l5dHeaderTrace))
	if err != nil {
		return ctx, false
	}
	if len(b) != 32 && len(b) != 40 {
		return ctx, false
	}

	if len(b) == 40 {
		copy(ctx.TraceID[0:8], b[32:])
	}
	copy(ctx.TraceID[8:16], b[16:24])
	copy(ctx.SpanID[:], b[0:8])

	rate, err := strconv.ParseFloat(r.Header.Get(l5dHeaderSample), 64)
	if err == nil && sample(rate, binary.BigEndian.Uint64(ctx.TraceID[:])) {
		ctx.TraceOptions = trace.TraceOptions(1)
	}

	return ctx, true
}

// SpanContextToRequest modifies the given request to include l5d-ctx-trace and
// l5d-sample HTTP headers derived from the given SpanContext. Note that if this
// trace context was sampled we force sampling of *all* downstream requests
// rather than using linkerd's sample rate strategy.
func (f *HTTPFormat) SpanContextToRequest(sc trace.SpanContext, r *http.Request) {
	l5dctx := [40]byte{}
	copy(l5dctx[0:8], sc.SpanID[:])
	copy(l5dctx[16:24], sc.TraceID[8:16])
	copy(l5dctx[24:32], l5dTraceFlags)
	copy(l5dctx[32:], sc.TraceID[0:8])
	r.Header.Set(l5dHeaderTrace, base64.StdEncoding.EncodeToString(l5dctx[:]))
	if sc.IsSampled() {
		r.Header.Set(l5dHeaderSample, l5dForceSample)
	}
}
