package buildkite

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/shurcooL/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

var (
	resourceNotFoundRegex = regexp.MustCompile(`(?i)(No\s+\w+(\s+\w+)*\s+found|not\s+found|no\s+longer\s+exists)`)
	activeJobsRegex       = regexp.MustCompile(`(?i)(active\s+(builds|jobs)|running\s+(builds|jobs)|builds?\s+are\s+running|jobs?\s+are\s+running)`)
	alreadyExistsRegex    = regexp.MustCompile(`(?i)(already\s+been\s+added|already\s+exists)`)
	transientErrorRegex   = regexp.MustCompile(`(?i)currently\s+busy`)
)

// isResourceNotFoundError returns true if the error indicates the resource was not found
func isResourceNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, e := range errList {
			if resourceNotFoundRegex.MatchString(e.Message) {
				return true
			}
		}
		return false
	}
	return resourceNotFoundRegex.MatchString(err.Error())
}

// isActiveJobsError returns true if the error indicates the pipeline has active jobs/builds preventing deletion
func isActiveJobsError(err error) bool {
	if err == nil {
		return false
	}
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, e := range errList {
			if activeJobsRegex.MatchString(e.Message) {
				return true
			}
		}
		return false
	}
	return activeJobsRegex.MatchString(err.Error())
}

// isAlreadyExistsError returns true if the error indicates the resource already exists
func isAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, e := range errList {
			if alreadyExistsRegex.MatchString(e.Message) {
				return true
			}
		}
		return false
	}
	return alreadyExistsRegex.MatchString(err.Error())
}

// isTransientError returns true if the error is a transient, retryable backend condition
// (e.g. "clusterCreate Cluster creation is currently busy, please try again"). These arrive
// in a GraphQL 200 response body, so the retryablehttp client never retries them.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, e := range errList {
			if transientErrorRegex.MatchString(e.Message) {
				return true
			}
		}
		return false
	}
	return transientErrorRegex.MatchString(err.Error())
}

// gqlErrorContains reports whether any GraphQL error in err contains substring s.
// Falls back to strings.Contains(err.Error(), s) for non-gqlerror errors.
func gqlErrorContains(err error, s string) bool {
	if err == nil {
		return false
	}
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, e := range errList {
			if strings.Contains(e.Message, s) {
				return true
			}
		}
		return false
	}
	return strings.Contains(err.Error(), s)
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
// The underlying http client already retries transport and status errors, so we treat errors as
// non-retryable here to avoid duplicate retries. The exception is transient backend errors that
// arrive in a GraphQL 200 body (e.g. "... is currently busy, please try again"): the http client
// can't act on those, so we mark them retryable and let RetryContext back off within its timeout.
func retryContextError(err error) *retry.RetryError {
	if err == nil {
		return nil
	}
	if isTransientError(err) {
		return retry.RetryableError(err)
	}
	return retry.NonRetryableError(err)
}
