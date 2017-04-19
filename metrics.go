package service

import (
	"fmt"
	"net"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	metrics "github.com/rcrowley/go-metrics"
	"github.com/remerge/go-lock_free_timer"
)

var (
	memStats       runtime.MemStats
	runtimeMetrics struct {
		MemStats struct {
			Alloc         metrics.Gauge
			BuckHashSys   metrics.Gauge
			DebugGC       metrics.Gauge
			EnableGC      metrics.Gauge
			Frees         metrics.Gauge
			HeapAlloc     metrics.Gauge
			HeapIdle      metrics.Gauge
			HeapInuse     metrics.Gauge
			HeapObjects   metrics.Gauge
			HeapReleased  metrics.Gauge
			HeapSys       metrics.Gauge
			LastGC        metrics.Gauge
			Lookups       metrics.Gauge
			Mallocs       metrics.Gauge
			MCacheInuse   metrics.Gauge
			MCacheSys     metrics.Gauge
			MSpanInuse    metrics.Gauge
			MSpanSys      metrics.Gauge
			NextGC        metrics.Gauge
			NumGC         metrics.Gauge
			GCCPUFraction metrics.GaugeFloat64
			PauseNs       metrics.Histogram
			PauseTotalNs  metrics.Gauge
			StackInuse    metrics.Gauge
			StackSys      metrics.Gauge
			Sys           metrics.Gauge
			TotalAlloc    metrics.Gauge
		}
		NumCgoCall   metrics.Gauge
		NumGoroutine metrics.Gauge
		NumThread    metrics.Gauge
		ReadMemStats metrics.Timer
	}
	frees   uint64
	lookups uint64
	mallocs uint64
	numGC   uint32

	threadCreateProfile = pprof.Lookup("threadcreate")
)

// CaptureRuntimeMemStats captures new values for the Go runtime statistics
// exported in runtime.MemStats.  This is designed to be called as a goroutine.
func captureRuntimeMemStats(r metrics.Registry, d time.Duration) {
	for range time.Tick(d) {
		captureRuntimeMemStatsOnce(r)
	}
}

// Capture new values for the Go runtime statistics exported in
// runtime.MemStats.  This is designed to be called in a background goroutine.
// Giving a registry which has not been given to registerRuntimeMemStats will
// panic.
//
// Be very careful with this because runtime.ReadMemStats calls the C functions
// runtime·semacquire(&runtime·worldsema) and runtime·stoptheworld() and that
// last one does what it says on the tin.
func captureRuntimeMemStatsOnce(r metrics.Registry) {
	t := time.Now()
	runtime.ReadMemStats(&memStats) // This takes 50-200us.
	runtimeMetrics.ReadMemStats.UpdateSince(t)

	runtimeMetrics.MemStats.Alloc.Update(int64(memStats.Alloc))
	runtimeMetrics.MemStats.BuckHashSys.Update(int64(memStats.BuckHashSys))
	if memStats.DebugGC {
		runtimeMetrics.MemStats.DebugGC.Update(1)
	} else {
		runtimeMetrics.MemStats.DebugGC.Update(0)
	}
	if memStats.EnableGC {
		runtimeMetrics.MemStats.EnableGC.Update(1)
	} else {
		runtimeMetrics.MemStats.EnableGC.Update(0)
	}

	runtimeMetrics.MemStats.Frees.Update(int64(memStats.Frees - frees))
	runtimeMetrics.MemStats.HeapAlloc.Update(int64(memStats.HeapAlloc))
	runtimeMetrics.MemStats.HeapIdle.Update(int64(memStats.HeapIdle))
	runtimeMetrics.MemStats.HeapInuse.Update(int64(memStats.HeapInuse))
	runtimeMetrics.MemStats.HeapObjects.Update(int64(memStats.HeapObjects))
	runtimeMetrics.MemStats.HeapReleased.Update(int64(memStats.HeapReleased))
	runtimeMetrics.MemStats.HeapSys.Update(int64(memStats.HeapSys))
	runtimeMetrics.MemStats.LastGC.Update(int64(memStats.LastGC))
	runtimeMetrics.MemStats.Lookups.Update(int64(memStats.Lookups - lookups))
	runtimeMetrics.MemStats.Mallocs.Update(int64(memStats.Mallocs - mallocs))
	runtimeMetrics.MemStats.MCacheInuse.Update(int64(memStats.MCacheInuse))
	runtimeMetrics.MemStats.MCacheSys.Update(int64(memStats.MCacheSys))
	runtimeMetrics.MemStats.MSpanInuse.Update(int64(memStats.MSpanInuse))
	runtimeMetrics.MemStats.MSpanSys.Update(int64(memStats.MSpanSys))
	runtimeMetrics.MemStats.NextGC.Update(int64(memStats.NextGC))
	runtimeMetrics.MemStats.NumGC.Update(int64(memStats.NumGC))
	runtimeMetrics.MemStats.GCCPUFraction.Update(memStats.GCCPUFraction)

	// <https://code.google.com/p/go/source/browse/src/pkg/runtime/mgc0.c>
	i := numGC % uint32(len(memStats.PauseNs))
	ii := memStats.NumGC % uint32(len(memStats.PauseNs))
	if memStats.NumGC-numGC >= uint32(len(memStats.PauseNs)) {
		for i = 0; i < uint32(len(memStats.PauseNs)); i++ {
			runtimeMetrics.MemStats.PauseNs.Update(int64(memStats.PauseNs[i]))
		}
	} else {
		if i > ii {
			for ; i < uint32(len(memStats.PauseNs)); i++ {
				runtimeMetrics.MemStats.PauseNs.Update(int64(memStats.PauseNs[i]))
			}
			i = 0
		}
		for ; i < ii; i++ {
			runtimeMetrics.MemStats.PauseNs.Update(int64(memStats.PauseNs[i]))
		}
	}
	frees = memStats.Frees
	lookups = memStats.Lookups
	mallocs = memStats.Mallocs
	numGC = memStats.NumGC

	runtimeMetrics.MemStats.PauseTotalNs.Update(int64(memStats.PauseTotalNs))
	runtimeMetrics.MemStats.StackInuse.Update(int64(memStats.StackInuse))
	runtimeMetrics.MemStats.StackSys.Update(int64(memStats.StackSys))
	runtimeMetrics.MemStats.Sys.Update(int64(memStats.Sys))
	runtimeMetrics.MemStats.TotalAlloc.Update(int64(memStats.TotalAlloc))

	runtimeMetrics.NumCgoCall.Update(runtime.NumCgoCall())

	runtimeMetrics.NumGoroutine.Update(int64(runtime.NumGoroutine()))

	runtimeMetrics.NumThread.Update(int64(threadCreateProfile.Count()))
}

