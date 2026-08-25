package buildkite

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/vektah/gqlparser/v2/gqlerror"
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

func TestIsResourceNotFoundError(t *testing.T) {
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
			name:        "not found plain error",
			err:         errors.New("Resource not found"),
			shouldMatch: true,
		},
		{
			name:        "no longer exists plain error",
			err:         errors.New("This resource no longer exists"),
			shouldMatch: true,
		},
		{
			name:        "gqlerror.List not found",
			err:         gqlerror.List{&gqlerror.Error{Message: "No Pipeline found"}},
			shouldMatch: true,
		},
		{
			name:        "gqlerror.List no longer exists",
			err:         gqlerror.List{&gqlerror.Error{Message: "This resource no longer exists"}},
			shouldMatch: true,
		},
		{
			name:        "gqlerror.List unrelated",
			err:         gqlerror.List{&gqlerror.Error{Message: "Network connection failed"}},
			shouldMatch: false,
		},
		{
			name:        "unrelated plain error",
			err:         errors.New("Network connection failed"),
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isResourceNotFoundError(tt.err)
			if result != tt.shouldMatch {
				t.Errorf("isResourceNotFoundError(%v) = %v, want %v", tt.err, result, tt.shouldMatch)
			}
		})
	}
}

func TestIsAlreadyExistsError(t *testing.T) {
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
			name:        "already been added error",
			err:         errors.New("This pipeline has already been added to this team"),
			shouldMatch: true,
		},
		{
			name:        "already exists error",
			err:         errors.New("Resource already exists"),
			shouldMatch: true,
		},
		{
			name:        "case insensitive already exists",
			err:         errors.New("ALREADY EXISTS in the system"),
			shouldMatch: true,
		},
		{
			name:        "unrelated error",
			err:         errors.New("Network connection failed"),
			shouldMatch: false,
		},
		{
			name:        "not found error",
			err:         errors.New("Resource not found"),
			shouldMatch: false,
		},
		{
			name:        "gqlerror.List already been added",
			err:         gqlerror.List{&gqlerror.Error{Message: "This pipeline has already been added to this team"}},
			shouldMatch: true,
		},
		{
			name:        "gqlerror.List unrelated",
			err:         gqlerror.List{&gqlerror.Error{Message: "Network connection failed"}},
			shouldMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isAlreadyExistsError(tt.err)
			if result != tt.shouldMatch {
				t.Errorf("isAlreadyExistsError(%v) = %v, want %v", tt.err, result, tt.shouldMatch)
			}
		})
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
		{
			name:        "gqlerror.List active builds",
			err:         gqlerror.List{&gqlerror.Error{Message: "Cannot delete pipeline with active builds"}},
			shouldMatch: true,
		},
		{
			name:        "gqlerror.List unrelated",
			err:         gqlerror.List{&gqlerror.Error{Message: "Insufficient permissions"}},
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
}

func TestSplitHumanImportID(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		id    string
		parts int
		want  []string
	}{
		{"my-pipeline", 1, []string{"my-pipeline"}},
		{"my-pipeline/deploy-team", 2, []string{"my-pipeline", "deploy-team"}},
		{"my-pipeline/0bd5ea7c-89b3-4f40-8ca3-ffac805771eb", 2, []string{"my-pipeline", "0bd5ea7c-89b3-4f40-8ca3-ffac805771eb"}},
		{"0bd5ea7c-89b3-4f40-8ca3-ffac805771eb/default", 2, []string{"0bd5ea7c-89b3-4f40-8ca3-ffac805771eb", "default"}},
		// GraphQL IDs and anything else are passed through
		{"UGlwZWxpbmUtLS00MzVjYWQ1OC1lODFkLTQ1YWYtODYzNy1iMWNmODA3MDIzOGQ=", 1, nil},
		{"VGVhbVBpcGVsaW5lLS0tMmQ5ZmRjYjctMjJjYS00ZDU3LTkwMWMtYmI3NzY1MmM5ZTk2", 1, nil},
		{"VGVhbVBpcGVsaW5lLS0tMmQ5ZmRjYjctMjJjYS00ZDU3LTkwMWMtYmI3NzY1MmM5ZTk2", 2, nil},
		{"my-pipeline", 2, nil},
		{"my-pipeline/deploy-team", 1, nil},
		{"my-pipeline/deploy-team/extra", 2, nil},
		{"My-Pipeline", 1, nil},
		{"", 1, nil},
		{"my-pipeline/", 2, nil},
		{"Q2x1c3RlclF1ZXVlLS0tNGM2YzNkYzEtM2Q5MC00NGQxLWIwNGMtNzBjYzRlZTg3NGJj,0bd5ea7c-89b3-4f40-8ca3-ffac805771eb", 2, nil},
	}

	for _, tc := range testCases {
		t.Run(tc.id, func(t *testing.T) {
			got, ok := splitHumanImportID(tc.id, tc.parts)
			if ok != (tc.want != nil) || strings.Join(got, "/") != strings.Join(tc.want, "/") {
				t.Errorf("splitHumanImportID(%q, %d) = %v, %t; want %v", tc.id, tc.parts, got, ok, tc.want)
			}
		})
	}
}
