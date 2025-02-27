package metrics

import (
	"io"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/roadrunner-server/config/v4"
	"github.com/roadrunner-server/endure/v2"
	goridgeRpc "github.com/roadrunner-server/goridge/v3/pkg/rpc"
	httpPlugin "github.com/roadrunner-server/http/v4"
	"github.com/roadrunner-server/logger/v4"
	"github.com/roadrunner-server/metrics/v4"
	"github.com/roadrunner-server/prometheus/v4"
	rpcPlugin "github.com/roadrunner-server/rpc/v4"
	mocklogger "github.com/roadrunner-server/rr-e2e-tests/mock"
	"github.com/roadrunner-server/server/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"golang.org/x/exp/slog"
)

const dialAddr = "127.0.0.1:6001"
const dialNetwork = "tcp"
const getAddr = "http://127.0.0.1:2112/metrics"
const getAddr2 = "http://127.0.0.1:2113/metrics"
const getIPV6Addr = "http://[::1]:2112/metrics"

func TestMetricsInit(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.9.0",
	}
	cfg.Prefix = "rr"
	cfg.Path = "configs/.rr-test.yaml"

	err := cont.RegisterAll(
		cfg,
		&metrics.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&Plugin1{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	tt := time.NewTimer(time.Second * 5)
	defer tt.Stop()

	time.Sleep(time.Second * 2)
	out, err := getIPV6()
	assert.NoError(t, err)

	assert.Contains(t, out, "go_gc_duration_seconds")
	assert.Contains(t, out, "app_metric_counter")

	for {
		select {
		case e := <-ch:
			assert.Fail(t, "error", e.Error.Error())
		case <-sig:
			err = cont.Stop()
			if err != nil {
				assert.FailNow(t, "error", err.Error())
			}
			return
		case <-tt.C:
			// timeout
			err = cont.Stop()
			if err != nil {
				assert.FailNow(t, "error", err.Error())
			}
			return
		}
	}
}

func TestMetricsIssue571(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.9.0"}
	cfg.Prefix = "rr"
	cfg.Path = "configs/.rr-issue-571.yaml"

	l, oLogger := mocklogger.ZapTestLogger(zap.DebugLevel)
	err := cont.RegisterAll(
		cfg,
		&metrics.Plugin{},
		&rpcPlugin.Plugin{},
		&server.Plugin{},
		l,
		&httpPlugin.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// give some time to wait http
	time.Sleep(time.Second)
	_, err = issue571Http()
	assert.NoError(t, err)

	out, err := issue571Metrics()
	assert.NoError(t, err)

	assert.Contains(t, out, "HELP test Test counter")
	assert.Contains(t, out, "TYPE test counter")

	stopCh := make(chan struct{}, 1)

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-stopCh:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second)
	stopCh <- struct{}{}
	wg.Wait()

	require.Equal(t, 1, oLogger.FilterMessageSnippet("http server was started").Len())
	require.Equal(t, 1, oLogger.FilterMessageSnippet("http log").Len())

	require.Equal(t, 5, oLogger.FilterMessageSnippet("declaring new metric").Len())
	require.Equal(t, 2, oLogger.FilterMessageSnippet("metric successfully added").Len())
	require.Equal(t, 1, oLogger.FilterMessageSnippet("adding metric").Len())
	require.Equal(t, 4, oLogger.FilterMessageSnippet("metric with provided name already exist").Len())
	require.Equal(t, 0, oLogger.FilterMessageSnippet("scan command").Len())
}

// get request and return body
func issue571Http() (string, error) {
	r, err := http.Get("http://127.0.0.1:56444")
	if err != nil {
		return "", err
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	err = r.Body.Close()
	if err != nil {
		return "", err
	}
	// unsafe
	return string(b), err
}

// get request and return body
func issue571Metrics() (string, error) {
	r, err := http.Get("http://127.0.0.1:23557")
	if err != nil {
		return "", err
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	err = r.Body.Close()
	if err != nil {
		return "", err
	}
	// unsafe
	return string(b), err
}

func TestMetricsGaugeCollector(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.9.0",
	}
	cfg.Prefix = "rr"
	cfg.Path = "configs/.rr-test.yaml"

	err := cont.RegisterAll(
		cfg,
		&metrics.Plugin{},
		&rpcPlugin.Plugin{},
		&logger.Plugin{},
		&Plugin1{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	time.Sleep(time.Second)
	tt := time.NewTimer(time.Second * 5)
	defer tt.Stop()

	time.Sleep(time.Second * 2)
	out, err := getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, out, "my_gauge 100")
	assert.Contains(t, out, "my_gauge2 100")

	out, err = getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, out, "go_gc_duration_seconds")

	for {
		select {
		case e := <-ch:
			assert.Fail(t, "error", e.Error.Error())
		case <-sig:
			err = cont.Stop()
			if err != nil {
				assert.FailNow(t, "error", err.Error())
			}
			return
		case <-tt.C:
			// timeout
			err = cont.Stop()
			if err != nil {
				assert.FailNow(t, "error", err.Error())
			}
			return
		}
	}
}

func TestMetricsDifferentRPCCalls(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.9.0",
	}
	cfg.Prefix = "rr"
	cfg.Path = "configs/.rr-test.yaml"

	l, oLogger := mocklogger.ZapTestLogger(zap.DebugLevel)
	err := cont.RegisterAll(
		cfg,
		&metrics.Plugin{},
		&rpcPlugin.Plugin{},
		l,
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	tt := time.NewTimer(time.Minute * 3)
	defer tt.Stop()
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-tt.C:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)
	t.Run("DeclareMetric", declareMetricsTest)
	genericOut, err := getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, "test_metrics_named_collector")

	t.Run("AddMetric", addMetricsTest)
	genericOut, err = getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, "test_metrics_named_collector 10000")

	t.Run("SetMetric", setMetric)
	genericOut, err = getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, "user_gauge_collector 100")

	t.Run("VectorMetric", vectorMetric)
	genericOut, err = getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, "gauge_2_collector{section=\"first\",type=\"core\"} 100")

	t.Run("MissingSection", missingSection)
	t.Run("SetWithoutLabels", setWithoutLabels)
	t.Run("SetOnHistogram", setOnHistogram)
	t.Run("MetricSub", subMetric)
	genericOut, err = getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, "sub_gauge_subMetric 1")

	t.Run("SubVector", subVector)
	genericOut, err = getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, "sub_gauge_subVector{section=\"first\",type=\"core\"} 1")

	t.Run("RegisterHistogram", registerHistogram)

	genericOut, err = getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, `TYPE histogram_registerHistogram`)

	// check buckets
	assert.Contains(t, genericOut, `histogram_registerHistogram_bucket{le="0.1"} 0`)
	assert.Contains(t, genericOut, `histogram_registerHistogram_bucket{le="0.2"} 0`)
	assert.Contains(t, genericOut, `histogram_registerHistogram_bucket{le="0.5"} 0`)
	assert.Contains(t, genericOut, `histogram_registerHistogram_bucket{le="+Inf"} 0`)
	assert.Contains(t, genericOut, `histogram_registerHistogram_sum 0`)
	assert.Contains(t, genericOut, `histogram_registerHistogram_count 0`)

	t.Run("CounterMetric", counterMetric)
	genericOut, err = getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, "HELP default_default_counter_CounterMetric test_counter")
	assert.Contains(t, genericOut, `default_default_counter_CounterMetric{section="section2",type="type2"}`)

	t.Run("ObserveMetric", observeMetric)
	genericOut, err = getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, "observe_observeMetric")

	t.Run("ObserveMetricNotEnoughLabels", observeMetricNotEnoughLabels)

	t.Run("ConfiguredCounterMetric", configuredCounterMetric)
	genericOut, err = getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, "HELP app_metric_counter Custom application counter.")
	assert.Contains(t, genericOut, `app_metric_counter 100`)

	close(sig)
	wg.Wait()

	require.Equal(t, 0, oLogger.FilterMessageSnippet("http server was started").Len())
	require.Equal(t, 0, oLogger.FilterMessageSnippet("http log").Len())

	require.Equal(t, 6, oLogger.FilterMessageSnippet("adding metric").Len())
	require.Equal(t, 17, oLogger.FilterMessageSnippet("metric successfully added").Len())
	require.Equal(t, 12, oLogger.FilterMessageSnippet("declaring new metric").Len())
	require.Equal(t, 7, oLogger.FilterMessageSnippet("observing metric").Len())
	require.Equal(t, 1, oLogger.FilterMessageSnippet("observe operation finished successfully").Len())

	require.Equal(t, 2, oLogger.FilterMessageSnippet("set operation finished successfully").Len())
	require.Equal(t, 2, oLogger.FilterMessageSnippet("subtracting value from metric").Len())
	require.Equal(t, 2, oLogger.FilterMessageSnippet("subtracting operation finished successfully").Len())
	require.Equal(t, 2, oLogger.FilterMessageSnippet("failed to get metrics with label values").Len())
	require.Equal(t, 1, oLogger.FilterMessageSnippet("required labels for collector").Len())
	require.Equal(t, 2, oLogger.FilterMessageSnippet("failed to get metrics with label values").Len())
}

