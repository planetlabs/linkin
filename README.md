# linkin  [![Godoc](https://img.shields.io/badge/godoc-reference-blue.svg)](https://godoc.org/github.com/negz/linkin) [![Travis](https://img.shields.io/travis/negz/linkin.svg?maxAge=300)](https://travis-ci.org/negz/linkin/) [![Codecov](https://img.shields.io/codecov/c/github/negz/linkin.svg?maxAge=3600)](https://codecov.io/gh/negz/linkin/)
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
propagates trace data via a non-standard header. This package may be used as a
drop-in replacement for https://godoc.org/go.opencensus.io/plugin/ochttp/propagation/b3
in environments that use linkerd for part or all of their request tracing needs.

In addition to propagating linkerd trace context from incoming to outgoing HTTP
requests via the [ochttp](https://godoc.org/go.opencensus.io/plugin/ochttp)
library, Opencensus may be used to add spans representing in-application
method or database calls to a linkerd trace.
