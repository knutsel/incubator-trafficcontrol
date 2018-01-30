package stat

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/apache/incubator-trafficcontrol/grove/remap"
	"github.com/apache/incubator-trafficcontrol/grove/web"

	"github.com/apache/incubator-trafficcontrol/lib/go-log"
)

type Cache interface {
	Size() uint64
}

type StatsSystem interface {
	AddConfigReloadRequests()
	SetLastReloadRequest(time.Time)
	AddConfigReload()
	SetLastReload(time.Time)
	SetAstatsLoad(time.Time)

	ConfigReloadRequests() uint64
	LastReloadRequest() time.Time
	ConfigReloads() uint64
	LastReload() time.Time
	AstatsLoad() time.Time
}

type Stats interface {
	System() StatsSystem
	Remap() StatsRemaps

	Connections() uint64
	IncConnections()
	DecConnections()

	CacheHits() uint64
	AddCacheHit()
	CacheMisses() uint64
	AddCacheMiss()

	CacheSize() uint64
	CacheCapacity() uint64

	// Write writes to the remapRuleStats of s, and returns the bytes written to the connection
	Write(w http.ResponseWriter, conn *web.InterceptConn, reqFQDN string, remoteAddr string, code int, bytesWritten uint64, cacheHit bool) uint64
}

func New(remapRules []remap.RemapRule, cache Cache, cacheCapacityBytes uint64) Stats {
	connections := uint64(0)
	cacheHits := uint64(0)
	cacheMisses := uint64(0)
	return &stats{
		system:             NewStatsSystem(),
		remap:              NewStatsRemaps(remapRules),
		connections:        &connections,
		cacheHits:          &cacheHits,
		cacheMisses:        &cacheMisses,
		cache:              cache,
		cacheCapacityBytes: cacheCapacityBytes,
	}
}

// Write writes to the remapRuleStats of s, and returns the bytes written to the connection
func (stats *stats) Write(w http.ResponseWriter, conn *web.InterceptConn, reqFQDN string, remoteAddr string, code int, bytesWritten uint64, cacheHit bool) uint64 {
	remapRuleStats, ok := stats.Remap().Stats(reqFQDN)
	if !ok {
		log.Errorf("Remap rule %v not in Stats\n", reqFQDN)
		return bytesWritten
	}

	if wFlusher, ok := w.(http.Flusher); !ok {
		log.Errorf("ResponseWriter is not a Flusher, could not flush written bytes, stat out_bytes will be inaccurate!\n")
	} else {
		wFlusher.Flush()
	}

	bytesRead := 0 // TODO get somehow? Count body? Sum header?
	if conn != nil {
		bytesRead = conn.BytesRead()
		bytesWritten = uint64(conn.BytesWritten()) // get the more accurate interceptConn bytes written, if we can
		// Don't log - the Handler has already logged the failure to get the conn
	}

	// bytesRead, bytesWritten := getConnInfoAndDestroyWriter(w, stats, remapRuleName)
	remapRuleStats.AddInBytes(uint64(bytesRead))
	remapRuleStats.AddOutBytes(uint64(bytesWritten))

	if cacheHit {
		stats.AddCacheHit()
		remapRuleStats.AddCacheHit()
	} else {
		stats.AddCacheMiss()
		remapRuleStats.AddCacheMiss()
	}

	switch {
	case code < 200:
		log.Errorf("responded with invalid code %v\n", code)
	case code < 300:
		remapRuleStats.AddStatus2xx(1)
	case code < 400:
		remapRuleStats.AddStatus3xx(1)
	case code < 500:
		remapRuleStats.AddStatus4xx(1)
	case code < 600:
		remapRuleStats.AddStatus5xx(1)
	default:
		log.Errorf("responded with invalid code %v\n", code)
	}
	return bytesWritten
}

