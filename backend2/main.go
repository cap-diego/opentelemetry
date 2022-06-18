package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.10.0"
	"go.opentelemetry.io/otel/trace"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

const name string = "fraud"

var tracer trace.Tracer

func main() {
	fmt.Println("creating fraud backend...")

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

	tracer = tp.Tracer(name)

	http.HandleFunc("/api/fraud", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			get(w, r)
			return
		}

		ctx, span := tracer.Start(r.Context(), "HTTP POST /api/fraud")
		defer span.End()

		var p struct {
			Amount string `json:"amount"`
			CardID string `json:"card_id"`
		}

		b, err := io.ReadAll(r.Body)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		if err := json.Unmarshal(b, &p); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			w.WriteHeader(http.StatusBadRequest)

			return
		}

		approved := score(ctx, p.CardID, p.Amount)

		if err := save(ctx, p.CardID, p.Amount, approved); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if err := sendNotification(ctx, p.CardID); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())

			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	})

	if err := http.ListenAndServe("localhost:9001", nil); err != nil {
		panic(err)
	}
}

func get(w http.ResponseWriter, r *http.Request) {
	_, span := tracer.Start(r.Context(), "HTTP GET /api/fraud")
	defer span.End()

	var p struct {
		CardID string `json:"card_id"`
	}
	b, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(b, &p); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())

		w.WriteHeader(http.StatusBadRequest)

		return
	}

	time.Sleep(250 * time.Millisecond)

	w.Write([]byte(fmt.Sprintf(`{"status":"active"}`)))
}

func score(ctx context.Context, cardID, amount string) bool {
	_, span := tracer.Start(ctx, "calculate-score")
	defer span.End()

	time.Sleep(800 * time.Millisecond)

	return rand.Intn(2) == 1
}

func sendNotification(ctx context.Context, cardID string) error {
	_, span := tracer.Start(ctx, "notification-api")
	defer span.End()

	req, _ := http.NewRequest("POST",
		"http://localhost:9003/api/notification",
		strings.NewReader(fmt.Sprintf(`{"card_id":"%s"}`, cardID)),
	)

	ctx, cancelFn := context.WithTimeout(ctx, 3*time.Second)
	defer cancelFn()

	req.WithContext(ctx)
	req.Header.Set("content-type", "application/json")

	_, err := http.DefaultClient.Do(req)

	return err
}

func save(ctx context.Context, cardID, amount string, approved bool) error {
	ctx, span := tracer.Start(ctx,
		"save-score",
		trace.WithAttributes(attribute.Bool("approved", approved)),
	)
	defer span.End()

	s := rand.Intn(5)
	time.Sleep(time.Duration(s) * time.Second)

	const timeout int = 2
	if s > timeout {
		return errors.New("timeout saving score")
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
			semconv.ServiceNameKey.String("fraud"),
			semconv.ServiceVersionKey.String("v0.1.0"),
			attribute.String("environment", "staging"),
		),
	)
	return r
}
