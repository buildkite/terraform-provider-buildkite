package buildkite

import (
	"testing"
)

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
