package prom

import (
	"strconv"
	"sync"
	"time"

	"github.com/linkxzhou/LessDB/internal/utils"
	"github.com/prometheus/client_golang/prometheus"

	_ "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	rtcodeList = []string{"r", "t", "code"}

	rtcodeSysCounts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "sys_total",
			Help: "Number of request.",
		},
		rtcodeList,
	)

	rtcodeSysDurations = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "sys_durations_seconds",
			Help:    "latency distributions.",
			Buckets: []float64{5.0, 10.0, 20.0, 50.0, 100.0, 200.0, 500.0, 1000.0, 3000.0, 5000.0, 10000.0, 30000.0},
		},
		rtcodeList,
	)

	rtcodeReqCounts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "req_total",
			Help: "Number of request.",
		},
		rtcodeList,
	)

	rtcodeReqDurations = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "req_durations_seconds",
			Help:    "latency distributions.",
			Buckets: []float64{5.0, 10.0, 20.0, 50.0, 100.0, 200.0, 500.0, 1000.0, 3000.0, 5000.0, 10000.0, 30000.0},
		},
		rtcodeList,
	)

	rtcodeRpcReqCounts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_req_total",
			Help: "Number of rpc request.",
		},
		rtcodeList,
	)

	rtcodeRpcDurations = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rpc_durations_seconds",
			Help:    "rpc latency distributions.",
			Buckets: []float64{5.0, 10.0, 20.0, 50.0, 100.0, 200.0, 500.0, 1000.0, 3000.0, 5000.0, 10000.0, 30000.0},
		},
		rtcodeList,
	)

	rtcodeRpcBytes = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rpc_bytes",
			Help:    "rpc bytes distributions.",
			Buckets: []float64{4, 32, 64, 128, 512, 1024, 2048, 4196, 10240, 102400, 1024000, 2048000, 10240000},
		},
		rtcodeList,
	)

	promInit           bool = false
	refreshMetricsInit bool = false
)

func init() {
	prometheus.MustRegister(rtcodeSysCounts)
	prometheus.MustRegister(rtcodeReqCounts)
	prometheus.MustRegister(rtcodeReqDurations)
	prometheus.MustRegister(rtcodeRpcReqCounts)
	prometheus.MustRegister(rtcodeRpcDurations)
	prometheus.MustRegister(rtcodeRpcBytes)

	refreshMetricsTimer(rtcodeSysCounts.MetricVec, rtcodeReqCounts.MetricVec,
		rtcodeReqDurations.MetricVec, rtcodeRpcReqCounts.MetricVec,
		rtcodeRpcDurations.MetricVec, rtcodeRpcBytes.MetricVec)

	promInit = true
}

func getPrometheusLabels(r, t, code string, cost int64) prometheus.Labels {
	return prometheus.Labels{"r": r, "t": t, "code": code}
}

// SysCounts sys counts
func SysCounts(r, t, code string, cost int64) {
	if promInit {
		rtcodeSysCounts.With(getPrometheusLabels(r, t, code, cost)).Inc()
		if cost >= 0 {
			rtcodeSysDurations.With(getPrometheusLabels(r, t, code, cost)).Observe(float64(cost))
		}
	}
}

// ReqCounts req_total
func ReqCounts(r, t, code string, cost int64) {
	if promInit {
		rtcodeReqCounts.With(getPrometheusLabels(r, t, code, cost)).Inc()
		if cost >= 0 {
			rtcodeReqDurations.With(getPrometheusLabels(r, t, code, cost)).Observe(float64(cost))
		}
	}
}

// ReqDurations req_durations_seconds
func ReqDurations(r, t, code string, cost int64) {
	if promInit {
		rtcodeReqDurations.With(getPrometheusLabels(r, t, code, cost)).Observe(float64(cost))
	}
}

// RpcReqCounts rpc_req_total
func RpcReqCounts(r, t, code string, cost int64) {
	if promInit {
		rtcodeRpcReqCounts.With(getPrometheusLabels(r, t, code, cost)).Inc()
		if cost >= 0 {
			rtcodeRpcDurations.With(getPrometheusLabels(r, t, code, cost)).Observe(float64(cost))
		}
	}
}

// RpcDurations rpc_durations_seconds
func RpcDurations(r, t, code string, cost int64) {
	if promInit {
		rtcodeRpcDurations.With(getPrometheusLabels(r, t, code, cost)).Observe(float64(cost))
	}
}

// RpcBytes rpc_bytes
func RpcBytes(r, t, code string, bytes int64) {
	if promInit {
		rtcodeRpcBytes.With(getPrometheusLabels(r, t, code, 0)).Observe(float64(bytes))
	}
}

var refreshMetricsList []*prometheus.MetricVec
var refreshMetricsListMutex sync.Mutex

func refreshMetricsTimer(list ...*prometheus.MetricVec) {
	refreshMetricsPeriodSecond, err := strconv.ParseInt(utils.GetEnviron("REFRESH_METRICS_PERIODSECOND"), 10, 64)
	if err != nil || refreshMetricsPeriodSecond < 60 {
		return
	}

	refreshMetricsListMutex.Lock()
	defer refreshMetricsListMutex.Unlock()

	refreshMetricsList = append(refreshMetricsList, list...)
	if !refreshMetricsInit {
		refreshMetricsInit = true
		go func() {
			for {
				time.Sleep(time.Duration(refreshMetricsPeriodSecond) * time.Second)
				refreshMetricsListMutex.Lock()
				for _, metrics := range refreshMetricsList {
					if metrics != nil {
						metrics.Reset()
					}
				}
				refreshMetricsListMutex.Unlock()
			}
		}()
	}
}
