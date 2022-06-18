package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"
)

const name string = "fraud"

var tracer trace.Tracer

func main() {
	fmt.Println("creating notification backend...")

	l := log.New(os.Stdout, "", 0)

	exp, err := otlptrace.New(
		context.Background(),
		otlptracegrpc.NewClient(
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		),
	)
	if err != nil {
		l.Fatal(err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(newResource()),
	)
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			l.Fatal(err)
		}
	}()

	otel.SetTracerProvider(tp)

	tracer = tp.Tracer(name)

	h := func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(),
			"process",
			trace.WithAttributes(attribute.String("a", "val")),
		)
		defer span.End()
		labeler, _ := otelhttp.LabelerFromContext(ctx)

		span.AddEvent("an-event")

		fmt.Println("new request")

		var p struct {
			CardID string `json:"card_id"`
		}

		b, err := io.ReadAll(r.Body)
		if err != nil {
			fmt.Println("error reading body", err.Error())

			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			labeler.Add(attribute.Bool("error", true))

			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		fmt.Println("body", string(b))
		if err := json.Unmarshal(b, &p); err != nil {
			fmt.Println("error processing payment", err.Error())

			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			labeler.Add(attribute.Bool("error", true))

			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		if err := save(ctx, p.CardID); err != nil {
			fmt.Println("error saving", err.Error())

			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			labeler.Add(attribute.Bool("error", true))

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		labeler.Add(attribute.Bool("error", false))
		w.WriteHeader(http.StatusOK)
		return
	}

	var mux http.ServeMux
	mux.Handle("/api/notification", otelhttp.WithRouteTag("/api/notification", http.HandlerFunc(h)))

	if err := http.ListenAndServe(":9003", otelhttp.NewHandler(&mux,
		"POST /api/notification",
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents)),
	); err != nil {
		log.Fatal(err)
	}
}

func save(ctx context.Context, cardID string) error {
	ctx, span := tracer.Start(ctx, "save-notification")
	defer span.End()

	s := rand.Intn(5)
	time.Sleep(time.Duration(s) * time.Second)

	const timeout int = 2
	if s > timeout {
		return errors.New("timeout saving notification")
	}

	return nil
}

// newResource returns a resource describing this application.
func newResource() *resource.Resource {
	r, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("notification"),
			semconv.ServiceVersionKey.String("v0.1.0"),
			attribute.String("environment", "staging"),
		),
	)
	return r
}
