package buildkite

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var errOrganizationMembershipNotFound = errors.New("organization member or invitation not found")

type organizationMembershipMember struct {
	ID      string  `json:"id"`
	Email   string  `json:"email"`
	Role    string  `json:"role"`
	SSOMode *string `json:"sso_mode"`
}

type organizationMembershipInvitation struct {
	ID         string                        `json:"id"`
	Email      string                        `json:"email"`
	State      string                        `json:"state"`
	Role       string                        `json:"role"`
	SSOMode    string                        `json:"sso_mode"`
	AcceptedBy *organizationMembershipMember `json:"accepted_by"`
	Teams      []struct {
		ID string `json:"id"`
	} `json:"teams"`
}

func (c *Client) organizationMembershipPath(collection, id string) string {
	p := "/v2/organizations/" + url.PathEscape(c.organization) + "/" + collection
	if id != "" {
		p += "/" + url.PathEscape(id)
	}
	return p
}

func (c *Client) getOrganizationMembershipMember(ctx context.Context, id string) (*organizationMembershipMember, error) {
	var member organizationMembershipMember
	err := c.makeRequest(ctx, http.MethodGet, c.organizationMembershipPath("members", id), nil, &member)
	if isAPIStatus(err, http.StatusNotFound) {
		return nil, errOrganizationMembershipNotFound
	}
	return &member, err
}

func (c *Client) findOrganizationMembershipMember(ctx context.Context, email string) (*organizationMembershipMember, error) {
	for page := 1; ; page++ {
		var members []organizationMembershipMember
		p := fmt.Sprintf("%s?per_page=100&page=%d", c.organizationMembershipPath("members", ""), page)
		if err := c.makeRequest(ctx, http.MethodGet, p, nil, &members); err != nil {
			return nil, err
		}
		for _, member := range members {
			if strings.EqualFold(member.Email, email) {
				return &member, nil
			}
		}
		if len(members) < 100 {
			return nil, errOrganizationMembershipNotFound
		}
	}
}

func (c *Client) getOrganizationMembershipInvitation(ctx context.Context, id string) (*organizationMembershipInvitation, error) {
	var invitation organizationMembershipInvitation
	err := c.makeRequest(ctx, http.MethodGet, c.organizationMembershipPath("invitations", id), nil, &invitation)
	if isAPIStatus(err, http.StatusNotFound) {
		return nil, errOrganizationMembershipNotFound
	}
	return &invitation, err
}

func (c *Client) findOrganizationMembershipInvitation(ctx context.Context, email string) (*organizationMembershipInvitation, error) {
	base := c.organizationMembershipPath("invitations", "")
	for next := base + "?per_page=100"; next != ""; {
		var page struct {
			Items []organizationMembershipInvitation `json:"items"`
			Links struct {
				Next string `json:"next"`
			} `json:"links"`
		}
		if err := c.makeRequest(ctx, http.MethodGet, next, nil, &page); err != nil {
			return nil, err
		}
		for _, invitation := range page.Items {
			if invitation.State == "pending" && strings.EqualFold(invitation.Email, email) {
				return &invitation, nil
			}
		}
		if page.Links.Next == "" {
			break
		}
		u, err := url.Parse(page.Links.Next)
		if err != nil || u.Path != base || u.RequestURI() == next {
			return nil, fmt.Errorf("invalid invitation pagination link")
		}
		// Keep requests on the configured REST host; use the API's cursor unchanged.
		next = u.RequestURI()
	}
	return nil, errOrganizationMembershipNotFound
}

func (c *Client) createOrganizationMembershipInvitation(ctx context.Context, email, role, ssoMode string) (*organizationMembershipInvitation, error) {
	var invitations []organizationMembershipInvitation
	err := c.makeRequest(ctx, http.MethodPost, c.organizationMembershipPath("invitations", ""), map[string]any{
		"emails": []string{email}, "role": strings.ToLower(role), "sso_mode": strings.ToLower(ssoMode),
	}, &invitations)
	if err != nil {
		return nil, err
	}
	if len(invitations) != 1 || invitations[0].ID == "" {
		return nil, fmt.Errorf("expected one created invitation in the API response")
	}
	return &invitations[0], nil
}

func (c *Client) updateOrganizationMembershipMember(ctx context.Context, member *organizationMembershipMember, role, ssoMode string) (*organizationMembershipMember, error) {
	payload := map[string]string{}
	if role != member.Role {
		payload["role"] = role
	}
	if ssoMode != "" && (member.SSOMode == nil || ssoMode != *member.SSOMode) {
		payload["sso_mode"] = ssoMode
	}
	if len(payload) == 0 {
		return member, nil
	}
	var updated organizationMembershipMember
	err := c.makeRequest(ctx, http.MethodPatch, c.organizationMembershipPath("members", member.ID), payload, &updated)
	if err != nil {
		return member, err
	}
	return &updated, nil
}
