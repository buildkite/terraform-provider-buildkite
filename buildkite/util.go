package buildkite

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/shurcooL/graphql"
)

var resourceNotFoundRegex = regexp.MustCompile(`(?i)(No\s+\w+(\s+\w+)*\s+found|not\s+found|no\s+longer\s+exists)`)

// isResourceNotFoundError returns true if the error indicates the resource was not found
func isResourceNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	return resourceNotFoundRegex.MatchString(err.Error())
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

func isUUID(uuid string) bool {
	r := regexp.MustCompile("^[a-f0-9]{8}(-[a-f0-9]{4}){3}-[a-f0-9]{12}$")
	return r.MatchString(uuid)
}

func getenv(key string) string {
	val, ok := os.LookupEnv(key)
	if !ok {
		return os.Getenv("BUILDKITE_ORGANIZATION")
	}
	return val
}

func createCidrSliceFromList(cidrList types.List) []string {
	cidrs := make([]string, len(cidrList.Elements()))
	for i, v := range cidrList.Elements() {
		cidrs[i] = strings.Trim(v.String(), "\"")
	}

	return cidrs
}

// retryContextError wraps an error for use with hashicorp/terraform-plugin-sdk/v2/helper/retry.
// Since the underlying http client now handles retries, we always treat errors as non-retryable
// at this layer to avoid duplicate retries.
func retryContextError(err error) *retry.RetryError {
	if err != nil {
		// Always return NonRetryableError as the http client handles retries
		return retry.NonRetryableError(err)
	}
	return nil
}
