package prom

import (
	"github.com/prometheus/client_golang/prometheus"
	_ "github.com/prometheus/client_golang/prometheus/promhttp"

	"time"
)

var (
	rtcodeList         = []string{"r", "t", "code"}
	rtcodeDurationList = []float64{0.03, 0.2, 0.5, 1.0, 3.0, 10.0}
	rtcodeBytesList    = []float64{128, 512, 1024, 4096, 10240, 102400, 1024000, 2048000}

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
	promInit = true
}

type PromTrace struct {
	R         string
	T         string
	Code      string
	startTime time.Time
}

func NewPromTrace(r, t string) *PromTrace {
	return &PromTrace{
		R:         r,
		T:         t,
		startTime: time.Now(),
	}
}

func NewPromTraceWithCode(r, t, code string) *PromTrace {
	return &PromTrace{
		R:         r,
		T:         t,
		Code:      code,
		startTime: time.Now(),
	}
}

func (p *PromTrace) Cost() float64 {
	return time.Since(p.startTime).Seconds()
}

func (p *PromTrace) SetCode(code string) {
	p.Code = code
}

// SysCounts sys counts
func (p *PromTrace) SysCounts() {
	if promInit {
		rtcodeSysCounts.WithLabelValues(p.R, p.T, p.Code).Inc()
	}
}

func (p *PromTrace) SysDurations() {
	if promInit {
		rtcodeSysDurations.WithLabelValues(p.R, p.T, p.Code).Observe(p.Cost())
	}
}

// ReqCounts req_total
func (p *PromTrace) ReqCounts() {
	if promInit {
		rtcodeReqCounts.WithLabelValues(p.R, p.T, p.Code).Inc()
	}
}

// ReqDurations req_durations_seconds
func (p *PromTrace) ReqDurations() {
	if promInit {
		rtcodeReqDurations.WithLabelValues(p.R, p.T, p.Code).Observe(p.Cost())
	}
}

// RPCReqCounts rpc_req_total
func (p *PromTrace) RPCReqCounts() {
	if promInit {
		rtcodeRPCReqCounts.WithLabelValues(p.R, p.T, p.Code).Inc()
	}
}

// RPCDurations rpc_durations_seconds
func (p *PromTrace) RPCDurations() {
	if promInit {
		rtcodeRPCDurations.WithLabelValues(p.R, p.T, p.Code).Observe(p.Cost())
	}
}

// RPCBytes rpc_bytes
func (p *PromTrace) RPCBytes(bytes int64) {
	if promInit {
		rtcodeRPCBytes.WithLabelValues(p.R, p.T, p.Code).Observe(float64(bytes))
	}
}
