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

	client, err := NewClient(config)
	if err == nil {
		t.Fatalf("err: %s", err)
	}

	id, err := GetOrganizationID(slug, client.graphql)
	if err == nil {
		t.Fatalf("err: %s", err)
	}
	if id != "" {
		t.Fatalf("Nonexistent organization found")
	}
}
