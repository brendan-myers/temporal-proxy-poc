package metrics

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.temporal.io/sdk/client"
)

const TemporalCloudMeterName = "temporal-cloud-proxy"

var _ client.MetricsHandler = MetricsHandler{}

// MetricsHandler is an implementation of client.MetricsHandler
// for open telemetry.
type MetricsHandler struct {
	meter      metric.Meter
	attributes attribute.Set
	onError    func(error)
}

// MetricsHandlerOptions are options provided to NewMetricsHandler.
type MetricsHandlerOptions struct {
	// Meter is the Meter to use. If not set, one is obtained from the global
	// meter provider using the name "temporal-sdk-go".
	Meter metric.Meter
	// InitialAttributes to set on the handler
	//
	// Optional: Defaults to the empty set.
	InitialAttributes attribute.Set
	// OnError Callback to invoke if the provided meter returns an error.
	//
	// Optional: Defaults to panicking on any error.
	OnError func(error)
}

// CounterFunc implements Counter with a single function.
type CounterFunc func(int64)

// Inc implements Counter.Inc.
func (c CounterFunc) Inc(d int64) { c(d) }

// GaugeFunc implements Gauge with a single function.
type GaugeFunc func(float64)

// Update implements Gauge.Update.
func (g GaugeFunc) Update(d float64) { g(d) }

// TimerFunc implements Timer with a single function.
type TimerFunc func(time.Duration)

// Record implements Timer.Record.
func (t TimerFunc) Record(d time.Duration) { t(d) }

// NewMetricsHandler returns a client.MetricsHandler that is backed by the given Meter
func NewMetricsHandler(options MetricsHandlerOptions) MetricsHandler {
	if options.Meter == nil {
		options.Meter = otel.GetMeterProvider().Meter(TemporalCloudMeterName)
	}
	if options.OnError == nil {
		options.OnError = func(err error) { panic(err) }
	}
	return MetricsHandler{
		meter:      options.Meter,
		attributes: options.InitialAttributes,
		onError:    options.OnError,
	}
}

// ExtractMetricsHandler gets the underlying Open Telemetry MetricsHandler from a MetricsHandler
// if any is present.
//
// Raw use of the MetricHandler is discouraged but may be used for Histograms or other
// advanced features. This scope does not skip metrics during replay like the
// metrics handler does. Therefore the caller should check replay state.
func ExtractMetricsHandler(handler client.MetricsHandler) *MetricsHandler {
	// Continually unwrap until we find an instance of our own handler
	for {
		otelHandler, ok := handler.(MetricsHandler)
		if ok {
			return &otelHandler
		}
		// If unwrappable, do so, otherwise return noop
		unwrappable, _ := handler.(interface{ Unwrap() client.MetricsHandler })
		if unwrappable == nil {
			return nil
		}
		handler = unwrappable.Unwrap()
	}
}

// GetMeter returns the meter used by this handler.
func (m MetricsHandler) GetMeter() metric.Meter {
	return m.meter
}

// GetAttributes returns the attributes set on this handler.
func (m MetricsHandler) GetAttributes() attribute.Set {
	return m.attributes
}

func (m MetricsHandler) WithTags(tags map[string]string) client.MetricsHandler {
	attributes := m.attributes.ToSlice()
	for k, v := range tags {
		attributes = append(attributes, attribute.String(k, v))
	}
	return MetricsHandler{
		meter:      m.meter,
		attributes: attribute.NewSet(attributes...),
		onError:    m.onError,
	}
}

func (m MetricsHandler) Counter(name string) client.MetricsCounter {
	c, err := m.meter.Int64UpDownCounter(name)
	if err != nil {
		m.onError(err)
		return client.MetricsNopHandler.Counter(name)
	}
	return CounterFunc(func(d int64) {
		c.Add(context.Background(), d, metric.WithAttributeSet(m.attributes))
	})
}

func (m MetricsHandler) Gauge(name string) client.MetricsGauge {
	g, err := m.meter.Float64Gauge(name)
	if err != nil {
		m.onError(err)
		return client.MetricsNopHandler.Gauge(name)
	}
	return GaugeFunc(func(f float64) {
		g.Record(context.Background(), f, metric.WithAttributeSet(m.attributes))
	})
}

func (m MetricsHandler) Timer(name string) client.MetricsTimer {
	h, err := m.meter.Float64Histogram(name, metric.WithUnit("s"))
	if err != nil {
		m.onError(err)
		return client.MetricsNopHandler.Timer(name)
	}
	return TimerFunc(func(t time.Duration) {
		h.Record(context.Background(), t.Seconds(), metric.WithAttributeSet(m.attributes))
	})
}
