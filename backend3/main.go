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
	"strings"
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
	// this backed uses SigNoz as observability & monitoring platform
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

	fmt.Println("settings trace provider")
	otel.SetTracerProvider(tp)

	tracer = tp.Tracer(name)
	fmt.Println("tracer set")

	h := func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("handler")
		ctx, span := tracer.Start(r.Context(),
			"process",
			trace.WithAttributes(attribute.String("a", "val")),
		)
		defer span.End()

		labeler, _ := otelhttp.LabelerFromContext(ctx)

		span.AddEvent("an-event")

		var p struct {
			CardID string `json:"card_id"`
		}

		b, err := io.ReadAll(r.Body)
		if err != nil {

			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			labeler.Add(attribute.Bool("error", true))

			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		if err := json.Unmarshal(b, &p); err != nil {

			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			labeler.Add(attribute.Bool("error", true))

			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		if err := save(ctx, p.CardID); err != nil {

			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			labeler.Add(attribute.Bool("error", true))

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		status, err := checkFraud(ctx, p.CardID)
		if err != nil || status != "active" {

			span.RecordError(err)
			span.SetStatus(codes.Error, "error")
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

	fmt.Println("handler set")

	if err := http.ListenAndServe(":9003", otelhttp.NewHandler(&mux,
		"POST /api/notification",
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents)),
	); err != nil {
		log.Fatal(err)
	}
}

func checkFraud(ctx context.Context, cardID string) (string, error) {
	_, span := tracer.Start(ctx, "check-fraud")
	defer span.End()

	req, _ := http.NewRequest("GET",
		"http://localhost:9001/api/fraud",
		strings.NewReader(fmt.Sprintf(`{"card_id":"%s"}`, cardID)),
	)

	ctx, cancelFn := context.WithTimeout(ctx, 3*time.Second)
	defer cancelFn()

	req.WithContext(ctx)
	req.Header.Set("content-type", "application/json")

	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	b, _ := io.ReadAll(response.Body)

	var p struct {
		Status string `json:"status"`
	}

	json.Unmarshal(b, &p)

	return p.Status, nil
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
