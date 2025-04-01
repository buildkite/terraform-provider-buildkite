# Test Cleanup System

This document explains the resource tracking and cleanup system that helps prevent resources from being left behind during acceptance tests.

## Overview

The test cleanup system tracks resources created during acceptance tests and ensures they are properly cleaned up, even when tests fail unexpectedly. It provides a simple way to register resources for tracking and automatically untrack them when they are destroyed.

## Core Components

### Resource Tracking Map

The system maintains a thread-safe map to keep track of created resources:

```go
var (
    resourceTrackingMutex sync.Mutex
    resourceTracking      = make(map[string]struct{})
)
```

### Key Functions

1. **TrackResource**: Adds a resource to the tracking map by type and ID
   ```go
   func TrackResource(resourceType, resourceID string) {
       resourceTrackingMutex.Lock()
       defer resourceTrackingMutex.Unlock()
   
       key := fmt.Sprintf("%s:%s", resourceType, resourceID)
       resourceTracking[key] = struct{}{}
   }
   ```

2. **UntrackResource**: Removes a resource from tracking when it's destroyed
   ```go
   func UntrackResource(resourceType, resourceID string) {
       resourceTrackingMutex.Lock()
       defer resourceTrackingMutex.Unlock()
       
       key := fmt.Sprintf("%s:%s", resourceType, resourceID)
       delete(resourceTracking, key)
   }
   ```

3. **CleanupResources**: Logs any resources that weren't properly cleaned up
   ```go
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
   ```

4. **RegisterResourceTracking**: Sets up cleanup to run at the end of a test
   ```go
   func RegisterResourceTracking(t *testing.T) {
       t.Helper()
       
       t.Cleanup(func() {
           CleanupResources(t)
       })
   }
   ```

## How to Use the System

### Step 1: Register Resource Tracking

At the beginning of each test function, call `RegisterResourceTracking(t)`:

```go
func TestAccBuildkiteResource(t *testing.T) {
    RegisterResourceTracking(t)
    // rest of test...
}
```

### Step 2: Track Resources in Existence Checks

In your resource existence check functions, add a call to `TrackResource`:

```go
func testAccCheckResourceExists(name string, resource *ResourceModel) resource.TestCheckFunc {
    return func(s *terraform.State) error {
        rs, ok := s.RootModule().Resources[name]
        if !ok {
            return fmt.Errorf("Not found: %s", name)
        }
        
        // Track this resource for cleanup
        TrackResource("buildkite_resource_type", rs.Primary.ID)
        
        // Rest of existence check...
    }
}
```

### Step 3: Untrack Resources in Destroy Functions

In your destroy verification functions, call `UntrackResource` for resources that no longer exist:

```go
func testAccCheckResourceDestroy(s *terraform.State) error {
    for _, rs := range s.RootModule().Resources {
        if rs.Type != "buildkite_resource_type" {
            continue
        }
        
        // Check if resource is actually gone...
        
        // If it's gone, untrack it
        UntrackResource("buildkite_resource_type", rs.Primary.ID)
    }
    return nil
}
```

## Benefits

1. **Automatic Cleanup**: Resources are tracked even if tests fail unexpectedly
2. **Visibility**: Logs resources that weren't properly cleaned up
3. **Thread-safe**: Works with parallel test execution
4. **Simple**: Easy to integrate into existing tests

## Implementation Status

The system has been implemented for these resource types:

- Agent tokens
- Clusters and cluster queues
- Pipelines and pipeline templates
- Pipeline schedules
- Teams and team members
- Test suites

## Troubleshooting

If resources are still being left behind:

1. Verify that `RegisterResourceTracking(t)` is called at the beginning of test functions
2. Check that `TrackResource()` is called in resource existence functions
3. Ensure that `UntrackResource()` is called in destroy functions
4. Add verbosity to tests with `-v` to see which resources aren't being cleaned up