package buildkite

import (
	"os"
	"testing"
)

func TestGetOrganizationIDMissing(t *testing.T) {
	slug := "doesnt match API key"
	client := NewClient(slug, os.Getenv("BUILDKITE_API_TOKEN"), graphqlEndpoint, restEndpoint, "test-user-agent")

	id, err := GetOrganizationID(slug, client.graphql)
	if err == nil {
		t.Fatalf("err: %s", err)
	}
	if id != "" {
		t.Fatalf("Nonexistent organization found")
	}
}
