package provider

import (
	"context"

	"github.com/elmerisllc/terraform-provider-algizian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ datasource.DataSource              = (*tenantDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*tenantDataSource)(nil)
	_ datasource.DataSource              = (*providersDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*providersDataSource)(nil)
)

func NewTenantDataSource() datasource.DataSource    { return &tenantDataSource{} }
func NewProvidersDataSource() datasource.DataSource { return &providersDataSource{} }
func NewTenantsDataSource() datasource.DataSource   { return &tenantsDataSource{} }

// ---- algizian_tenant

type tenantDataSource struct{ d *Data }

type tenantDataModel struct {
	ID                   types.String `tfsdk:"id"`
	Slug                 types.String `tfsdk:"slug"`
	Name                 types.String `tfsdk:"name"`
	Org                  types.String `tfsdk:"org"`
	Providers            types.Set    `tfsdk:"providers"`
	Issuer               types.String `tfsdk:"issuer"`
	CustomDomainHostname types.String `tfsdk:"custom_domain_hostname"`
	CustomDomainStatus   types.String `tfsdk:"custom_domain_status"`
	CustomDomainIssuer   types.String `tfsdk:"custom_domain_issuer"`
	Tags                 types.Map    `tfsdk:"tags"`
	Locked               types.Bool   `tfsdk:"locked"`
	CreatedAt            types.String `tfsdk:"created_at"`
}

func (r *tenantDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant"
}

func (r *tenantDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Read a tenant, for example to use its issuer in an application's configuration.",
		Attributes: map[string]schema.Attribute{
			"id":                     schema.StringAttribute{Computed: true},
			"slug":                   schema.StringAttribute{Required: true},
			"name":                   schema.StringAttribute{Computed: true},
			"org":                    schema.StringAttribute{Computed: true},
			"providers":              schema.SetAttribute{Computed: true, ElementType: types.StringType},
			"issuer":                 schema.StringAttribute{Computed: true, Description: "The `<slug>.algizian.com` issuer."},
			"custom_domain_hostname": schema.StringAttribute{Computed: true},
			"custom_domain_status":   schema.StringAttribute{Computed: true},
			"custom_domain_issuer":   schema.StringAttribute{Computed: true, Description: "The issuer to use once the custom domain is active."},
			"tags":                   schema.MapAttribute{Computed: true, ElementType: types.StringType},
			"locked":                 schema.BoolAttribute{Computed: true, Description: "Whether the deletion lock is on."},
			"created_at":             schema.StringAttribute{Computed: true},
		},
	}
}

func (r *tenantDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	r.d = configure(req.ProviderData, &resp.Diagnostics)
}