func configuredCounterMetric(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	assert.NoError(t, client.Call("metrics.Add", metrics.Metric{
		Name:  "app_metric_counter",
		Value: 100.0,
	}, &ret))
	assert.True(t, ret)
}

func observeMetricNotEnoughLabels(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "observe_observeMetricNotEnoughLabels",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Help:      "test_observe",
			Type:      metrics.Histogram,
			Labels:    []string{"type", "section"},
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
	ret = false

	assert.Error(t, client.Call("metrics.Observe", metrics.Metric{
		Name:   "observe_observeMetric",
		Value:  100.0,
		Labels: []string{"test"},
	}, &ret))
	assert.False(t, ret)
}

func observeMetric(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "observe_observeMetric",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Help:      "test_observe",
			Type:      metrics.Histogram,
			Labels:    []string{"type", "section"},
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
	ret = false

	assert.NoError(t, client.Call("metrics.Observe", metrics.Metric{
		Name:   "observe_observeMetric",
		Value:  100.0,
		Labels: []string{"test", "test2"},
	}, &ret))
	assert.True(t, ret)
}

func counterMetric(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "counter_CounterMetric",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Help:      "test_counter",
			Type:      metrics.Counter,
			Labels:    []string{"type", "section"},
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)

	ret = false

	assert.NoError(t, client.Call("metrics.Add", metrics.Metric{
		Name:   "counter_CounterMetric",
		Value:  100.0,
		Labels: []string{"type2", "section2"},
	}, &ret))
	assert.True(t, ret)
}

