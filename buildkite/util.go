package buildkite

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/shurcooL/graphql"
)

type RepositoryProvider string

const (
	RepositoryProviderGitHub    RepositoryProvider = "GITHUB"
	RepositoryProviderGitLab    RepositoryProvider = "GITLAB"
	RepositoryProviderBitbucket RepositoryProvider = "BITBUCKET"
	RepositoryProviderPrivate   RepositoryProvider = "PRIVATE"
)

func findRepositoryProvider(repo string) (RepositoryProvider, error) {
	r, err := repository.Parse(repo)
	if err != nil {
		return "", err
	}

	switch r.Host {
	case "github.com":
		return RepositoryProviderGitHub, nil
	case "bitbucket.org":
		return RepositoryProviderBitbucket, nil
	case "gitlab.com":
		return RepositoryProviderGitLab, nil
	default:
		return RepositoryProviderPrivate, nil
	}
}

// GetOrganizationID retrieves the Buildkite organization ID associated with the supplied slug
func GetOrganizationID(slug string, client *graphql.Client) (string, error) {
	var query struct {
		Organization struct {
			ID graphql.ID
		} `graphql:"organization(slug: $slug)"`
	}
	vars := map[string]interface{}{
		"slug": slug,
	}
	err := client.Query(context.Background(), &query, vars)
	if err != nil {
		return "", err
	}

	id, ok := query.Organization.ID.(string)
	if !ok {
		return "", fmt.Errorf("organization %s not found", slug)
	}

	return id, nil
}

// GetTeamID retrieves the Buildkite team ID associated with the supplied team slug
func GetTeamID(slug string, client *Client) (string, error) {
	// Make sure the slug is prefixed with the organization
	prefix := fmt.Sprintf("%s/", client.organization)
	if !strings.HasPrefix(slug, prefix) {
		slug = prefix + slug
	}
	var query struct {
		Team TeamNode `graphql:"team(slug: $slug)"`
	}
	params := map[string]interface{}{
		"slug": graphql.ID(slug),
	}
	err := client.graphql.Query(context.Background(), &query, params)
	if err != nil {
		return "", err
	}
	id := string(query.Team.ID)
	log.Printf("Found id '%s' for team '%s'.", id, slug)
	return id, nil
}

func getenv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return os.Getenv("BUILDKITE_ORGANIZATION")
	}
	return val
}

func retryContextError(err error) *retry.RetryError {
	if err != nil {
		if isRetryableError(err) {
			return retry.RetryableError(err)
		}
		return retry.NonRetryableError(err)
	}
	return nil
}
