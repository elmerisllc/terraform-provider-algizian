// Package client is a small client for the Algizian management API (https://id.algizian.com/api/v1).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultEndpoint = "https://id.algizian.com/api/v1"

// Client calls the API with one bearer token (an `ag_` platform, org, service-account or tenant token).
type Client struct {
	BaseURL   string
	Token     string
	UserAgent string
	HTTP      *http.Client
	// MaxRetries is how often a rate-limited (429) or temporarily failing (502, 503, 504) request is repeated.
	MaxRetries int
	// Sleep waits between retries; tests replace it.
	Sleep func(ctx context.Context, d time.Duration) error
}

func New(endpoint, token, userAgent string) *Client {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Client{
		BaseURL:    strings.TrimRight(endpoint, "/"),
		Token:      token,
		UserAgent:  userAgent,
		HTTP:       &http.Client{Timeout: 60 * time.Second},
		MaxRetries: 3,
		Sleep: func(ctx context.Context, d time.Duration) error {
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-t.C:
				return nil
			}
		},
	}
}

// APIError is an RFC 9457 problem answer.
type APIError struct {
	Status int
	Title  string
	Detail string
	Field  string
	Method string
	Path   string
}

func (e *APIError) Error() string {
	msg := e.Detail
	if msg == "" {
		msg = e.Title
	}
	if e.Field != "" {
		msg += " (field: " + e.Field + ")"
	}
	return fmt.Sprintf("%s %s: %d %s", e.Method, e.Path, e.Status, msg)
}

func IsNotFound(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == http.StatusNotFound
}

func IsStatus(err error, status int) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == status
}

// Do sends `in` (JSON, may be nil) and decodes the JSON answer into `out` (may be nil).
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var payload []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		payload = b
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/json")
		if c.UserAgent != "" {
			req.Header.Set("User-Agent", c.UserAgent)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		res, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(res.Body, 4<<20))
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		if res.StatusCode >= 200 && res.StatusCode < 300 {
			if out == nil || len(body) == 0 {
				return nil
			}
			if err := json.Unmarshal(body, out); err != nil {
				return fmt.Errorf("%s %s: unreadable answer: %w", method, path, err)
			}
			return nil
		}
		apiErr := &APIError{Status: res.StatusCode, Method: method, Path: path}
		var p struct {
			Title  string `json:"title"`
			Detail string `json:"detail"`
			Field  string `json:"field"`
		}
		if json.Unmarshal(body, &p) == nil {
			apiErr.Title, apiErr.Detail, apiErr.Field = p.Title, p.Detail, p.Field
		}
		if apiErr.Title == "" && apiErr.Detail == "" {
			apiErr.Title = http.StatusText(res.StatusCode)
		}
		// 429 is always safe to repeat (the request was not processed); 5xx only for requests that are safe to repeat.
		retry := res.StatusCode == http.StatusTooManyRequests ||
			((res.StatusCode == 502 || res.StatusCode == 503 || res.StatusCode == 504) && (method == http.MethodGet || method == http.MethodPut || method == http.MethodDelete))
		if !retry || attempt >= c.MaxRetries {
			return apiErr
		}
		wait := time.Duration(attempt+1) * 2 * time.Second
		if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s >= 0 && s <= 60 {
			wait = time.Duration(s) * time.Second
		}
		if err := c.Sleep(ctx, wait); err != nil {
			return err
		}
	}
}

// ---- data shapes

type Tenant struct {
	Slug         string            `json:"slug"`
	Name         string            `json:"name"`
	Issuer       string            `json:"issuer"`
	Org          string            `json:"org"`
	Branding     map[string]any    `json:"branding"`
	Providers    []string          `json:"providers"`
	Tags         map[string]string `json:"tags"`
	Locked       bool              `json:"locked"`
	CreatedAt    string            `json:"createdAt"`
	CustomDomain *CustomDomain     `json:"customDomain"`
}

type CustomDomain struct {
	Hostname  string `json:"hostname"`
	Status    string `json:"status"`
	SSLStatus string `json:"sslStatus"`
	Error     string `json:"error"`
	DNS       struct {
		Type   string `json:"type"`
		Name   string `json:"name"`
		Target string `json:"target"`
	} `json:"dns"`
	Issuer string `json:"issuer"`
}

type OIDCClient struct {
	ID           string            `json:"id"`
	Tenant       string            `json:"tenant"`
	Name         string            `json:"name"`
	RedirectURIs []string          `json:"redirectUris"`
	Type         string            `json:"type"`
	GrantTypes   []string          `json:"grantTypes"`
	Tags         map[string]string `json:"tags"`
	FirstParty   bool              `json:"firstParty"`
	Registration string            `json:"registration"`
	ClientSecret string            `json:"clientSecret"`
}

