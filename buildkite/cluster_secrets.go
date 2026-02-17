package buildkite

import (
	"context"
	"fmt"
	"net/http"
)

// ClusterSecret represents a Buildkite cluster secret
type ClusterSecret struct {
	ID          string  `json:"id,omitempty"`
	Key         string  `json:"key"`
	Value       string  `json:"value,omitempty"`
	Description *string `json:"description,omitempty"`
	Policy      *string `json:"policy,omitempty"`
	CreatedAt   string  `json:"created_at,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
	ClusterURL  string  `json:"cluster_url,omitempty"`
}

// GetClusterSecret retrieves a cluster secret by ID
func (c *Client) GetClusterSecret(ctx context.Context, orgSlug, clusterID, secretID string) (*ClusterSecret, error) {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/secrets/%s", orgSlug, clusterID, secretID)

	var secret ClusterSecret
	if err := c.makeRequest(ctx, http.MethodGet, path, nil, &secret); err != nil {
		return nil, err
	}

	return &secret, nil
}

// CreateClusterSecret creates a new cluster secret
func (c *Client) CreateClusterSecret(ctx context.Context, orgSlug, clusterID string, secret *ClusterSecret) (*ClusterSecret, error) {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/secrets", orgSlug, clusterID)

	var created ClusterSecret
	if err := c.makeRequest(ctx, http.MethodPost, path, secret, &created); err != nil {
		return nil, err
	}

	return &created, nil
}

// UpdateClusterSecret updates a secret's description and policy
func (c *Client) UpdateClusterSecret(ctx context.Context, orgSlug, clusterID, secretID string, updates map[string]string) (*ClusterSecret, error) {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/secrets/%s", orgSlug, clusterID, secretID)

	var updated ClusterSecret
	if err := c.makeRequest(ctx, http.MethodPut, path, updates, &updated); err != nil {
		return nil, err
	}

	return &updated, nil
}

// UpdateClusterSecretValue updates a secret's value
func (c *Client) UpdateClusterSecretValue(ctx context.Context, orgSlug, clusterID, secretID, value string) (*ClusterSecret, error) {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/secrets/%s/value", orgSlug, clusterID, secretID)

	payload := map[string]string{"value": value}

	var updated ClusterSecret
	if err := c.makeRequest(ctx, http.MethodPut, path, payload, &updated); err != nil {
		return nil, err
	}

	return &updated, nil
}

// DeleteClusterSecret deletes a cluster secret
func (c *Client) DeleteClusterSecret(ctx context.Context, orgSlug, clusterID, secretID string) error {
	path := fmt.Sprintf("/v2/organizations/%s/clusters/%s/secrets/%s", orgSlug, clusterID, secretID)
	return c.makeRequest(ctx, http.MethodDelete, path, nil, nil)
}
