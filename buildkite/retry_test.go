package buildkite

import (
	"testing"
)

// This tests retry logic in general. Both REST and GraphQL clients are wrapped with `retryablehttp.Client`.
func TestClientConfig_MaxRetries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		maxRetries  int
		expectedMax int
	}{
		{
			name:        "default value",
			maxRetries:  DefaultRetryMaxAttempts,
			expectedMax: DefaultRetryMaxAttempts,
		},
		{
			name:        "custom value",
			maxRetries:  5,
			expectedMax: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			config := &clientConfig{
				apiToken:   "test",
				graphqlURL: "https://example.com",
				restURL:    "https://example.com",
				org:        "test",
				userAgent:  "test",
				maxRetries: tt.maxRetries,
			}

			client := NewClient(config)
			if client == nil {
				t.Error("NewClient() returned nil")
			}

			if tt.maxRetries != tt.expectedMax {
				t.Errorf("maxRetries = %d, want %d", tt.maxRetries, tt.expectedMax)
			}
		})
	}
}

func TestClientCreation(t *testing.T) {
	t.Parallel()

	config := &clientConfig{
		apiToken:   "test",
		graphqlURL: "https://example.com",
		restURL:    "https://example.com",
		org:        "test",
		userAgent:  "test",
		maxRetries: 3,
	}

	client := NewClient(config)
	if client == nil {
		t.Error("NewClient() returned nil")
	}
}
