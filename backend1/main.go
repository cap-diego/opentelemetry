package main

import (
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
)

func main() {
	fmt.Println("creating backend 1...")

	l := log.New(os.Stdout, "", 0)

	exp, err := newJaegerExporter()
	if err != nil {
		l.Fatal(err)
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(exp),
		trace.WithResource(newResource()),
	)
	defer func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			l.Fatal(err)
		}
	}()

	otel.SetTracerProvider(tp)

	http.HandleFunc("/api/payment", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("new payment")

		var p struct {
			Amount string `json:"amount"`
			CardID string `json:"card_id"`
		}

		b, _ := io.ReadAll(r.Body)
		err := json.Unmarshal(b, &p)
		if err != nil {
			fmt.Println("error processing payment", err.Error())

			w.WriteHeader(http.StatusBadRequest)

			return
		}

		paymentID := rand.Int()

		fmt.Println("payment id", paymentID, "created successfully")

		w.Write([]byte(fmt.Sprintf(`{"id":"%d"}`, paymentID)))
		return
	})

	if err := http.ListenAndServe("localhost:9000", nil); err != nil {
		panic(err)
	}
}

func newJaegerExporter() (trace.SpanExporter, error) {
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
			semconv.ServiceNameKey.String("httpserver"),
			semconv.ServiceVersionKey.String("v0.1.0"),
			attribute.String("environment", "staging"),
		),
	)
	return r
}
