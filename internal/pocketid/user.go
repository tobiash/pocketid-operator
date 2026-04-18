package pocketid

import (
	"context"
	"fmt"
)

// ListUsers returns all users
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	var resp struct {
		Data []User `json:"data"`
	}
	if err := c.doRequest(ctx, "GET", "/api/users", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetUser returns a specific user by ID
func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	var user User
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/users/%s", id), nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateUser creates a new user
func (c *Client) CreateUser(ctx context.Context, user *User) (*User, error) {
	var createdUser User
	if err := c.doRequest(ctx, "POST", "/api/users", user, &createdUser); err != nil {
		return nil, err
	}
	return &createdUser, nil
}

// UpdateUser updates an existing user
func (c *Client) UpdateUser(ctx context.Context, id string, user *User) (*User, error) {
	var updatedUser User
	if err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/users/%s", id), user, &updatedUser); err != nil {
		return nil, err
	}
	return &updatedUser, nil
}

// DeleteUser deletes a user
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/users/%s", id), nil, nil)
}

// SendOnboardingEmail sends a one-time access email to a user
func (c *Client) SendOnboardingEmail(ctx context.Context, userID string) (*OnboardingResponse, error) {
	var resp OnboardingResponse
	if err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/users/%s/onboarding", userID), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
