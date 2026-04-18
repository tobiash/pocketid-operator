package pocketid

import (
	"context"
	"fmt"
)

// ListOIDCClients returns all OIDC clients
func (c *Client) ListOIDCClients(ctx context.Context) ([]OIDCClient, error) {
	var resp struct {
		Data []OIDCClient `json:"data"`
	}
	if err := c.doRequest(ctx, "GET", "/api/oidc/clients", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetOIDCClient returns a specific OIDC client by ID
func (c *Client) GetOIDCClient(ctx context.Context, id string) (*OIDCClient, error) {
	var client OIDCClient
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/oidc/clients/%s", id), nil, &client); err != nil {
		return nil, err
	}
	return &client, nil
}

// CreateOIDCClient creates a new OIDC client
func (c *Client) CreateOIDCClient(ctx context.Context, client *OIDCClient) (*OIDCClient, error) {
	var createdClient OIDCClient
	if err := c.doRequest(ctx, "POST", "/api/oidc/clients", client, &createdClient); err != nil {
		return nil, err
	}
	return &createdClient, nil
}

// UpdateOIDCClient updates an existing OIDC client
func (c *Client) UpdateOIDCClient(ctx context.Context, id string, client *OIDCClient) (*OIDCClient, error) {
	var updatedClient OIDCClient
	if err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/oidc/clients/%s", id), client, &updatedClient); err != nil {
		return nil, err
	}
	return &updatedClient, nil
}

// DeleteOIDCClient deletes an OIDC client
func (c *Client) DeleteOIDCClient(ctx context.Context, id string) error {
	return c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/oidc/clients/%s", id), nil, nil)
}

// GenerateClientSecret generates a new secret for an OIDC client
// Warning: The secret is only returned once and cannot be retrieved later
func (c *Client) GenerateClientSecret(ctx context.Context, id string) (string, error) {
	var resp struct {
		ClientSecret string `json:"secret"`
	}
	// Assuming the endpoint is POST /api/oidc/clients/{id}/secret
	if err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/oidc/clients/%s/secret", id), nil, &resp); err != nil {
		return "", err
	}
	return resp.ClientSecret, nil
}