type ProviderState struct {
	Provider string `json:"provider"`
	Enabled  bool   `json:"enabled"`
	Source   string `json:"source"` // shared | own | platform
	ClientID string `json:"clientId"`
}

type EmailSettings struct {
	Configured          bool   `json:"configured"`
	Enabled             bool   `json:"enabled"`
	Ready               bool   `json:"ready"`
	Sender              string `json:"sender"`
	Host                string `json:"host"`
	Port                int64  `json:"port"`
	Security            string `json:"security"`
	Username            string `json:"username"`
	FromAddress         string `json:"fromAddress"`
	FromName            string `json:"fromName"`
	Template            string `json:"template"`
	DailyCap            *int64 `json:"dailyCap"`
	MonthlyCap          *int64 `json:"monthlyCap"`
	EffectiveDailyCap   int64  `json:"effectiveDailyCap"`
	EffectiveMonthlyCap int64  `json:"effectiveMonthlyCap"`
	Status              string `json:"status"`
}

type ServiceAccount struct {
	ID   string            `json:"id"`
	Name string            `json:"name"`
	Role string            `json:"role"`
	Tags map[string]string `json:"tags"`
}

type OrgToken struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Role           string            `json:"role"`
	Prefix         string            `json:"prefix"`
	ServiceAccount string            `json:"serviceAccount"`
	Tags           map[string]string `json:"tags"`
	CreatedAt      string            `json:"createdAt"`
	ExpiresAt      string            `json:"expiresAt"`
	Token          string            `json:"token"`
}

// ---- tenants

func tenantPath(slug string) string { return "/tenants/" + url.PathEscape(slug) }

// CreateTenant creates under an organization (the name is assigned unless the platform token sends a slug in body) or,
// with an empty org, as a platform tenant (body carries the slug).
func (c *Client) CreateTenant(ctx context.Context, org string, body map[string]any) (*Tenant, error) {
	var out Tenant
	path := "/tenants"
	if org != "" {
		path = "/orgs/" + url.PathEscape(org) + "/tenants"
	}
	return &out, c.Do(ctx, http.MethodPost, path, body, &out)
}
func (c *Client) GetTenant(ctx context.Context, slug string) (*Tenant, error) {
	var out Tenant
	return &out, c.Do(ctx, http.MethodGet, tenantPath(slug), nil, &out)
}
func (c *Client) PatchTenant(ctx context.Context, slug string, body map[string]any) (*Tenant, error) {
	var out Tenant
	return &out, c.Do(ctx, http.MethodPatch, tenantPath(slug), body, &out)
}
func (c *Client) DeleteTenant(ctx context.Context, slug string) error {
	return c.Do(ctx, http.MethodDelete, tenantPath(slug), nil, nil)
}

// ---- custom domain

func (c *Client) SetDomain(ctx context.Context, slug, hostname string) (*Tenant, error) {
	var out Tenant
	return &out, c.Do(ctx, http.MethodPut, tenantPath(slug)+"/domain", map[string]any{"hostname": hostname}, &out)
}
func (c *Client) RefreshDomain(ctx context.Context, slug string) (*Tenant, error) {
	var out Tenant
	return &out, c.Do(ctx, http.MethodPost, tenantPath(slug)+"/domain/refresh", nil, &out)
}
func (c *Client) DeleteDomain(ctx context.Context, slug string) error {
	return c.Do(ctx, http.MethodDelete, tenantPath(slug)+"/domain", nil, nil)
}

// ---- OIDC clients

func (c *Client) CreateClient(ctx context.Context, slug string, body map[string]any) (*OIDCClient, error) {
	var out OIDCClient
	return &out, c.Do(ctx, http.MethodPost, tenantPath(slug)+"/clients", body, &out)
}
func (c *Client) GetClient(ctx context.Context, slug, id string) (*OIDCClient, error) {
	var out OIDCClient
	return &out, c.Do(ctx, http.MethodGet, tenantPath(slug)+"/clients/"+url.PathEscape(id), nil, &out)
}
func (c *Client) PatchClient(ctx context.Context, slug, id string, body map[string]any) (*OIDCClient, error) {
	var out OIDCClient
	return &out, c.Do(ctx, http.MethodPatch, tenantPath(slug)+"/clients/"+url.PathEscape(id), body, &out)
}
func (c *Client) DeleteClient(ctx context.Context, slug, id string) error {
	return c.Do(ctx, http.MethodDelete, tenantPath(slug)+"/clients/"+url.PathEscape(id), nil, nil)
}

// ---- provider apps

