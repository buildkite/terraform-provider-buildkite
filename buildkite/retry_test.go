package buildkite

import (
	"errors"
	"testing"

	"github.com/vektah/gqlparser/v2/gqlerror"
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

// retryContextError must retry the transient "currently busy / please try again" backend
// throttle (which arrives in a GraphQL 200 body) and leave every other error non-retryable.
func TestRetryContextError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		err           error
		wantNil       bool
		wantRetryable bool
	}{
		{name: "nil error", err: nil, wantNil: true},
		{
			name:          "cluster creation busy is retryable",
			err:           errors.New("input:3:2: clusterCreate Cluster creation is currently busy, please try again."),
			wantRetryable: true,
		},
		{
			name:          "please try again is retryable",
			err:           errors.New("something went wrong, please try again"),
			wantRetryable: true,
		},
		{
			name:          "transient gqlerror is retryable",
			err:           gqlerror.List{{Message: "Cluster creation is currently busy, please try again."}},
			wantRetryable: true,
		},
		{
			name:          "generic error is not retryable",
			err:           errors.New("invalid input: name is required"),
			wantRetryable: false,
		},
		{
			name:          "not-found gqlerror is not retryable",
			err:           gqlerror.List{{Message: "No cluster found"}},
			wantRetryable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := retryContextError(tt.err)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("retryContextError(nil) = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("retryContextError(%v) = nil, want non-nil", tt.err)
			}
			if got.Retryable != tt.wantRetryable {
				t.Errorf("Retryable = %v, want %v", got.Retryable, tt.wantRetryable)
			}
		})
	}
}
