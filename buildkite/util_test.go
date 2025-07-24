package buildkite

import (
	"errors"
	"os"
	"testing"
)

func TestGetOrganizationIDMissing(t *testing.T) {
	slug := "doesnt match API key"

	config := &clientConfig{
		org:        slug,
		apiToken:   os.Getenv("BUILDKITE_API_TOKEN"),
		graphqlURL: defaultGraphqlEndpoint,
		restURL:    defaultRestEndpoint,
		userAgent:  "test-user-agent",
	}

	client := NewClient(config)
	org, err := client.GetOrganizationID()
	if err == nil {
		t.Fatal("No error occurred")
	}
	if org != nil {
		t.Fatalf("Nonexistent organization found")
	}
}

func TestIsActiveJobsError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		shouldMatch bool
	}{
		{
			name:        "nil error",
			err:         nil,
			shouldMatch: false,
		},
		{
			name:        "active builds error",
			err:         errors.New("Cannot delete pipeline with active builds"),
			shouldMatch: true,
		},
		{
			name:        "running builds error",
			err:         errors.New("Pipeline has running builds"),
			shouldMatch: true,
		},
		{
			name:        "active jobs error",
			err:         errors.New("Cannot delete pipeline with active jobs"),
			shouldMatch: true,
		},
		{
			name:        "running jobs error",
			err:         errors.New("Pipeline has running jobs"),
			shouldMatch: true,
		},
		{
			name:        "builds are running error",
			err:         errors.New("builds are running"),
			shouldMatch: true,
		},
		{
			name:        "jobs are running error",
			err:         errors.New("jobs are running"),
			shouldMatch: true,
		},
		{
			name:        "case insensitive active builds",
			err:         errors.New("ACTIVE BUILDS prevent deletion"),
			shouldMatch: true,
		},
		{
			name:        "case insensitive running builds",
			err:         errors.New("Running Builds Found"),
			shouldMatch: true,
		},
		{
			name:        "unrelated error",
			err:         errors.New("Network connection failed"),
			shouldMatch: false,
		},
		{
			name:        "permission error",
			err:         errors.New("Insufficient permissions"),
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isActiveJobsError(tt.err)
			if result != tt.shouldMatch {
				t.Errorf("isActiveJobsError(%v) = %v, want %v", tt.err, result, tt.shouldMatch)
			}
		})
	}

	// Intentional failing assertion to test GitHub backlink formatting in fork
	t.Errorf("Testing GitHub backlink formatting for forked repository")
}