func (c *Client) ListProviders(ctx context.Context, slug string) ([]ProviderState, error) {
	var out struct {
		Providers []ProviderState `json:"providers"`
	}
	err := c.Do(ctx, http.MethodGet, tenantPath(slug)+"/providers", nil, &out)
	return out.Providers, err
}
func (c *Client) SetCredentials(ctx context.Context, slug, provider string, body map[string]any) error {
	return c.Do(ctx, http.MethodPut, tenantPath(slug)+"/providers/"+url.PathEscape(provider)+"/credentials", body, nil)
}
func (c *Client) DeleteCredentials(ctx context.Context, slug, provider string) error {
	return c.Do(ctx, http.MethodDelete, tenantPath(slug)+"/providers/"+url.PathEscape(provider)+"/credentials", nil, nil)
}

// ---- email sign-in

func (c *Client) GetEmail(ctx context.Context, slug string) (*EmailSettings, error) {
	var out EmailSettings
	return &out, c.Do(ctx, http.MethodGet, tenantPath(slug)+"/email", nil, &out)
}
func (c *Client) SetEmail(ctx context.Context, slug string, body map[string]any) error {
	return c.Do(ctx, http.MethodPut, tenantPath(slug)+"/email", body, nil)
}
func (c *Client) DeleteEmail(ctx context.Context, slug string) error {
	return c.Do(ctx, http.MethodDelete, tenantPath(slug)+"/email", nil, nil)
}

// ---- organizations: service accounts and tokens

func orgPath(org string) string { return "/orgs/" + url.PathEscape(org) }

func (c *Client) CreateServiceAccount(ctx context.Context, org string, body map[string]any) (*ServiceAccount, error) {
	var out ServiceAccount
	return &out, c.Do(ctx, http.MethodPost, orgPath(org)+"/service-accounts", body, &out)
}
func (c *Client) ListServiceAccounts(ctx context.Context, org string) ([]ServiceAccount, error) {
	var out struct {
		ServiceAccounts []ServiceAccount `json:"serviceAccounts"`
	}
	err := c.Do(ctx, http.MethodGet, orgPath(org)+"/service-accounts", nil, &out)
	return out.ServiceAccounts, err
}
func (c *Client) PatchServiceAccount(ctx context.Context, org, id string, body map[string]any) (*ServiceAccount, error) {
	var out ServiceAccount
	return &out, c.Do(ctx, http.MethodPatch, orgPath(org)+"/service-accounts/"+url.PathEscape(id), body, &out)
}
func (c *Client) DeleteServiceAccount(ctx context.Context, org, id string) error {
	return c.Do(ctx, http.MethodDelete, orgPath(org)+"/service-accounts/"+url.PathEscape(id), nil, nil)
}
func (c *Client) CreateServiceAccountToken(ctx context.Context, org, id string, body map[string]any) (*OrgToken, error) {
	var out OrgToken
	return &out, c.Do(ctx, http.MethodPost, orgPath(org)+"/service-accounts/"+url.PathEscape(id)+"/tokens", body, &out)
}
func (c *Client) ListOrgTokens(ctx context.Context, org string) ([]OrgToken, error) {
	var out struct {
		Tokens []OrgToken `json:"tokens"`
	}
	err := c.Do(ctx, http.MethodGet, orgPath(org)+"/tokens", nil, &out)
	return out.Tokens, err
}
func (c *Client) DeleteOrgToken(ctx context.Context, org, id string) error {
	return c.Do(ctx, http.MethodDelete, orgPath(org)+"/tokens/"+url.PathEscape(id), nil, nil)
}

// ListTenants lists the tenants of an organization, optionally only those that carry all the given tags.
func (c *Client) ListTenants(ctx context.Context, org string, tags map[string]string) ([]Tenant, error) {
	q := url.Values{}
	for k, v := range tags {
		q.Add("tag", k+"="+v)
	}
	path := "/orgs/" + url.PathEscape(org) + "/tenants"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out struct {
		Tenants []Tenant `json:"tenants"`
	}
	err := c.Do(ctx, http.MethodGet, path, nil, &out)
	return out.Tenants, err
}

// ---- providers catalogue and identity

type CatalogProvider struct {
	Provider    string `json:"provider"`
	DisplayName string `json:"displayName"`
}

func (c *Client) Catalogue(ctx context.Context) ([]CatalogProvider, error) {
	var out struct {
		Providers []CatalogProvider `json:"providers"`
	}
	err := c.Do(ctx, http.MethodGet, "/providers", nil, &out)
	return out.Providers, err
}

type Me struct {
	Scope          string `json:"scope"`
	Tenant         string `json:"tenant"`
	Org            string `json:"org"`
	ServiceAccount string `json:"serviceAccount"`
}

func (c *Client) Me(ctx context.Context) (*Me, error) {
	var out Me
	return &out, c.Do(ctx, http.MethodGet, "/me", nil, &out)
}