// Register runtimeMetrics for the Go runtime statistics exported in runtime
// and specifically runtime.MemStats.  The runtimeMetrics are named by their
// fully-qualified Go symbols, i.e. runtime.MemStats.Alloc.
func registerRuntimeMemStats(r metrics.Registry) {
	runtimeMetrics.MemStats.Alloc = metrics.NewGauge()
	runtimeMetrics.MemStats.BuckHashSys = metrics.NewGauge()
	runtimeMetrics.MemStats.DebugGC = metrics.NewGauge()
	runtimeMetrics.MemStats.EnableGC = metrics.NewGauge()
	runtimeMetrics.MemStats.Frees = metrics.NewGauge()
	runtimeMetrics.MemStats.HeapAlloc = metrics.NewGauge()
	runtimeMetrics.MemStats.HeapIdle = metrics.NewGauge()
	runtimeMetrics.MemStats.HeapInuse = metrics.NewGauge()
	runtimeMetrics.MemStats.HeapObjects = metrics.NewGauge()
	runtimeMetrics.MemStats.HeapReleased = metrics.NewGauge()
	runtimeMetrics.MemStats.HeapSys = metrics.NewGauge()
	runtimeMetrics.MemStats.LastGC = metrics.NewGauge()
	runtimeMetrics.MemStats.Lookups = metrics.NewGauge()
	runtimeMetrics.MemStats.Mallocs = metrics.NewGauge()
	runtimeMetrics.MemStats.MCacheInuse = metrics.NewGauge()
	runtimeMetrics.MemStats.MCacheSys = metrics.NewGauge()
	runtimeMetrics.MemStats.MSpanInuse = metrics.NewGauge()
	runtimeMetrics.MemStats.MSpanSys = metrics.NewGauge()
	runtimeMetrics.MemStats.NextGC = metrics.NewGauge()
	runtimeMetrics.MemStats.NumGC = metrics.NewGauge()
	runtimeMetrics.MemStats.GCCPUFraction = metrics.NewGaugeFloat64()
	runtimeMetrics.MemStats.PauseNs = metrics.NewHistogram(
		lft.NewLockFreeSample(1028))
	runtimeMetrics.MemStats.PauseTotalNs = metrics.NewGauge()
	runtimeMetrics.MemStats.StackInuse = metrics.NewGauge()
	runtimeMetrics.MemStats.StackSys = metrics.NewGauge()
	runtimeMetrics.MemStats.Sys = metrics.NewGauge()
	runtimeMetrics.MemStats.TotalAlloc = metrics.NewGauge()
	runtimeMetrics.NumCgoCall = metrics.NewGauge()
	runtimeMetrics.NumGoroutine = metrics.NewGauge()
	runtimeMetrics.NumThread = metrics.NewGauge()
	runtimeMetrics.ReadMemStats = lft.NewLockFreeTimer()

	r.Register("go.runtime mem_stat_alloc",
		runtimeMetrics.MemStats.Alloc)
	r.Register("go.runtime mem_stat_buck_hash_sys",
		runtimeMetrics.MemStats.BuckHashSys)
	r.Register("go.runtime mem_stat_debug_gc",
		runtimeMetrics.MemStats.DebugGC)
	r.Register("go.runtime mem_stat_enable_gc",
		runtimeMetrics.MemStats.EnableGC)
	r.Register("go.runtime mem_stat_frees",
		runtimeMetrics.MemStats.Frees)
	r.Register("go.runtime mem_stat_heap_alloc",
		runtimeMetrics.MemStats.HeapAlloc)
	r.Register("go.runtime mem_stat_heap_idle",
		runtimeMetrics.MemStats.HeapIdle)
	r.Register("go.runtime mem_stat_heap_inuse",
		runtimeMetrics.MemStats.HeapInuse)
	r.Register("go.runtime mem_stat_heap_objects",
		runtimeMetrics.MemStats.HeapObjects)
	r.Register("go.runtime mem_stat_heap_released",
		runtimeMetrics.MemStats.HeapReleased)
	r.Register("go.runtime mem_stat_heap_sys",
		runtimeMetrics.MemStats.HeapSys)
	r.Register("go.runtime mem_stat_last_gc",
		runtimeMetrics.MemStats.LastGC)
	r.Register("go.runtime mem_stat_lookups",
		runtimeMetrics.MemStats.Lookups)
	r.Register("go.runtime mem_stat_m_allocs",
		runtimeMetrics.MemStats.Mallocs)
	r.Register("go.runtime mem_stat_m_cache_inuse",
		runtimeMetrics.MemStats.MCacheInuse)
	r.Register("go.runtime mem_stat_m_cache_sys",
		runtimeMetrics.MemStats.MCacheSys)
	r.Register("go.runtime mem_stat_m_span_inuse",
		runtimeMetrics.MemStats.MSpanInuse)
	r.Register("go.runtime mem_stat_m_span_sys",
		runtimeMetrics.MemStats.MSpanSys)
	r.Register("go.runtime mem_stat_next_gc",
		runtimeMetrics.MemStats.NextGC)
	r.Register("go.runtime mem_stat_num_gc",
		runtimeMetrics.MemStats.NumGC)
	r.Register("go.runtime mem_stat_gc_cpu_fraction",
		runtimeMetrics.MemStats.GCCPUFraction)
	r.Register("go.runtime mem_stat_pause_ns",
		runtimeMetrics.MemStats.PauseNs)
	r.Register("go.runtime mem_stat_pause_total_ns",
		runtimeMetrics.MemStats.PauseTotalNs)
	r.Register("go.runtime mem_stat_stack_inuse",
		runtimeMetrics.MemStats.StackInuse)
	r.Register("go.runtime mem_stat_stack_sys",
		runtimeMetrics.MemStats.StackSys)
	r.Register("go.runtime mem_stat_sys",
		runtimeMetrics.MemStats.Sys)
	r.Register("go.runtime mem_stat_total_alloc",
		runtimeMetrics.MemStats.TotalAlloc)
	r.Register("go.runtime num_cgo_call",
		runtimeMetrics.NumCgoCall)
	r.Register("go.runtime num_goroutine",
		runtimeMetrics.NumGoroutine)
	r.Register("go.runtime num_thread",
		runtimeMetrics.NumThread)
	r.Register("go.runtime read_mem_stats",
		runtimeMetrics.ReadMemStats)
}

