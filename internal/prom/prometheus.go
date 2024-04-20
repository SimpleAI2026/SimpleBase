package prom

import (
	"github.com/linkxzhou/LessDB/internal/utils"
	"github.com/prometheus/client_golang/prometheus"

	_ "github.com/prometheus/client_golang/prometheus/promhttp"

	"strconv"
	"sync"
	"time"
)

const (
	RNameVFS = "vfs"

	TNameCacheGet = "cache_get"
	TNameCacheSize = "cache_size"
	TNameHTTPGet = "http_get"
	TNameHTTPSize = "http_size"

	CodeCacheMiss = "0"
	CodeCacheHit  = "1"
)

var (
	rtcodeList = []string{"r", "t", "code"}
	rtcodeDurationList = []float64{30.0, 100.0, 200.0, 500.0, 1000.0, 3000.0, 5000.0, 10000.0}
	rtcodeBytesList = []float64{32, 128, 512, 1024, 4196, 10240, 102400, 1024000, 2048000}	

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
			Buckets: rtcodeDurationList,
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
			Buckets: rtcodeDurationList,
		},
		rtcodeList,
	)

	rtcodeRPCReqCounts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rpc_req_total",
			Help: "Number of rpc request.",
		},
		rtcodeList,
	)

	rtcodeRPCDurations = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rpc_durations_seconds",
			Help:    "rpc latency distributions.",
			Buckets: rtcodeDurationList,
		},
		rtcodeList,
	)

	rtcodeRPCBytes = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rpc_bytes",
			Help:    "rpc bytes distributions.",
			Buckets: rtcodeBytesList,
		},
		rtcodeList,
	)

	promInit           bool = false
	refreshMetricsInit bool = false
)

func init() {
	prometheus.MustRegister(rtcodeSysCounts)
	prometheus.MustRegister(rtcodeSysDurations)
	prometheus.MustRegister(rtcodeReqCounts)
	prometheus.MustRegister(rtcodeReqDurations)
	prometheus.MustRegister(rtcodeRPCReqCounts)
	prometheus.MustRegister(rtcodeRPCDurations)
	prometheus.MustRegister(rtcodeRPCBytes)

	refreshMetricsTimer(rtcodeSysCounts.MetricVec, rtcodeReqCounts.MetricVec,
		rtcodeReqDurations.MetricVec, rtcodeRPCReqCounts.MetricVec,
		rtcodeRPCDurations.MetricVec, rtcodeRPCBytes.MetricVec)

	promInit = true
}

type PromTrace struct {
	R string
	T string
	Code string
	startTime time.Time
}

func NewPromTrace(r, t string) *PromTrace {
	return &PromTrace{
		R: r, 
		T: t,
		startTime: time.Now(),
	}
}

func (p *PromTrace) Cost() float64 {
	return float64(time.Now().Sub(p.startTime) / time.Millisecond)
}

func getPrometheusLabels(r, t, code string) prometheus.Labels {
	return prometheus.Labels{"r": r, "t": t, "code": code}
}

// SysCounts sys counts
func (p *PromTrace) SysCounts() {
	if promInit {
		rtcodeSysCounts.With(
			getPrometheusLabels(p.R, p.T, p.Code)).Inc()
	}
}

func (p *PromTrace) SysDurations() {
	if promInit {
		rtcodeSysDurations.With(
			getPrometheusLabels(p.R, p.T, p.Code)).Observe(p.Cost())
	}
}

// ReqCounts req_total
func (p *PromTrace) ReqCounts() {
	if promInit {
		rtcodeReqCounts.With(
			getPrometheusLabels(p.R, p.T, p.Code)).Inc()
	}
}

// ReqDurations req_durations_seconds
func (p *PromTrace) ReqDurations() {
	if promInit {
		rtcodeReqDurations.With(
			getPrometheusLabels(p.R, p.T, p.Code)).Observe(p.Cost())
	}
}

// RPCReqCounts rpc_req_total
func (p *PromTrace) RPCReqCounts() {
	if promInit {
		rtcodeRPCReqCounts.With(getPrometheusLabels(p.R, p.T, p.Code)).Inc()
	}
}

// RPCDurations rpc_durations_seconds
func (p *PromTrace) RPCDurations(r, t, code string, cost int64) {
	if promInit {
		rtcodeRPCDurations.With(
			getPrometheusLabels(p.R, p.T, p.Code)).Observe(p.Cost())
	}
}

// RPCBytes rpc_bytes
func (p *PromTrace) RPCBytes(bytes int64) {
	if promInit {
		rtcodeRPCBytes.With(
			getPrometheusLabels(p.R, p.T, p.Code)).Observe(float64(bytes))
	}
}

var refreshMetricsList []*prometheus.MetricVec
var refreshMetricsListMutex sync.Mutex

func refreshMetricsTimer(list ...*prometheus.MetricVec) {
	refreshMetricsPeriodSecond, err := 
		strconv.ParseInt(utils.GetEnviron("REFRESH_METRICS_PERIOD"), 10, 64)
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
				for _, metrics := range refreshMetricsList {
					if metrics != nil {
						metrics.Reset()
					}
				}
			}
		}()
	}
}
