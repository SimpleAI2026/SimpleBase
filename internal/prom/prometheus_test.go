package prom

import (
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

const (
	CodeCacheHit  = "cache_hit"
	CodeCacheMiss = "cache_miss"
)

func uniqLabel(prefix string) string {
	return prefix + strconv.FormatInt(time.Now().UnixNano(), 10)
}

func TestCostPositive(t *testing.T) {
	p := NewPromTrace("r", "t")
	time.Sleep(5 * time.Millisecond)
	if p.Cost() <= 0 {
		t.Fatalf("expected cost > 0, got %v", p.Cost())
	}
}

func TestSysCountsIncrement(t *testing.T) {
	r := uniqLabel("r-")
	tname := uniqLabel("t-")
	p := NewPromTraceWithCode(r, tname, CodeCacheHit)
	c := rtcodeSysCounts.WithLabelValues(r, tname, CodeCacheHit)
	before := testutil.ToFloat64(c)
	p.SysCounts()
	after := testutil.ToFloat64(c)
	if after != before+1 {
		t.Fatalf("expected sys_total increment by 1, before=%v after=%v", before, after)
	}
}

func TestReqCountsIncrement(t *testing.T) {
	r := uniqLabel("r-")
	tname := uniqLabel("t-")
	p := NewPromTraceWithCode(r, tname, CodeCacheMiss)
	c := rtcodeReqCounts.WithLabelValues(r, tname, CodeCacheMiss)
	before := testutil.ToFloat64(c)
	p.ReqCounts()
	after := testutil.ToFloat64(c)
	if after != before+1 {
		t.Fatalf("expected req_total increment by 1, before=%v after=%v", before, after)
	}
}

func TestRPCReqCountsIncrement(t *testing.T) {
	r := uniqLabel("r-")
	tname := uniqLabel("t-")
	p := NewPromTraceWithCode(r, tname, CodeCacheHit)
	c := rtcodeRPCReqCounts.WithLabelValues(r, tname, CodeCacheHit)
	before := testutil.ToFloat64(c)
	p.RPCReqCounts()
	after := testutil.ToFloat64(c)
	if after != before+1 {
		t.Fatalf("expected rpc_req_total increment by 1, before=%v after=%v", before, after)
	}
}

func TestRPCDurationsSeriesCreated(t *testing.T) {
	r := uniqLabel("r-")
	tname := uniqLabel("t-")
	p := NewPromTraceWithCode(r, tname, CodeCacheHit)
	before := testutil.CollectAndCount(rtcodeRPCDurations)
	time.Sleep(2 * time.Millisecond)
	p.RPCDurations()
	after := testutil.CollectAndCount(rtcodeRPCDurations)
	if after != before+1 {
		t.Fatalf("expected rpc_durations series count +1, before=%d after=%d", before, after)
	}
}

func TestRPCBytesSeriesCreated(t *testing.T) {
	r := uniqLabel("r-")
	tname := uniqLabel("t-")
	p := NewPromTraceWithCode(r, tname, CodeCacheHit)
	before := testutil.CollectAndCount(rtcodeRPCBytes)
	p.RPCBytes(1024)
	after := testutil.CollectAndCount(rtcodeRPCBytes)
	if after != before+1 {
		t.Fatalf("expected rpc_bytes series count +1, before=%d after=%d", before, after)
	}
}
