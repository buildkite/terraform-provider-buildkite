package buildkite

import (
	"fmt"
	"sync"
	"testing"
)

var (
	resourceTrackingMutex sync.Mutex
	resourceTracking      = make(map[string]struct{})
)

func TrackResource(resourceType, resourceID string) {
	resourceTrackingMutex.Lock()
	defer resourceTrackingMutex.Unlock()

	key := fmt.Sprintf("%s:%s", resourceType, resourceID)
	resourceTracking[key] = struct{}{}
}

func UntrackResource(resourceType, resourceID string) {
	resourceTrackingMutex.Lock()
	defer resourceTrackingMutex.Unlock()

	key := fmt.Sprintf("%s:%s", resourceType, resourceID)
	delete(resourceTracking, key)
}

func CleanupResources(t *testing.T) {
	t.Helper()

	resourceTrackingMutex.Lock()
	defer resourceTrackingMutex.Unlock()

	if len(resourceTracking) > 0 {
		t.Logf("Resources that still need cleanup: %d", len(resourceTracking))

		for key := range resourceTracking {
			t.Logf("Resource needs manual cleanup: %s", key)
		}
	}
}

func RegisterResourceTracking(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		CleanupResources(t)
	})
}