func registerHistogram(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "histogram_registerHistogram",
		Collector: metrics.Collector{
			Help:    "test_histogram",
			Type:    metrics.Histogram,
			Buckets: []float64{0.1, 0.2, 0.5},
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)

	ret = false

	m := metrics.Metric{
		Name:   "histogram_registerHistogram",
		Value:  10000,
		Labels: nil,
	}

	err = client.Call("metrics.Add", m, &ret)
	assert.Error(t, err)
	assert.False(t, ret)
}

func subVector(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()

	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "sub_gauge_subVector",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Type:      metrics.Gauge,
			Labels:    []string{"type", "section"},
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
	ret = false

	m := metrics.Metric{
		Name:   "sub_gauge_subVector",
		Value:  100000,
		Labels: []string{"core", "first"},
	}

	err = client.Call("metrics.Add", m, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
	ret = false

	m = metrics.Metric{
		Name:   "sub_gauge_subVector",
		Value:  99999,
		Labels: []string{"core", "first"},
	}

	err = client.Call("metrics.Sub", m, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
}

func subMetric(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "sub_gauge_subMetric",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Type:      metrics.Gauge,
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
	ret = false

	m := metrics.Metric{
		Name:  "sub_gauge_subMetric",
		Value: 100000,
	}

	err = client.Call("metrics.Add", m, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
	ret = false

	m = metrics.Metric{
		Name:  "sub_gauge_subMetric",
		Value: 99999,
	}

	err = client.Call("metrics.Sub", m, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
}

func setOnHistogram(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "histogram_setOnHistogram",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Type:      metrics.Histogram,
			Labels:    []string{"type", "section"},
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)

	ret = false

	m := metrics.Metric{
		Name:  "gauge_setOnHistogram",
		Value: 100.0,
	}

	err = client.Call("metrics.Set", m, &ret) // expected 2 label values but got 1 in []string{"missing"}
	assert.Error(t, err)
	assert.False(t, ret)
}

func setWithoutLabels(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "gauge_setWithoutLabels",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Type:      metrics.Gauge,
			Labels:    []string{"type", "section"},
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)

	ret = false

	m := metrics.Metric{
		Name:  "gauge_setWithoutLabels",
		Value: 100.0,
	}

	err = client.Call("metrics.Set", m, &ret) // expected 2 label values but got 1 in []string{"missing"}
	assert.Error(t, err)
	assert.False(t, ret)
}

func missingSection(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "gauge_missing_section_collector",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Type:      metrics.Gauge,
			Labels:    []string{"type", "section"},
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)

	ret = false

	m := metrics.Metric{
		Name:   "gauge_missing_section_collector",
		Value:  100.0,
		Labels: []string{"missing"},
	}

	err = client.Call("metrics.Set", m, &ret) // expected 2 label values but got 1 in []string{"missing"}
	assert.Error(t, err)
	assert.False(t, ret)
}

func vectorMetric(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "gauge_2_collector",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Type:      metrics.Gauge,
			Labels:    []string{"type", "section"},
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)

	ret = false

	m := metrics.Metric{
		Name:   "gauge_2_collector",
		Value:  100.0,
		Labels: []string{"core", "first"},
	}

	err = client.Call("metrics.Set", m, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
}

func setMetric(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "user_gauge_collector",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Type:      metrics.Gauge,
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
	ret = false

	m := metrics.Metric{
		Name:  "user_gauge_collector",
		Value: 100.0,
	}

	err = client.Call("metrics.Set", m, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
}

func addMetricsTest(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	m := metrics.Metric{
		Name:   "test_metrics_named_collector",
		Value:  10000,
		Labels: nil,
	}

	err = client.Call("metrics.Add", m, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
}

func declareMetricsTest(t *testing.T) {
	conn, err := net.Dial(dialNetwork, dialAddr)
	assert.NoError(t, err)
	defer func() {
		_ = conn.Close()
	}()
	client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
	var ret bool

	nc := metrics.NamedCollector{
		Name: "test_metrics_named_collector",
		Collector: metrics.Collector{
			Namespace: "default",
			Subsystem: "default",
			Type:      metrics.Counter,
			Help:      "NO HELP!",
			Labels:    nil,
			Buckets:   nil,
		},
	}

	err = client.Call("metrics.Declare", nc, &ret)
	assert.NoError(t, err)
	assert.True(t, ret)
}

func unregisterMetric(name string) func(t *testing.T) {
	return func(t *testing.T) {
		conn, err := net.Dial(dialNetwork, dialAddr)
		assert.NoError(t, err)
		defer func() {
			_ = conn.Close()
		}()
		client := rpc.NewClientWithCodec(goridgeRpc.NewClientCodec(conn))
		var ret bool

		err = client.Call("metrics.Unregister", "test_metrics_named_collector", &ret)
		assert.NoError(t, err)
		assert.True(t, ret)
	}
}

func TestHTTPMetrics(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.9.0"}
	cfg.Prefix = "rr"
	cfg.Path = "configs/.rr-http-metrics.yaml"

	l, oLogger := mocklogger.ZapTestLogger(zap.DebugLevel)
	err := cont.RegisterAll(
		cfg,
		&metrics.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		l,
		&prometheus.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	tt := time.NewTimer(time.Minute * 3)
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer tt.Stop()
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-tt.C:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)
	t.Run("req1", echoHTTP("13223"))
	t.Run("req2", echoHTTP("13223"))

	time.Sleep(time.Millisecond * 500)
	genericOut, err := get()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, `rr_http_request_duration_seconds_bucket`)
	assert.Contains(t, genericOut, `rr_http_request_duration_seconds_sum{status="200"}`)
	assert.Contains(t, genericOut, `rr_http_request_duration_seconds_count{status="200"}`)
	assert.Contains(t, genericOut, `rr_http_request_total{status="200"}`)
	assert.Contains(t, genericOut, "rr_http_workers_memory_bytes")
	assert.Contains(t, genericOut, `state="ready"}`)
	assert.Contains(t, genericOut, `{pid=`)
	assert.Contains(t, genericOut, `rr_http_total_workers 10`)

	close(sig)
	wg.Wait()

	require.Equal(t, 2, oLogger.FilterMessageSnippet("http log").Len())
}

func echoHTTP(port string) func(t *testing.T) {
	return func(t *testing.T) {
		req, err := http.NewRequest("GET", "http://127.0.0.1:"+port, nil)
		assert.NoError(t, err)

		r, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_, err = io.ReadAll(r.Body)
		assert.NoError(t, err)

		err = r.Body.Close()
		assert.NoError(t, err)
	}
}

func TestHTTPMetricsNoFreeWorkers(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.9.0"}
	cfg.Prefix = "rr"
	cfg.Path = "configs/.rr-http-metrics-no-free-workers.yaml"

	err := cont.RegisterAll(
		cfg,
		&metrics.Plugin{},
		&server.Plugin{},
		&httpPlugin.Plugin{},
		&logger.Plugin{},
		&prometheus.Plugin{},
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	tt := time.NewTimer(time.Minute * 3)

	go func() {
		defer tt.Stop()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-tt.C:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)
	go func() {
		t.Run("req_slow", echoHTTP("15442"))
	}()
	time.Sleep(time.Second * 2)
	t.Run("req2", echoHTTP("15442"))

	genericOut, err := get2()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, `rr_http_requests_queue`)
	assert.Contains(t, genericOut, `rr_http_no_free_workers_total 1`)

	close(sig)
}

func TestUnregister(t *testing.T) {
	cont := endure.New(slog.LevelDebug)

	cfg := &config.Plugin{
		Version: "2.12.0",
	}
	cfg.Prefix = "rr"
	cfg.Path = "configs/.rr-test.yaml"

	l, oLogger := mocklogger.ZapTestLogger(zap.DebugLevel)
	err := cont.RegisterAll(
		cfg,
		&metrics.Plugin{},
		&rpcPlugin.Plugin{},
		l,
	)
	assert.NoError(t, err)

	err = cont.Init()
	if err != nil {
		t.Fatal(err)
	}

	ch, err := cont.Serve()
	assert.NoError(t, err)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	tt := time.NewTimer(time.Minute * 3)
	defer tt.Stop()
	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			select {
			case e := <-ch:
				assert.Fail(t, "error", e.Error.Error())
			case <-sig:
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			case <-tt.C:
				// timeout
				err = cont.Stop()
				if err != nil {
					assert.FailNow(t, "error", err.Error())
				}
				return
			}
		}
	}()

	time.Sleep(time.Second * 2)
	t.Run("DeclareMetric", declareMetricsTest)
	genericOut, err := getIPV6()
	assert.NoError(t, err)
	assert.Contains(t, genericOut, "test_metrics_named_collector")

	time.Sleep(time.Second * 2)
	t.Run("UnregisterMetric", unregisterMetric("test_metrics_named_collector"))
	genericOut, err = getIPV6()
	assert.NoError(t, err)
	assert.NotContains(t, genericOut, "test_metrics_named_collector")

	require.Equal(t, 1, oLogger.FilterMessageSnippet("collector was successfully unregistered").Len())
}

// get request and return body
func get() (string, error) {
	r, err := http.Get(getAddr)
	if err != nil {
		return "", err
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	err = r.Body.Close()
	if err != nil {
		return "", err
	}
	// unsafe
	return string(b), err
}

// get request and return body
func get2() (string, error) {
	r, err := http.Get(getAddr2)
	if err != nil {
		return "", err
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	err = r.Body.Close()
	if err != nil {
		return "", err
	}
	// unsafe
	return string(b), err
}

// get request and return body
func getIPV6() (string, error) {
	r, err := http.Get(getIPV6Addr)
	if err != nil {
		return "", err
	}

	b, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	err = r.Body.Close()
	if err != nil {
		return "", err
	}
	// unsafe
	return string(b), err
}
