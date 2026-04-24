package pocketid

import (
	"context"
	"fmt"
)

func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	var resp PaginatedResponse[User]
	if err := c.doRequest(ctx, "GET", "/api/users", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Data, nil
}

func (c *Client) GetUser(ctx context.Context, id string) (*User, error) {
	var user User
	if err := c.doRequest(ctx, "GET", fmt.Sprintf("/api/users/%s", id), nil, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

func (c *Client) CreateUser(ctx context.Context, user *User) (*User, error) {
	var createdUser User
	if err := c.doRequest(ctx, "POST", "/api/users", user, &createdUser); err != nil {
		return nil, err
	}
	return &createdUser, nil
}

func (c *Client) UpdateUser(ctx context.Context, id string, user *User) (*User, error) {
	var updatedUser User
	if err := c.doRequest(ctx, "PUT", fmt.Sprintf("/api/users/%s", id), user, &updatedUser); err != nil {
		return nil, err
	}
	return &updatedUser, nil
}

func (c *Client) DeleteUser(ctx context.Context, id string) error {
	return c.doRequest(ctx, "DELETE", fmt.Sprintf("/api/users/%s", id), nil, nil)
}

func (c *Client) CreateOneTimeAccessToken(ctx context.Context, userID string) (*OnboardingResponse, error) {
	var resp OnboardingResponse
	if err := c.doRequest(ctx, "POST", fmt.Sprintf("/api/users/%s/one-time-access-token", userID), nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) SetUserGroups(ctx context.Context, userID string, groupIDs []string) error {
	body := map[string][]string{"userGroupIds": groupIDs}
	return c.doRequest(ctx, "PUT", fmt.Sprintf("/api/users/%s/user-groups", userID), body, nil)
}
