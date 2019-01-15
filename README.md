# linkin  [![Godoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/planetlabs/linkin) [![Travis](https://img.shields.io/travis/planetlabs/linkin.svg?maxAge=300)](https://travis-ci.com/planetlabs/linkin/) [![Codecov](https://img.shields.io/codecov/c/github/planetlabs/linkin.svg?maxAge=3600)](https://codecov.io/gh/planetlabs/linkin/)
Opencensus Trace propagation for linkerd.

[Opencensus](https://opencensus.io/) is a single distribution of libraries that
automatically collects traces and metrics from your app, displays them locally,
and sends them to any analysis tool.

Opencensus supports the [Zipkin](https://zipkin.io/) request tracing system,
among others. Zipkin is a popular distributed tracing system, allowing requests
to be traced through a distributed system. A request is broken into 'spans',
each representing one part of the greater request path. Software that wishes to
leverage Zipkin must be instrumented to do so (for example via Opencensus).

[linkerd](http://linkerd.io/) is a popular service mesh. One of linkerd's
selling points is that it provides Zipkin request tracing 'for free'. Software
need not be 'fully' instrumented, and instead need only copy linkerd's
`l5d-ctx-trace` HTTP headers from incoming HTTP requests to any outgoing HTTP
requests they spawn. Unfortunately while linkerd emits traces to Zipkin, it
propagates trace data along the request path via a non-standard header. This
package may be used as a drop-in replacement for Opencensus's
[standard Zipkin propagation](https://godoc.org/go.opencensus.io/plugin/ochttp/propagation/b3)
in environments that use linkerd for part or all of their request tracing needs.

In addition to propagating linkerd trace context from incoming to outgoing HTTP
requests via the [ochttp](https://godoc.org/go.opencensus.io/plugin/ochttp)
library, Opencensus may be used to add spans representing in-application
method or database calls to a linkerd trace.

## Usage
```go
// ochttp will automatically inject a span into the context of requests handled
// by usersHandler. If incoming requests contain a valid l5d-ctx-trace header
// the injected span will be a child of the calling span.
http.Handle("/users", usersHandler)
log.Fatal(http.ListenAndServe("localhost:8080", &ochttp.Handler{Propagation: &linkin.HTTPFormat{}}))

// ochttp will automatically inject a span into the context of outgoing requests
// sent by the client. Span metadata will be propagated via outgoing requests'
// l5d-ctx-trace header.
client := http.Client{Transport: &ochttp.Transport{Propagation: &linkin.HTTPFormat{}}}

// Create a relationship between the incoming and outgoing requests by
// propagating the incoming request's context to the child. If the incoming
// request's context contains a span the outgoing request's span will be its
// child.
outgoingRequest, _ := http.NewRequest("GET", d, nil)
outgoingRequest = out.WithContext(incomingRequest.Context())
rsp, _ := client.Do(out)
```

A more complete example exists at [example/](example/). The
[ochttp godoc](https://godoc.org/go.opencensus.io/plugin/ochttp) may also be
illustrative.
