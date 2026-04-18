package pocketid

import (
	"context"
	"fmt"
)

// ListGroups returns all user groups
func (c *Client) ListGroups(ctx context.Context) ([]UserGroup, error) {
	var resp struct {
		Data []UserGroup `json:"data"`
	}
	if err := c.doRequest(ctx, "GET", "/api/groups", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

// GetGroup returns a specific group by ID
func (c *Client) GetGroup(ctx context.Context, id string) (*UserGroup, error) {
	var group UserGroup
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/groups/%s", id), nil, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// CreateGroup creates a new user group
func (c *Client) CreateGroup(ctx context.Context, group *UserGroup) (*UserGroup, error) {
	var createdGroup UserGroup
	if err := c.doRequest(ctx, "POST", "/api/groups", group, &createdGroup); err != nil {
		return nil, err
	}
	return &createdGroup, nil
}

// UpdateGroup updates an existing group
func (c *Client) UpdateGroup(ctx context.Context, id string, group *UserGroup) (*UserGroup, error) {
	var updatedGroup UserGroup
	if err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/groups/%s", id), group, &updatedGroup); err != nil {
		return nil, err
	}
	return &updatedGroup, nil
}

// DeleteGroup deletes a group
func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	return c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/groups/%s", id), nil, nil)
}

// AddUserToGroup adds a user to a group
func (c *Client) AddUserToGroup(ctx context.Context, groupID, userID string) error {
	body := map[string]string{"userId": userID}
	return c.doRequest(ctx, "POST", fmt.Sprintf("/api/groups/%s/members", groupID), body, nil)
}

// RemoveUserFromGroup removes a user from a group
func (c *Client) RemoveUserFromGroup(ctx context.Context, groupID, userID string) error {
	return c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/groups/%s/members/%s", groupID, userID), nil, nil)
}

// ListGroupMembers lists all users in a group
func (c *Client) ListGroupMembers(ctx context.Context, groupID string) ([]User, error) {
	var resp struct {
		Data []User `json:"data"`
	}
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/groups/%s/members", groupID), nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}
