package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel/propagation"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"
)

const name = "payments"

var tracer trace.Tracer

func main() {
	fmt.Println("creating payments backend...")

	l := log.New(os.Stdout, "", 0)

	exp, err := newJaegerExporter()
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

	otel.SetTextMapPropagator(propagation.TraceContext{})

	tracer = tp.Tracer(name)

	http.HandleFunc("/api/payment", processPayment())

	if err := http.ListenAndServe("localhost:9000", nil); err != nil {
		panic(err)
	}
}

func processPayment() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracer.Start(r.Context(), "HTTP POST /api/payment")
		defer span.End()

		var p struct {
			Amount string `json:"amount"`
			CardID string `json:"card_id"`
		}

		b, _ := io.ReadAll(r.Body)
		err := json.Unmarshal(b, &p)
		if err != nil {
			span.RecordError(err)

			http.Error(w, err.Error(), http.StatusBadRequest)

			return
		}

		paymentID := rand.Int()

		if err := fraudScoringCheck(ctx, p.CardID, p.Amount); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		if err := save(ctx, fmt.Sprintf("%d", paymentID)); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}

		// fmt.Println("payment id", paymentID, "created successfully")

		w.Write([]byte(fmt.Sprintf(`{"id":"%d"}`, paymentID)))

		fmt.Println(span.SpanContext().TraceID())

		return
	}
}

func fraudScoringCheck(ctx context.Context, cardID, amount string) error {
	_, span := tracer.Start(ctx, "fraud-scoring-api")
	defer span.End()

	req, _ := http.NewRequest("POST",
		"http://localhost:9001/api/fraud",
		strings.NewReader(fmt.Sprintf(`{"card_id":"%s", "amount":"%s"}`, cardID, amount)),
	)

	ctx, cancelFn := context.WithTimeout(ctx, 3*time.Second)
	defer cancelFn()

	req.WithContext(ctx)
	req.Header.Set("content-type", "application/json")

	_, err := http.DefaultClient.Do(req)

	return err
}

// save simulates a db call
func save(ctx context.Context, paymentID string) error {
	_, span := tracer.Start(ctx, "save-payment")
	defer span.End()

	s := rand.Intn(5)
	time.Sleep(time.Duration(s) * time.Second)

	const timeout int = 2
	if s > timeout {
		return errors.New("save timeout")
	}

	return nil
}

func newJaegerExporter() (sdktrace.SpanExporter, error) {
	os.Setenv("OTEL_EXPORTER_JAEGER_ENDPOINT", "http://localhost:14268/api/traces")
	os.Setenv("OTEL_EXPORTER_JAEGER_AGENT_PORT", "6831")

	return jaeger.New(jaeger.WithAgentEndpoint())
}

// newResource returns a resource describing this application.
func newResource() *resource.Resource {
	r, _ := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("payments"),
			semconv.ServiceVersionKey.String("v0.1.0"),
			attribute.String("environment", "staging"),
		),
	)
	return r
}
