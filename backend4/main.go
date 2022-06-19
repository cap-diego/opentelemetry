package main

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus/push"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	dummyGaugeMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "dummy_gauge_metric",
		Help: "A dummy gauge metric",
	})

	dummyCounterVecMetric = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "dummy_countervec_metric",
		Help: "A dummy countervec metric",
	},
		[]string{"instance", "cprovider"},
	)

	dummyCounterMetric = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "non_scrappable_counter_metric",
		Help: "A dummy non scrappable counter metric",
	})
)

func recordScrappableMetrics() {
	go func() {
		for {
			r := rand.Intn(2)
			a := []string{"instance_a", "instance_b"}[r]
			b := []string{"gpc", "aws"}[r]
			dummyCounterVecMetric.WithLabelValues(a, b).Add(float64(rand.Intn(50)))
			dummyGaugeMetric.Set(float64(rand.Intn(502)))

			time.Sleep(2 * time.Second)
		}
	}()
}

func main() {
	recordScrappableMetrics()

	// We use a registry here to benefit from the consistency checks that
	// happen during registration.
	registry := prometheus.NewRegistry()
	registry.MustRegister(dummyCounterMetric)

	pusher := push.New("http://localhost:9091", "dummy_non_scrappable_target").Gatherer(registry)

	go func() {
		t := time.NewTicker(2 * time.Second)
		for {
			select {
			case <-t.C:
				dummyCounterMetric.Inc()

				if err := pusher.Add(); err != nil {
					fmt.Println("could not push to pushgateway:", err)
				} else {
					fmt.Println("pushed metric to pushgateway successfully")
				}

			default:
			}
		}

	}()

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9004", nil))
}
