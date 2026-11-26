package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/elmerisllc/terraform-provider-algizian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*customDomainResource)(nil)
	_ resource.ResourceWithConfigure   = (*customDomainResource)(nil)
	_ resource.ResourceWithImportState = (*customDomainResource)(nil)
)

func NewCustomDomainResource() resource.Resource { return &customDomainResource{} }

type customDomainResource struct{ d *Data }

type customDomainModel struct {
	ID            types.String `tfsdk:"id"`
	Tenant        types.String `tfsdk:"tenant"`
	Hostname      types.String `tfsdk:"hostname"`
	WaitForActive types.Bool   `tfsdk:"wait_for_active"`
	Status        types.String `tfsdk:"status"`
	SSLStatus     types.String `tfsdk:"ssl_status"`
	Error         types.String `tfsdk:"error"`
	CNAMEName     types.String `tfsdk:"cname_name"`
	CNAMETarget   types.String `tfsdk:"cname_target"`
	Issuer        types.String `tfsdk:"issuer"`
}

func (r *customDomainResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_custom_domain"
}

func (r *customDomainResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The custom domain of a tenant. Once it is `active` it is the issuer of the tenant (`iss` of every token), so decide it before any client or token depends on the tenant. " +
			"Create the DNS record from `cname_name` and `cname_target` (for example with the Cloudflare provider) in the same configuration. " +
			"Custom domains are an add-on of the organization's plan.",
		Attributes: map[string]schema.Attribute{
			"id":       schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"tenant":   schema.StringAttribute{Required: true, Description: "Slug of the tenant.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"hostname": schema.StringAttribute{Required: true, Description: "A subdomain you control, for example `login.example.com` (not an apex name)."},
			"wait_for_active": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				Description: "Wait until the domain is `active` (up to 15 minutes). Leave this off when the DNS record is created in the same run: it can only become active after the record exists, so a wait here would block the record. Run `terraform apply` again, or set this after the record, to wait.",
			},
			"status":       schema.StringAttribute{Computed: true, Description: "`pending`, `active` or `error`."},
			"ssl_status":   schema.StringAttribute{Computed: true},
			"error":        schema.StringAttribute{Computed: true},
			"cname_name":   schema.StringAttribute{Computed: true, Description: "Name of the CNAME record to create (the hostname)."},
			"cname_target": schema.StringAttribute{Computed: true, Description: "Target of the CNAME record (`cname.algizian.com`).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"issuer":       schema.StringAttribute{Computed: true, Description: "`https://<hostname>`, the issuer once the domain is active."},
		},
	}
}

func (r *customDomainResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.d = configure(req.ProviderData, &resp.Diagnostics)
}

func (m *customDomainModel) fill(t *client.Tenant) bool {
	d := t.CustomDomain
	if d == nil {
		return false
	}
	m.ID = types.StringValue(t.Slug)
	m.Tenant = types.StringValue(t.Slug)
	m.Hostname = types.StringValue(d.Hostname)
	m.Status = types.StringValue(d.Status)
	m.SSLStatus = types.StringValue(d.SSLStatus)
	m.Error = types.StringValue(d.Error)
	m.CNAMEName = types.StringValue(d.DNS.Name)
	m.CNAMETarget = types.StringValue(d.DNS.Target)
	m.Issuer = types.StringValue(d.Issuer)
	return true
}

const domainWait = 15 * time.Minute

// domainPoll is how often a pending domain is checked while waiting (a variable so tests can shorten it).
var domainPoll = 15 * time.Second

func (r *customDomainResource) waitActive(ctx context.Context, slug string, t *client.Tenant) (*client.Tenant, error) {
	deadline := time.Now().Add(domainWait)
	for t.CustomDomain != nil && t.CustomDomain.Status == "pending" {
		if time.Now().After(deadline) {
			return t, fmt.Errorf("the domain is still pending after %s: check that the CNAME record exists and has propagated", domainWait)
		}
		if err := r.d.Client.Sleep(ctx, domainPoll); err != nil {
			return t, err
		}
		next, err := r.d.Client.RefreshDomain(ctx, slug)
		if err != nil {
			return t, err
		}
		t = next
	}
	if t.CustomDomain != nil && t.CustomDomain.Status == "error" {
		return t, fmt.Errorf("the domain failed: %s", t.CustomDomain.Error)
	}
	return t, nil
}

func (r *customDomainResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan customDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	slug := plan.Tenant.ValueString()
	t, err := r.d.Client.SetDomain(ctx, slug, plan.Hostname.ValueString())
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not set the custom domain", err)
		return
	}
	plan.fill(t)
	// save the state before waiting, so a timeout leaves the domain tracked
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	if plan.WaitForActive.ValueBool() {
		t, err = r.waitActive(ctx, slug, t)
		if t != nil {
			plan.fill(t)
			resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
		}
		if err != nil {
			resp.Diagnostics.AddError("The custom domain is not active", err.Error())
		}
	}
}

func (r *customDomainResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st customDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	slug := st.Tenant.ValueString()
	t, err := r.d.Client.GetTenant(ctx, slug)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not read the tenant", err)
		return
	}
	if t.CustomDomain != nil && t.CustomDomain.Status == "pending" {
		if rt, err := r.d.Client.RefreshDomain(ctx, slug); err == nil {
			t = rt
		}
	}
	if !st.fill(t) {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *customDomainResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, st customDomainModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	slug := st.Tenant.ValueString()
	var t *client.Tenant
	var err error
	if plan.Hostname.Equal(st.Hostname) {
		t, err = r.d.Client.RefreshDomain(ctx, slug)
	} else {
		t, err = r.d.Client.SetDomain(ctx, slug, plan.Hostname.ValueString())
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not update the custom domain", err)
		return
	}
	if plan.WaitForActive.ValueBool() {
		if wt, werr := r.waitActive(ctx, slug, t); werr != nil {
			resp.Diagnostics.AddError("The custom domain is not active", werr.Error())
		} else {
			t = wt
		}
	}
	plan.fill(t)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *customDomainResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st customDomainModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.d.Client.DeleteDomain(ctx, st.Tenant.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "Could not remove the custom domain", err)
	}
}

func (r *customDomainResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	slug := strings.TrimSpace(req.ID)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), slug)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tenant"), slug)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("wait_for_active"), false)...)
}
