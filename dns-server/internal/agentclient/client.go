package agentclient

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

type NetworkServiceClient interface {
	HasRule(ctx context.Context, table uint32, iif string) (bool, error)
	AddRule(ctx context.Context, rule Rule) error
	ListRoutes(ctx context.Context, table uint32) ([]Route, error)
	AddRoute(ctx context.Context, route Route) error
	DeleteRoute(ctx context.Context, route Route) error
	ListHosts(ctx context.Context) ([]HostInfo, error)
}

func NewNetworkServiceClient(baseURL string, timeout time.Duration) (NetworkServiceClient, error) {
	c, err := NewClientWithResponses(baseURL, WithHTTPClient(&http.Client{Timeout: timeout}))
	if err != nil {
		return nil, err
	}
	return &networkServiceClient{c}, nil
}

type networkServiceClient struct {
	c *ClientWithResponses
}

func (c *networkServiceClient) HasRule(ctx context.Context, table uint32, iif string) (bool, error) {
	resp, err := c.c.HasRuleWithResponse(ctx, &HasRuleParams{Table: table, Iif: iif})
	if err != nil {
		return false, err
	}
	switch resp.StatusCode() {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, responseError("HasRule", resp.StatusCode(), resp.JSON500)
	}
}

func (c *networkServiceClient) AddRule(ctx context.Context, rule Rule) error {
	resp, err := c.c.AddRuleWithResponse(ctx, rule)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusNoContent {
		return responseError("AddRule", resp.StatusCode(), resp.JSON500)
	}
	return nil
}

func (c *networkServiceClient) ListRoutes(ctx context.Context, table uint32) ([]Route, error) {
	resp, err := c.c.ListRoutesWithResponse(ctx, &ListRoutesParams{Table: table})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, responseError("ListRoutes", resp.StatusCode(), resp.JSON500)
	}
	if resp.JSON200 == nil {
		return nil, nil
	}
	return *resp.JSON200, nil
}

func (c *networkServiceClient) AddRoute(ctx context.Context, route Route) error {
	resp, err := c.c.AddRouteWithResponse(ctx, route)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusNoContent {
		return responseError("AddRoute", resp.StatusCode(), resp.JSON500)
	}
	return nil
}

func (c *networkServiceClient) DeleteRoute(ctx context.Context, route Route) error {
	resp, err := c.c.DeleteRouteWithResponse(ctx, route)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusNoContent {
		return responseError("DeleteRoute", resp.StatusCode(), resp.JSON500)
	}
	return nil
}

func (c *networkServiceClient) ListHosts(ctx context.Context) ([]HostInfo, error) {
	resp, err := c.c.ListHostsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, responseError("ListHosts", resp.StatusCode(), resp.JSON500)
	}
	if resp.JSON200 == nil {
		return nil, nil
	}
	return *resp.JSON200, nil
}

func responseError(op string, statusCode int, apiErr *Error) error {
	if apiErr != nil {
		return fmt.Errorf("%s: %s (status %d)", op, apiErr.Error, statusCode)
	}
	return fmt.Errorf("%s: unexpected status %d", op, statusCode)
}