func (r *tenantDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m tenantDataModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.d.Client.GetTenant(ctx, m.Slug.ValueString())
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not read the tenant", err)
		return
	}
	m.ID, m.Name, m.Org = types.StringValue(t.Slug), types.StringValue(t.Name), optString(t.Org)
	m.Tags = tagsToMap(t.Tags)
	m.Locked = types.BoolValue(t.Locked)
	m.Providers, m.Issuer, m.CreatedAt = setOf(t.Providers), types.StringValue(t.Issuer), types.StringValue(t.CreatedAt)
	m.CustomDomainHostname, m.CustomDomainStatus, m.CustomDomainIssuer = types.StringNull(), types.StringNull(), types.StringNull()
	if d := t.CustomDomain; d != nil {
		m.CustomDomainHostname, m.CustomDomainStatus, m.CustomDomainIssuer = types.StringValue(d.Hostname), types.StringValue(d.Status), types.StringValue(d.Issuer)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

// ---- algizian_providers

type providersDataSource struct{ d *Data }

type providersModel struct {
	ID        types.String    `tfsdk:"id"`
	Providers []providerEntry `tfsdk:"providers"`
}
type providerEntry struct {
	Provider    types.String `tfsdk:"provider_name"`
	DisplayName types.String `tfsdk:"display_name"`
}

func (r *providersDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_providers"
}

func (r *providersDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The sign-in providers Algizian supports (generated by the server from its provider adapters).",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true},
			"providers": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"provider_name": schema.StringAttribute{Computed: true},
					"display_name":  schema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (r *providersDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	r.d = configure(req.ProviderData, &resp.Diagnostics)
}

func (r *providersDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	list, err := r.d.Client.Catalogue(ctx)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not read the providers", err)
		return
	}
	m := providersModel{ID: types.StringValue("providers")}
	for _, p := range list {
		m.Providers = append(m.Providers, providerEntry{Provider: types.StringValue(p.Provider), DisplayName: types.StringValue(p.DisplayName)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}

var _ = client.IsNotFound

// ---- algizian_tenants

type tenantsDataSource struct{ d *Data }

type tenantsModel struct {
	ID      types.String   `tfsdk:"id"`
	Org     types.String   `tfsdk:"org"`
	Tags    types.Map      `tfsdk:"tags"`
	Tenants []tenantsEntry `tfsdk:"tenants"`
}
type tenantsEntry struct {
	Slug                 types.String `tfsdk:"slug"`
	Name                 types.String `tfsdk:"name"`
	Issuer               types.String `tfsdk:"issuer"`
	Providers            types.Set    `tfsdk:"providers"`
	Tags                 types.Map    `tfsdk:"tags"`
	Locked               types.Bool   `tfsdk:"locked"`
	CustomDomainHostname types.String `tfsdk:"custom_domain_hostname"`
	CustomDomainStatus   types.String `tfsdk:"custom_domain_status"`
	CustomDomainIssuer   types.String `tfsdk:"custom_domain_issuer"`
	CreatedAt            types.String `tfsdk:"created_at"`
}

func (r *tenantsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenants"
}

func (r *tenantsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The tenants of an organization, optionally only those that carry all the given tags.",
		Attributes: map[string]schema.Attribute{
			"id":   schema.StringAttribute{Computed: true},
			"org":  schema.StringAttribute{Optional: true, Computed: true, Description: "Organization slug; defaults to the provider `org`, else the organization of the token."},
			"tags": schema.MapAttribute{Optional: true, ElementType: types.StringType, Description: "Keep only tenants that have all of these tags (exact values)."},
			"tenants": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"slug":                   schema.StringAttribute{Computed: true},
					"name":                   schema.StringAttribute{Computed: true},
					"issuer":                 schema.StringAttribute{Computed: true},
					"providers":              schema.SetAttribute{Computed: true, ElementType: types.StringType},
					"tags":                   schema.MapAttribute{Computed: true, ElementType: types.StringType},
					"locked":                 schema.BoolAttribute{Computed: true, Description: "Whether the deletion lock is on."},
					"custom_domain_hostname": schema.StringAttribute{Computed: true},
					"custom_domain_status":   schema.StringAttribute{Computed: true},
					"custom_domain_issuer":   schema.StringAttribute{Computed: true},
					"created_at":             schema.StringAttribute{Computed: true},
				}},
			},
		},
	}
}

func (r *tenantsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	r.d = configure(req.ProviderData, &resp.Diagnostics)
}

func (r *tenantsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m tenantsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org := orgOrDefault(m.Org, r.d)
	if org == "" {
		resp.Diagnostics.AddError("No organization", "Set `org`, the provider `org`, or use a token that belongs to an organization.")
		return
	}
	filter, d := tagsFromMap(ctx, m.Tags)
	resp.Diagnostics.Append(d...)
	list, err := r.d.Client.ListTenants(ctx, org, filter)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not list the tenants", err)
		return
	}
	m.ID, m.Org = types.StringValue(org), types.StringValue(org)
	m.Tenants = nil
	for _, t := range list {
		e := tenantsEntry{Slug: types.StringValue(t.Slug), Name: types.StringValue(t.Name), Issuer: types.StringValue(t.Issuer), Providers: setOf(t.Providers), Tags: tagsToMap(t.Tags), Locked: types.BoolValue(t.Locked),
			CustomDomainHostname: types.StringNull(), CustomDomainStatus: types.StringNull(), CustomDomainIssuer: types.StringNull(), CreatedAt: types.StringValue(t.CreatedAt)}
		if cd := t.CustomDomain; cd != nil {
			e.CustomDomainHostname, e.CustomDomainStatus, e.CustomDomainIssuer = types.StringValue(cd.Hostname), types.StringValue(cd.Status), types.StringValue(cd.Issuer)
		}
		m.Tenants = append(m.Tenants, e)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}