func (service *Service) flushMetrics(freq time.Duration) {
	registerRuntimeMemStats(metrics.DefaultRegistry)
	go captureRuntimeMemStats(metrics.DefaultRegistry, freq)

	raddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:8092")
	service.Log.Panic(err, "failed to resolve")

	laddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	service.Log.Panic(err, "failed to resolve")

	conn, err := net.DialUDP("udp", laddr, raddr)
	service.Log.Panic(err, "failed to connect to statsd")

	defer conn.Close()

	writeCb := func(format string, a ...interface{}) {
		msg := fmt.Sprintf(format, a...)
		conn.Write([]byte(msg))
	}

	for range time.Tick(freq) {
		ts := (time.Now().UnixNano() / int64(freq)) * int64(freq)
		metrics.DefaultRegistry.Each(func(name string, i interface{}) {
			service.flushMetric(name, i, ts, writeCb)
		})
	}
}

func (service *Service) flushMetric(
	name string,
	i interface{},
	ts int64,
	writeCb func(format string, a ...interface{}),
) {
	parts := strings.Split(name, " ")

	var prefix string
	if len(parts) > 1 {
		prefix = parts[1] + "_"
	}

	parts = strings.SplitN(parts[0], ",", 2)
	measurement := parts[0]
	tags := "service=" + service.Name

	if len(parts) > 1 {
		tags += "," + parts[1]
	}

	series := measurement + "," + tags

	switch metric := i.(type) {
	case metrics.Counter:
		writeCb("%s %scount=%di %d\n", series, prefix, metric.Count(), ts)
	case metrics.Gauge:
		writeCb("%s %svalue=%di %d\n", series, prefix, metric.Value(), ts)
	case metrics.GaugeFloat64:
		writeCb("%s %svalue=%f %d\n", series, prefix, metric.Value(), ts)
	case metrics.Healthcheck:
		metric.Check()
		writeCb("%s %serror=%s %d\n", series, prefix, metric.Error(), ts)
	case metrics.Histogram:
		s := metric.Snapshot()
		ps := s.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
		writeCb("%s %scount=%di %d\n", series, prefix, s.Count(), ts)
		writeCb("%s %smin=%di %d\n", series, prefix, s.Min(), ts)
		writeCb("%s %smax=%di %d\n", series, prefix, s.Max(), ts)
		writeCb("%s %smean=%f %d\n", series, prefix, s.Mean(), ts)
		writeCb("%s %sstddev=%f %d\n", series, prefix, s.StdDev(), ts)
		writeCb("%s %smedian=%f %d\n", series, prefix, ps[0], ts)
		writeCb("%s %sp75=%f %d\n", series, prefix, ps[1], ts)
		writeCb("%s %sp95=%f %d\n", series, prefix, ps[2], ts)
		writeCb("%s %sp99=%f %d\n", series, prefix, ps[3], ts)
		writeCb("%s %sp999=%f %d\n", series, prefix, ps[4], ts)
	case metrics.Meter:
		s := metric.Snapshot()
		writeCb("%s %scount=%di %d\n", series, prefix, s.Count(), ts)
		writeCb("%s %srate_m1=%f %d\n", series, prefix, s.Rate1(), ts)
		writeCb("%s %srate_m5=%f %d\n", series, prefix, s.Rate5(), ts)
		writeCb("%s %srate_m15=%f %d\n", series, prefix, s.Rate15(), ts)
		writeCb("%s %srate_mean=%f %d\n", series, prefix, s.RateMean(), ts)
	case metrics.Timer:
		s := metric.Snapshot()
		ps := s.Percentiles([]float64{0.5, 0.75, 0.95, 0.99, 0.999})
		writeCb("%s %scount=%di %d\n", series, prefix, s.Count(), ts)
		writeCb("%s %smin=%di %d\n", series, prefix, s.Min(), ts)
		writeCb("%s %smax=%di %d\n", series, prefix, s.Max(), ts)
		writeCb("%s %smean=%f %d\n", series, prefix, s.Mean(), ts)
		writeCb("%s %sstddev=%f %d\n", series, prefix, s.StdDev(), ts)
		writeCb("%s %smedian=%f %d\n", series, prefix, ps[0], ts)
		writeCb("%s %sp75=%f %d\n", series, prefix, ps[1], ts)
		writeCb("%s %sp95=%f %d\n", series, prefix, ps[2], ts)
		writeCb("%s %sp99=%f %d\n", series, prefix, ps[3], ts)
		writeCb("%s %sp999=%f %d\n", series, prefix, ps[4], ts)
		writeCb("%s %srate_m1=%f %d\n", series, prefix, s.Rate1(), ts)
		writeCb("%s %srate_m5=%f %d\n", series, prefix, s.Rate5(), ts)
		writeCb("%s %srate_m15=%f %d\n", series, prefix, s.Rate15(), ts)
		writeCb("%s %srate_mean=%f %d\n", series, prefix, s.RateMean(), ts)
	}
}