// stats fulfills the Stats interface
type stats struct {
	system             StatsSystem
	remap              StatsRemaps
	connections        *uint64
	cacheHits          *uint64
	cacheMisses        *uint64
	cache              Cache
	cacheCapacityBytes uint64
}

func (s stats) Connections() uint64   { return atomic.LoadUint64(s.connections) }
func (s stats) IncConnections()       { atomic.AddUint64(s.connections, 1) }
func (s stats) DecConnections()       { atomic.AddUint64(s.connections, ^uint64(0)) }
func (s stats) CacheHits() uint64     { return atomic.LoadUint64(s.cacheHits) }
func (s stats) AddCacheHit()          { atomic.AddUint64(s.cacheHits, 1) }
func (s stats) CacheMisses() uint64   { return atomic.LoadUint64(s.cacheMisses) }
func (s stats) AddCacheMiss()         { atomic.AddUint64(s.cacheMisses, 1) }
func (s *stats) System() StatsSystem  { return StatsSystem(s.system) }
func (s *stats) Remap() StatsRemaps   { return s.remap }
func (s stats) CacheSize() uint64     { return s.cache.Size() }
func (s stats) CacheCapacity() uint64 { return s.cacheCapacityBytes }

type StatsRemaps interface {
	Stats(fqdn string) (StatsRemap, bool)
	Rules() []string
}

type StatsRemap interface {
	InBytes() uint64
	AddInBytes(uint64)
	OutBytes() uint64
	AddOutBytes(uint64)
	Status2xx() uint64
	AddStatus2xx(uint64)
	Status3xx() uint64
	AddStatus3xx(uint64)
	Status4xx() uint64
	AddStatus4xx(uint64)
	Status5xx() uint64
	AddStatus5xx(uint64)

	CacheHits() uint64
	AddCacheHit()
	CacheMisses() uint64
	AddCacheMiss()
}

func getFromFQDN(r remap.RemapRule) string {
	path := r.From
	schemeEnd := `://`
	if i := strings.Index(path, schemeEnd); i != -1 {
		path = path[i+len(schemeEnd):]
	}
	pathStart := `/`
	if i := strings.Index(path, pathStart); i != -1 {
		path = path[:i]
	}
	return path
}

func NewStatsRemaps(remapRules []remap.RemapRule) StatsRemaps {
	m := make(map[string]StatsRemap, len(remapRules))
	for _, rule := range remapRules {
		m[getFromFQDN(rule)] = NewStatsRemap() // must pre-allocate, for threadsafety, so users are never changing the map itself, only the value pointed to.
	}
	return statsRemaps(m)
}

// statsRemaps fulfills the StatsRemaps interface
type statsRemaps map[string]StatsRemap

func (s statsRemaps) Stats(rule string) (StatsRemap, bool) {
	r, ok := s[rule]
	return r, ok
}

func (s statsRemaps) Rules() []string {
	rules := make([]string, len(s))
	for rule := range s {
		rules = append(rules, rule)
	}
	return rules
}

func NewStatsRemap() StatsRemap {
	return &statsRemap{}
}

type statsRemap struct {
	inBytes     uint64
	outBytes    uint64
	status2xx   uint64
	status3xx   uint64
	status4xx   uint64
	status5xx   uint64
	cacheHits   uint64
	cacheMisses uint64
}

