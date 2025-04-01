package buildkite

import (
	"testing"
)

func TestResourceTracking(t *testing.T) {
	RegisterResourceTracking(t)

	TrackResource("test_resource", "1")
	TrackResource("test_resource", "2")
	TrackResource("another_resource", "1")

	resources := CleanupResources()
	if len(resources) != 2 {
		t.Errorf("Expected 2 resource types, got %d", len(resources))
	}
	if len(resources["test_resource"]) != 2 {
		t.Errorf("Expected 2 test_resources, got %d", len(resources["test_resource"]))
	}
	if len(resources["another_resource"]) != 1 {
		t.Errorf("Expected 1 another_resource, got %d", len(resources["another_resource"]))
	}

	UntrackResource("test_resource", "1")

	resources = CleanupResources()
	if len(resources["test_resource"]) != 1 {
		t.Errorf("Expected 1 test_resource after untracking, got %d", len(resources["test_resource"]))
	}
	if resources["test_resource"][0] != "2" {
		t.Errorf("Expected remaining test_resource to be '2', got '%s'", resources["test_resource"][0])
	}
}

func TestIsTestResource(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
	}{
		{"test-resource", true},
		{"Test_resource", true},
		{"acc-resource", true},
		{"tf-acc-resource", true},
		{"tf-test-resource", true},
		{"acceptance-resource", true},
		{"resource with acc test in it", true},
		{"production-resource", false},
		{"", false},
	}

	for _, tc := range testCases {
		result := isTestResource(tc.name)
		if result != tc.expected {
			t.Errorf("isTestResource(%s) = %v, expected %v", tc.name, result, tc.expected)
		}
	}
}
