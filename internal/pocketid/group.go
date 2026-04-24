package pocketid

import (
	"context"
	"fmt"
)

func (c *Client) ListGroups(ctx context.Context) ([]UserGroup, error) {
	var resp PaginatedResponse[UserGroup]
	if err := c.doRequest(ctx, "GET", "/api/user-groups", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (c *Client) GetGroup(ctx context.Context, id string) (*UserGroup, error) {
	var group UserGroup
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/user-groups/%s", id), nil, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

func (c *Client) CreateGroup(ctx context.Context, group *UserGroupCreate) (*UserGroup, error) {
	var createdGroup UserGroup
	if err := c.doRequest(ctx, "POST", "/api/user-groups", group, &createdGroup); err != nil {
		return nil, err
	}
	return &createdGroup, nil
}

func (c *Client) UpdateGroup(ctx context.Context, id string, group *UserGroupCreate) (*UserGroup, error) {
	var updatedGroup UserGroup
	if err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/user-groups/%s", id), group, &updatedGroup); err != nil {
		return nil, err
	}
	return &updatedGroup, nil
}

func (c *Client) DeleteGroup(ctx context.Context, id string) error {
	return c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/user-groups/%s", id), nil, nil)
}

func (c *Client) SetGroupUsers(ctx context.Context, groupID string, userIDs []string) error {
	body := map[string][]string{"userIds": userIDs}
	return c.doRequest(ctx, "PUT", fmt.Sprintf("/api/user-groups/%s/users", groupID), body, nil)
}