func (r *statsRemap) InBytes() uint64       { return atomic.LoadUint64(&r.inBytes) }
func (r *statsRemap) AddInBytes(v uint64)   { atomic.AddUint64(&r.inBytes, v) }
func (r *statsRemap) OutBytes() uint64      { return atomic.LoadUint64(&r.outBytes) }
func (r *statsRemap) AddOutBytes(v uint64)  { atomic.AddUint64(&r.outBytes, v) }
func (r *statsRemap) Status2xx() uint64     { return atomic.LoadUint64(&r.status2xx) }
func (r *statsRemap) AddStatus2xx(v uint64) { atomic.AddUint64(&r.status2xx, v) }
func (r *statsRemap) Status3xx() uint64     { return atomic.LoadUint64(&r.status3xx) }
func (r *statsRemap) AddStatus3xx(v uint64) { atomic.AddUint64(&r.status3xx, v) }
func (r *statsRemap) Status4xx() uint64     { return atomic.LoadUint64(&r.status4xx) }
func (r *statsRemap) AddStatus4xx(v uint64) { atomic.AddUint64(&r.status4xx, v) }
func (r *statsRemap) Status5xx() uint64     { return atomic.LoadUint64(&r.status5xx) }
func (r *statsRemap) AddStatus5xx(v uint64) { atomic.AddUint64(&r.status5xx, v) }

func (r *statsRemap) CacheHits() uint64 { return atomic.LoadUint64(&r.cacheHits) }
func (r *statsRemap) AddCacheHit()      { atomic.AddUint64(&r.cacheHits, 1) }

func (r *statsRemap) CacheMisses() uint64 { return atomic.LoadUint64(&r.cacheMisses) }
func (r *statsRemap) AddCacheMiss()       { atomic.AddUint64(&r.cacheMisses, 1) }

func NewStatsSystem() StatsSystem {
	return &statsSystem{}
}

type statsSystem struct {
	configReloadRequests      uint64
	lastReloadRequestUnixNano int64
	configReloads             uint64
	lastReloadUnixNano        int64
	astatsLoadUnixNano        int64
}

func (s *statsSystem) ConfigReloadRequests() uint64 {
	return atomic.LoadUint64(&s.configReloadRequests)
}
func (s *statsSystem) AddConfigReloadRequests() {
	atomic.AddUint64(&s.configReloadRequests, 1)
}
func (s *statsSystem) LastReloadRequest() time.Time {
	return time.Unix(0, atomic.LoadInt64(&s.lastReloadRequestUnixNano))
}
func (s *statsSystem) SetLastReloadRequest(t time.Time) {
	atomic.StoreInt64(&s.lastReloadRequestUnixNano, t.UnixNano())
}
func (s *statsSystem) ConfigReloads() uint64 {
	return atomic.LoadUint64(&s.configReloads)
}
func (s *statsSystem) AddConfigReload() {
	atomic.AddUint64(&s.configReloads, 1)
}
func (s *statsSystem) LastReload() time.Time {
	return time.Unix(0, atomic.LoadInt64(&s.lastReloadUnixNano))
}
func (s *statsSystem) SetLastReload(t time.Time) {
	atomic.StoreInt64(&s.lastReloadUnixNano, t.UnixNano())
}
func (s *statsSystem) AstatsLoad() time.Time {
	return time.Unix(0, atomic.LoadInt64(&s.astatsLoadUnixNano))
}
func (s *statsSystem) SetAstatsLoad(t time.Time) {
	atomic.StoreInt64(&s.astatsLoadUnixNano, t.UnixNano())
}

const ATSVersion = "5.3.2" // of course, we're not really ATS. We're terrible liars.

// type StatsATSJSON struct {
// 	Server string            `json:"server"`
// 	Remap  map[string]uint64 `json:"remap"`
// }

type StatsSystemJSON struct {
	InterfaceName        string `json:"inf.name"`
	InterfaceSpeed       int64  `json:"inf.speed"`
	ProcNetDev           string `json:"proc.net.dev"`
	ProcLoadAvg          string `json:"proc.loadavg"`
	ConfigReloadRequests uint64 `json:"configReloadRequests"`
	LastReloadRequest    int64  `json:"lastReloadRequest"`
	ConfigReloads        uint64 `json:"configReloads"`
	LastReload           int64  `json:"lastReload"`
	AstatsLoad           int64  `json:"astatsLoad"`
	Something            string `json:"something"`
}

type StatsJSON struct {
	ATS    map[string]interface{} `json:"ats"`
	System StatsSystemJSON        `json:"system"`
}