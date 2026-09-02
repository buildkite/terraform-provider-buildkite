package buildkite

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestResourceReadCacheCoalescesConcurrentLoads(t *testing.T) {
	t.Parallel()

	var cache resourceReadCache[int]
	var loadCount atomic.Int32
	loadStarted := make(chan struct{})
	releaseLoad := make(chan struct{})
	load := func() (int, error) {
		if loadCount.Add(1) == 1 {
			close(loadStarted)
		}
		<-releaseLoad
		return 42, nil
	}

	const callers = 10
	results := make(chan int, callers)
	errors := make(chan error, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	for range callers {
		go func() {
			ready.Done()
			value, err := cache.get(context.Background(), "shared", load)
			results <- value
			errors <- err
		}()
	}
	ready.Wait()
	<-loadStarted
	close(releaseLoad)

	for range callers {
		if err := <-errors; err != nil {
			t.Fatalf("cache get returned an error: %v", err)
		}
		if value := <-results; value != 42 {
			t.Fatalf("cache get returned %d, want 42", value)
		}
	}
	if got := loadCount.Load(); got != 1 {
		t.Fatalf("load called %d times, want 1", got)
	}
}

func TestResourceReadCacheInvalidate(t *testing.T) {
	t.Parallel()

	var cache resourceReadCache[int]
	var loadCount int
	load := func() (int, error) {
		loadCount++
		return loadCount, nil
	}

	first, err := cache.get(context.Background(), "key", load)
	if err != nil {
		t.Fatalf("first cache get returned an error: %v", err)
	}
	cache.invalidate("key")
	second, err := cache.get(context.Background(), "key", load)
	if err != nil {
		t.Fatalf("second cache get returned an error: %v", err)
	}

	if first != 1 || second != 2 {
		t.Fatalf("cache values were %d and %d, want 1 and 2", first, second)
	}
}

func TestResourceReadCacheDoesNotCacheErrors(t *testing.T) {
	t.Parallel()

	var cache resourceReadCache[int]
	var loadCount int
	load := func() (int, error) {
		loadCount++
		if loadCount == 1 {
			return 0, errors.New("temporary failure")
		}
		return 42, nil
	}

	if _, err := cache.get(context.Background(), "key", load); err == nil {
		t.Fatal("first cache get succeeded, want an error")
	}
	value, err := cache.get(context.Background(), "key", load)
	if err != nil {
		t.Fatalf("second cache get returned an error: %v", err)
	}
	if value != 42 {
		t.Fatalf("second cache get returned %d, want 42", value)
	}
}

func TestResourceReadCacheDoesNotStoreAnInvalidatedInFlightLoad(t *testing.T) {
	t.Parallel()

	var cache resourceReadCache[int]
	loadStarted := make(chan struct{})
	releaseLoad := make(chan struct{})
	firstResult := make(chan int)
	go func() {
		value, _ := cache.get(context.Background(), "key", func() (int, error) {
			close(loadStarted)
			<-releaseLoad
			return 1, nil
		})
		firstResult <- value
	}()

	<-loadStarted
	cache.invalidate("key")
	close(releaseLoad)
	if value := <-firstResult; value != 1 {
		t.Fatalf("in-flight caller received %d, want 1", value)
	}

	value, err := cache.get(context.Background(), "key", func() (int, error) {
		return 2, nil
	})
	if err != nil {
		t.Fatalf("cache get returned an error: %v", err)
	}
	if value != 2 {
		t.Fatalf("cache returned invalidated value %d, want 2", value)
	}
}
