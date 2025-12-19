package buildkite

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetClusterSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET request, got %s", r.Method)
		}
		if r.URL.Path != "/v2/organizations/test-org/clusters/cluster-123/secrets/secret-456" {
			t.Errorf("Expected path /v2/organizations/test-org/clusters/cluster-123/secrets/secret-456, got %s", r.URL.Path)
		}

		secret := ClusterSecret{
			ID:          "secret-456",
			Key:         "MY_SECRET",
			Description: "Test secret",
			Policy:      "- pipeline_slug: my-pipeline",
			CreatedAt:   "2024-01-01T00:00:00Z",
			UpdatedAt:   "2024-01-01T00:00:00Z",
		}
		json.NewEncoder(w).Encode(secret)
	}))
	defer server.Close()

	client := &Client{
		http:         server.Client(),
		restURL:      server.URL,
		organization: "test-org",
	}

	secret, err := client.GetClusterSecret(context.Background(), "test-org", "cluster-123", "secret-456")
	if err != nil {
		t.Fatalf("GetClusterSecret failed: %v", err)
	}

	if secret.ID != "secret-456" {
		t.Errorf("Expected ID secret-456, got %s", secret.ID)
	}
	if secret.Key != "MY_SECRET" {
		t.Errorf("Expected Key MY_SECRET, got %s", secret.Key)
	}
}

func TestCreateClusterSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/v2/organizations/test-org/clusters/cluster-123/secrets" {
			t.Errorf("Expected path /v2/organizations/test-org/clusters/cluster-123/secrets, got %s", r.URL.Path)
		}

		var input ClusterSecret
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		if input.Key != "MY_SECRET" {
			t.Errorf("Expected Key MY_SECRET, got %s", input.Key)
		}

		created := ClusterSecret{
			ID:          "secret-789",
			Key:         input.Key,
			Value:       input.Value,
			Description: input.Description,
			Policy:      input.Policy,
			CreatedAt:   "2024-01-01T00:00:00Z",
			UpdatedAt:   "2024-01-01T00:00:00Z",
		}
		json.NewEncoder(w).Encode(created)
	}))
	defer server.Close()

	client := &Client{
		http:         server.Client(),
		restURL:      server.URL,
		organization: "test-org",
	}

	secret := &ClusterSecret{
		Key:         "MY_SECRET",
		Value:       "secret-value",
		Description: "Test secret",
		Policy:      "- pipeline_slug: my-pipeline",
	}

	created, err := client.CreateClusterSecret(context.Background(), "test-org", "cluster-123", secret)
	if err != nil {
		t.Fatalf("CreateClusterSecret failed: %v", err)
	}

	if created.ID != "secret-789" {
		t.Errorf("Expected ID secret-789, got %s", created.ID)
	}
	if created.Key != "MY_SECRET" {
		t.Errorf("Expected Key MY_SECRET, got %s", created.Key)
	}
}

func TestUpdateClusterSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT request, got %s", r.Method)
		}
		if r.URL.Path != "/v2/organizations/test-org/clusters/cluster-123/secrets/secret-456" {
			t.Errorf("Expected path /v2/organizations/test-org/clusters/cluster-123/secrets/secret-456, got %s", r.URL.Path)
		}

		var updates map[string]string
		if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		if updates["description"] != "Updated description" {
			t.Errorf("Expected description 'Updated description', got %s", updates["description"])
		}

		updated := ClusterSecret{
			ID:          "secret-456",
			Key:         "MY_SECRET",
			Description: updates["description"],
			Policy:      updates["policy"],
			UpdatedAt:   "2024-01-02T00:00:00Z",
		}
		json.NewEncoder(w).Encode(updated)
	}))
	defer server.Close()

	client := &Client{
		http:         server.Client(),
		restURL:      server.URL,
		organization: "test-org",
	}

	updates := map[string]string{
		"description": "Updated description",
		"policy":      "- pipeline_slug: updated-pipeline",
	}

	updated, err := client.UpdateClusterSecret(context.Background(), "test-org", "cluster-123", "secret-456", updates)
	if err != nil {
		t.Fatalf("UpdateClusterSecret failed: %v", err)
	}

	if updated.Description != "Updated description" {
		t.Errorf("Expected description 'Updated description', got %s", updated.Description)
	}
}

func TestUpdateClusterSecretValue(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT request, got %s", r.Method)
		}
		if r.URL.Path != "/v2/organizations/test-org/clusters/cluster-123/secrets/secret-456/value" {
			t.Errorf("Expected path /v2/organizations/test-org/clusters/cluster-123/secrets/secret-456/value, got %s", r.URL.Path)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("Failed to decode request body: %v", err)
		}

		if payload["value"] != "new-secret-value" {
			t.Errorf("Expected value 'new-secret-value', got %s", payload["value"])
		}

		updated := ClusterSecret{
			ID:        "secret-456",
			Key:       "MY_SECRET",
			UpdatedAt: "2024-01-02T00:00:00Z",
		}
		json.NewEncoder(w).Encode(updated)
	}))
	defer server.Close()

	client := &Client{
		http:         server.Client(),
		restURL:      server.URL,
		organization: "test-org",
	}

	updated, err := client.UpdateClusterSecretValue(context.Background(), "test-org", "cluster-123", "secret-456", "new-secret-value")
	if err != nil {
		t.Fatalf("UpdateClusterSecretValue failed: %v", err)
	}

	if updated.ID != "secret-456" {
		t.Errorf("Expected ID secret-456, got %s", updated.ID)
	}
}

func TestDeleteClusterSecret(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/v2/organizations/test-org/clusters/cluster-123/secrets/secret-456" {
			t.Errorf("Expected path /v2/organizations/test-org/clusters/cluster-123/secrets/secret-456, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &Client{
		http:         server.Client(),
		restURL:      server.URL,
		organization: "test-org",
	}

	err := client.DeleteClusterSecret(context.Background(), "test-org", "cluster-123", "secret-456")
	if err != nil {
		t.Fatalf("DeleteClusterSecret failed: %v", err)
	}
}
