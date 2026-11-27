// Package provider is the Terraform provider for Algizian (terraform-plugin-framework).
package provider

import (
	"context"
	"os"

	"github.com/elmerisllc/terraform-provider-algizian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ provider.Provider = (*algizianProvider)(nil)

type algizianProvider struct{ version string }

// New returns the provider factory used by main and by the tests.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &algizianProvider{version: version} }
}

type providerModel struct {
	Endpoint types.String `tfsdk:"endpoint"`
	Token    types.String `tfsdk:"token"`
	Org      types.String `tfsdk:"org"`
}

// Data is what resources receive from the provider block.
type Data struct {
	Client     *client.Client
	DefaultOrg string
}

func (p *algizianProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "algizian"
	resp.Version = p.version
}

func (p *algizianProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage Algizian (OAuth 2.1 / OpenID Connect with social logins): tenants, custom domains, OIDC clients, own provider apps, email sign-in and service accounts.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Description: "Management API base URL. Default `https://id.algizian.com/api/v1`; environment variable `ALGIZIAN_ENDPOINT`.",
			},
			"token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "API token (`ag_...`): a platform token, an org admin token, a service-account credential or a tenant `manage` token. Environment variable `ALGIZIAN_TOKEN`.",
			},
			"org": schema.StringAttribute{
				Optional:    true,
				Description: "Default organization slug for tenants and service accounts. A token that belongs to an organization is enough; set this with a platform token. Environment variable `ALGIZIAN_ORG`.",
			},
		},
	}
}

func (p *algizianProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	pick := func(v types.String, env string) string {
		if !v.IsNull() && !v.IsUnknown() {
			return v.ValueString()
		}
		return os.Getenv(env)
	}
	for _, a := range []struct {
		name string
		v    types.String
	}{{"token", cfg.Token}, {"endpoint", cfg.Endpoint}, {"org", cfg.Org}} {
		if a.v.IsUnknown() {
			resp.Diagnostics.AddAttributeError(path.Root(a.name), "Unknown value", "The "+a.name+" must be known when the provider is configured (it cannot depend on a resource that is created in the same run).")
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	token := pick(cfg.Token, "ALGIZIAN_TOKEN")
	if token == "" {
		resp.Diagnostics.AddAttributeError(path.Root("token"), "Missing API token", "Set `token` in the provider block or the ALGIZIAN_TOKEN environment variable.")
		return
	}
	d := &Data{
		Client:     client.New(pick(cfg.Endpoint, "ALGIZIAN_ENDPOINT"), token, "terraform-provider-algizian/"+p.version),
		DefaultOrg: pick(cfg.Org, "ALGIZIAN_ORG"),
	}
	// A token that belongs to an organization (org admin token or service account) names its org: no need to configure it.
	if me, err := d.Client.Me(ctx); err != nil {
		if client.IsStatus(err, 401) || client.IsStatus(err, 403) {
			resp.Diagnostics.AddAttributeError(path.Root("token"), "The API token was refused", err.Error())
			return
		}
		resp.Diagnostics.AddWarning("Could not check the API token", err.Error())
	} else if d.DefaultOrg == "" {
		d.DefaultOrg = me.Org
	}
	resp.ResourceData = d
	resp.DataSourceData = d
}

func (p *algizianProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewTenantResource,
		NewCustomDomainResource,
		NewClientResource,
		NewProviderCredentialsResource,
		NewEmailSettingsResource,
		NewServiceAccountResource,
		NewServiceAccountTokenResource,
	}
}

func (p *algizianProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{NewTenantDataSource, NewTenantsDataSource, NewProvidersDataSource}
}
