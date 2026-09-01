package buildkite

import (
	"context"
	"sync"
	"time"
)

const resourceReadCacheTTL = 5 * time.Second

type cachedClusterQueue = getClusterQueuesOrganizationClusterQueuesClusterQueueConnectionEdgesClusterQueueEdgeNodeClusterQueue

type cachedPipelineSchedule = getPipelineSchedulesNodePipelineSchedulesPipelineScheduleConnectionEdgesPipelineScheduleEdgeNodePipelineSchedule

type cachedPipelineSchedules struct {
	schedules []cachedPipelineSchedule
	complete  bool
}

type resourceReadCache[V any] struct {
	mu       sync.Mutex
	entries  map[string]resourceReadCacheEntry[V]
	inFlight map[string]*resourceReadCacheCall[V]
	versions map[string]uint64
}

type resourceReadCacheEntry[V any] struct {
	value     V
	expiresAt time.Time
}

type resourceReadCacheCall[V any] struct {
	done    chan struct{}
	value   V
	err     error
	version uint64
}

func (cache *resourceReadCache[V]) get(
	ctx context.Context,
	key string,
	load func() (V, error),
) (V, error) {
	cache.mu.Lock()
	if entry, ok := cache.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		cache.mu.Unlock()
		return entry.value, nil
	}
	if call, ok := cache.inFlight[key]; ok {
		cache.mu.Unlock()
		select {
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		case <-call.done:
			return call.value, call.err
		}
	}

	if cache.inFlight == nil {
		cache.inFlight = make(map[string]*resourceReadCacheCall[V])
	}
	call := &resourceReadCacheCall[V]{
		done:    make(chan struct{}),
		version: cache.versions[key],
	}
	cache.inFlight[key] = call
	cache.mu.Unlock()

	call.value, call.err = load()

	cache.mu.Lock()
	delete(cache.inFlight, key)
	if call.err == nil && cache.versions[key] == call.version {
		if cache.entries == nil {
			cache.entries = make(map[string]resourceReadCacheEntry[V])
		}
		cache.entries[key] = resourceReadCacheEntry[V]{
			value:     call.value,
			expiresAt: time.Now().Add(resourceReadCacheTTL),
		}
	}
	close(call.done)
	cache.mu.Unlock()

	return call.value, call.err
}

func (cache *resourceReadCache[V]) invalidate(key string) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	delete(cache.entries, key)
	if cache.versions == nil {
		cache.versions = make(map[string]uint64)
	}
	cache.versions[key]++
}
