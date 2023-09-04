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

	// NewClient calls GetOrganizationId so we can test the output
	client, err := NewClient(config)
	if err == nil {
		t.Fatalf("err: %s", err)
	}
	if client != nil {
		t.Fatalf("Nonexistent organization found")
	}
}

func TestFindRepositoryProvider(t *testing.T) {
	t.Parallel()

	t.Run("resolve github", func(t *testing.T) {
		t.Parallel()
		testcases := map[string]string{
			"git url":                     "git@github.com:buildkite/terraform-provider-buildkite.git",
			"https url":                   "https://github.com/buildkite/terraform-provider-buildkite.git",
			"https url without extension": "https://github.com/user/bitbucket-git-repo",
			"https url with user":         "https://user@github.com/buildkite/terraform-provider-buildkite.git",
			"ssh url":                     "git://github.com/buildkite/terraform-provider-buildkite.git",
		}

		for name, input := range testcases {
			t.Run(name, func(t *testing.T) {
				input := input
				t.Parallel()

				provider, _ := findRepositoryProvider(input)

				if provider != RepositoryProviderGitHub {
					t.Errorf("provider does not match %s != %s", provider, RepositoryProviderGitHub)
				}
			})
		}
	})

	t.Run("resolve gitlab", func(t *testing.T) {
		t.Parallel()
		testcases := map[string]string{
			"git url":   "git@gitlab.com:foo/bar.git",
			"https url": "https://user@gitlab.com/user/gitlab-git-repo.git",
		}

		for name, input := range testcases {
			t.Run(name, func(t *testing.T) {
				input := input
				t.Parallel()

				provider, _ := findRepositoryProvider(input)

				if provider != RepositoryProviderGitLab {
					t.Errorf("provider does not match %s != %s", provider, RepositoryProviderGitHub)
				}
			})
		}
	})

	t.Run("resolve bitbucket urls", func(t *testing.T) {
		t.Parallel()
		testcases := map[string]string{
			"git url":   "git@bitbucket.org:foo/bar.git",
			"https url": "https://user@bitbucket.org/user/bitbucket-git-repo.git",
		}

		for name, input := range testcases {
			t.Run(name, func(t *testing.T) {
				input := input
				t.Parallel()

				provider, _ := findRepositoryProvider(input)

				if provider != RepositoryProviderBitbucket {
					t.Errorf("provider does not match %s != %s", provider, RepositoryProviderBitbucket)
				}
			})
		}
	})
}
