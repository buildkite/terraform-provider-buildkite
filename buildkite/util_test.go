package buildkite

import (
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
