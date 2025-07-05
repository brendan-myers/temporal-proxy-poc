package metrics

import (
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
)

func InitPrometheus() (*metric.MeterProvider, error) {
	exporter, err := prometheus.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize prometheus exporter: %w", err)
	}

	// Custom histogram buckets with millisecond precision
	histogramView := metric.NewView(
		metric.Instrument{Kind: metric.InstrumentKindHistogram},
		metric.Stream{
			Aggregation: metric.AggregationExplicitBucketHistogram{
				Boundaries: []float64{
					0.00001, // 10 microseconds
					0.00005, // 50 microseconds
					0.0001,  // 100 microseconds
					0.0005,  // 500 microseconds
					0.001,   // 1 millisecond
					0.005,   // 5 milliseconds
					0.01,    // 10 milliseconds
					0.025,   // 25 milliseconds
					0.05,    // 50 milliseconds
					0.1,     // 100 milliseconds
					0.25,    // 250 milliseconds
					0.5,     // 500 milliseconds
					1.0,     // 1 second
					2.5,     // 2.5 seconds
					5.0,     // 5 seconds
					10.0,    // 10 seconds
				},
			},
		},
	)

	provider := metric.NewMeterProvider(
		metric.WithReader(exporter),
		metric.WithView(histogramView),
	)
	otel.SetMeterProvider(provider)

	return provider, nil
}
