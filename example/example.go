/*
Copyright 2018 Planet Labs Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing permissions
and limitations under the License.
*/

package main

import (
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"time"

	openzipkin "github.com/openzipkin/zipkin-go"
	openzipkinhttp "github.com/openzipkin/zipkin-go/reporter/http"
	"github.com/planetlabs/linkin"
	"go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/exporter/zipkin"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
	"go.uber.org/zap"
	"gopkg.in/alecthomas/kingpin.v2"
)

const serviceName = "example"

/*
TODO(negz): Throw some useful baggage on the traces?
*/

func loggingHandler(log *zap.Logger, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if dump, err := httputil.DumpRequest(r, false); err == nil {
			log.Info("request", zap.ByteString("in", dump))
		}
		h.ServeHTTP(w, r)
	})
}

type loggingTransport struct {
	log  *zap.Logger
	base http.RoundTripper
}

func (t *loggingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if dump, err := httputil.DumpRequest(r, false); err == nil {
		t.log.Info("request", zap.ByteString("out", dump))
	}
	rsp, err := t.base.RoundTrip(r)
	if err == nil {
		if dump, err := httputil.DumpResponse(rsp, false); err == nil {
			t.log.Info("response", zap.ByteString("in", dump))
		}
	}
	return rsp, err
}

func (t *loggingTransport) CancelRequest(r *http.Request) {
	if cr, ok := t.base.(interface {
		CancelRequest(*http.Request)
	}); ok {
		cr.CancelRequest(r)
	}
}

func propagateRequest(log *zap.Logger, downstreams []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		/*
			The ochttp.Handler middleware that wraps this handler automatically
			creates and injects a span representing this request into its
			context. If the request contained linkerd trace propagation headers
			the newly created span will be a child of the span that sent the
			request.
		*/

		// Create a new span to represent a database call. We discard the
		// resulting context as we don't intend to propagate this span.
		_, dbSpan := trace.StartSpan(r.Context(), "database query")
		dbSpan.AddAttributes(trace.StringAttribute("database.type", "pretend"))
		dbSpan.AddAttributes(trace.StringAttribute("database.success", "likely"))
		go func(s *trace.Span) {
			time.Sleep(50 * time.Millisecond)
			s.End()
		}(dbSpan)

		// Create an HTTP round tripper with some Opencensus middleware. Note we
		// use linkerd's trace propagation headers rather than Zipkin's.
		var t http.RoundTripper = &ochttp.Transport{Propagation: &linkin.HTTPFormat{}}

		// Wrap the HTTP router with some simple request logging middleware.
		t = &loggingTransport{log: log, base: t}

		// Create an HTTP client that uses our transport.
		client := http.Client{Transport: t}

		// Loop over each configured downstream (assumed to be instances of this
		// example application) and send an HTTP request to each.
		for _, d := range downstreams {
			out, err := http.NewRequest("GET", d, nil)
			if err != nil {
				log.Error("cannot form downstream request", zap.Error(err))
				continue
			}

			/*
				The ochttp.Transport round tripper uses the outgoing HTTP
				request's context to create a span representing the outgoing
				request. We set the outgoing request's context to that of the
				incoming request. Recall that the ochttp.Handler middleware
				injected a span into the incoming request's context; said span
				will be the parent of the span created by ochttp.Transport.

				Note that the incoming request's context is canceled when the
				client's connection closes, the request is canceled (with
				HTTP/2), or when the ServeHTTP method returns. You'll need to
				extract the span and stash it in a new context if this request
				should create asynchronous spans:

				  ctx := trace.NewContext(context.Background(), trace.FromContext(r.Context()))

			*/
			out = out.WithContext(r.Context())

			rsp, err := client.Do(out)
			if err != nil {
				log.Error("cannot send request", zap.Error(err))
				continue
			}
			rsp.Body.Close()
		}
	}
}

func main() {
	var (
		app            = kingpin.New(filepath.Base(os.Args[0]), "Traces stuff, and also junk!").DefaultEnvars()
		debug          = app.Flag("debug", "Run with debug logging.").Short('d').Bool()
		listen         = app.Flag("listen", "Address at which to listen.").Default("0.0.0.0:10002").String()
		zipkinEndpoint = app.Flag("zipkin", "Address at which Zipkin listens.").Default("http://zipkin.kube-system:9411/api/v2/spans").String()
		downstreams    = app.Arg("downstreams", "Downstream service URLs to query").Strings()
	)
	kingpin.MustParse(app.Parse(os.Args[1:]))

	// Create a logger.
	var log *zap.Logger
	log, err := zap.NewProduction()
	if *debug {
		log, err = zap.NewDevelopment()
	}
	kingpin.FatalIfError(err, "cannot create log")

	// Create and register an Opencensus Zipkin exporter.
	endpoint, err := openzipkin.NewEndpoint(serviceName, *listen)
	kingpin.FatalIfError(err, "cannot set Zipkin endpoint")
	zipkinExporter := zipkin.NewExporter(openzipkinhttp.NewReporter(*zipkinEndpoint), endpoint)
	trace.RegisterExporter(zipkinExporter)

	// Create and register an Opencensus Prometheus exporter.
	prometheusExporter, err := prometheus.NewExporter(prometheus.Options{Namespace: serviceName})
	kingpin.FatalIfError(err, "cannot create prometheus exporter")
	view.RegisterExporter(prometheusExporter)

	// Register default views (i.e. metrics) for ochttp server and clients.
	err = view.Register(ochttp.DefaultServerViews...)
	kingpin.FatalIfError(err, "cannot register opencensus HTTP server views")
	err = view.Register(ochttp.DefaultClientViews...)
	kingpin.FatalIfError(err, "cannot register opencensus HTTP client views")

	// Create an HTTP router.
	r := http.NewServeMux()
	r.HandleFunc("/", propagateRequest(log, *downstreams))
	r.Handle("/metrics", prometheusExporter)

	// Wrap the HTTP router with some simple request logging middleware.
	h := loggingHandler(log, r)

	// Wrap the HTTP router with some Opencensus middleware. Note we use
	// linkerd's trace propagation headers rather than Zipkin's.
	h = &ochttp.Handler{Handler: h, Propagation: &linkin.HTTPFormat{}}

	// Start listening for HTTP requests!
	s := &http.Server{Addr: *listen, Handler: h}
	log.Info("shutdown", zap.Error(s.ListenAndServe()))
}
