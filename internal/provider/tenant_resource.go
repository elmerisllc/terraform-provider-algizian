package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/elmerisllc/terraform-provider-algizian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*tenantResource)(nil)
	_ resource.ResourceWithConfigure   = (*tenantResource)(nil)
	_ resource.ResourceWithImportState = (*tenantResource)(nil)
)

func NewTenantResource() resource.Resource { return &tenantResource{} }

type tenantResource struct{ d *Data }

type tenantModel struct {
	ID        types.String `tfsdk:"id"`
	Slug      types.String `tfsdk:"slug"`
	Name      types.String `tfsdk:"name"`
	Org       types.String `tfsdk:"org"`
	Providers types.Set    `tfsdk:"providers"`
	Branding  types.Object `tfsdk:"branding"`
	Tags      types.Map    `tfsdk:"tags"`
	Locked    types.Bool   `tfsdk:"locked"`
	Issuer    types.String `tfsdk:"issuer"`
	CreatedAt types.String `tfsdk:"created_at"`
}

// brandingKeys maps the Terraform attribute to the API field.
var brandingKeys = []struct{ tf, api string }{
	{"display_name", "displayName"},
	{"primary_color", "primaryColor"},
	{"background_color", "backgroundColor"},
	{"text_color", "textColor"},
	{"sign_in_title", "signInTitle"},
	{"sign_in_subtitle", "signInSubtitle"},
	{"consent_note", "consentNote"},
	{"footer_text", "footerText"},
	{"shared_app_notice", "sharedAppNotice"},
}

func brandingAttrTypes() map[string]attr.Type {
	m := map[string]attr.Type{}
	for _, k := range brandingKeys {
		m[k.tf] = types.StringType
	}
	return m
}

func (r *tenantResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant"
}

func (r *tenantResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	brand := map[string]schema.Attribute{}
	desc := map[string]string{
		"display_name":      "Name shown on the sign-in page and in consent screens (1 to 80 characters).",
		"primary_color":     "Button colour, `#rrggbb`.",
		"background_color":  "Page background, `#rrggbb`.",
		"text_color":        "Text colour, `#rrggbb`.",
		"sign_in_title":     "Heading of the sign-in page (up to 80 characters).",
		"sign_in_subtitle":  "Text under the heading (up to 200 characters).",
		"consent_note":      "Note on the consent screen (up to 300 characters).",
		"footer_text":       "Footer line (up to 200 characters).",
		"shared_app_notice": "Notice shown while the shared provider app is used (up to 300 characters).",
	}
	for _, k := range brandingKeys {
		brand[k.tf] = schema.StringAttribute{Optional: true, Description: desc[k.tf]}
	}
	resp.Schema = schema.Schema{
		Description: "A tenant: one sign-in domain with its own users, OIDC clients, providers and branding. " +
			"With an organization (`org` or the provider default) the name `<slug>.algizian.com` is assigned and returned in `slug`; " +
			"only a platform token may choose a slug. Deleting a tenant needs a platform token or an organization admin token; set `locked = true` to protect it. " +
			"The logo is uploaded through the logo endpoint and is not managed by this resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "The slug.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"slug": schema.StringAttribute{
				Optional: true, Computed: true,
				Description:   "The tenant name in `<slug>.algizian.com` (4 to 40 characters). Assigned when the tenant is created under an organization; set it only with a platform token. Cannot be changed.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Optional: true, Computed: true,
				Description:   "Display name of the tenant (1 to 80 characters). Defaults to the branding display name, else the slug.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"org": schema.StringAttribute{
				Optional: true, Computed: true,
				Description:   "Organization slug. Defaults to the provider `org`. A tenant cannot move between organizations.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplaceIfConfigured(), stringplanmodifier.UseStateForUnknown()},
			},
			"providers": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Description:   "Sign-in providers offered: `google`, `microsoft`, `apple`, `email`. Default: all of them. See the `algizian_providers` data source.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			},
			"branding": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Sign-in page branding. An attribute removed from the configuration is cleared.",
				Attributes:  brand,
			},
			"locked":     schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), Description: "Deletion lock. While `true` the API refuses to delete the tenant (and so does `terraform destroy`); set it to `false` and apply before destroying. Leave it off for temporary test tenants. Default `false`."},
			"tags":       schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: tagsSchemaDescription, PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()}},
			"issuer":     schema.StringAttribute{Computed: true, Description: "The default issuer `https://<slug>.algizian.com`. With an active custom domain the issuer is the custom host, see `algizian_custom_domain`.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"created_at": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *tenantResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.d = configure(req.ProviderData, &resp.Diagnostics)
}

func brandingBody(ctx context.Context, plan, prior types.Object, patch bool) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := map[string]any{}
	get := func(o types.Object, tf string) (string, bool) {
		if o.IsNull() || o.IsUnknown() {
			return "", false
		}
		v, ok := o.Attributes()[tf].(types.String)
		if !ok || v.IsNull() || v.IsUnknown() {
			return "", false
		}
		return v.ValueString(), true
	}
	for _, k := range brandingKeys {
		if s, ok := get(plan, k.tf); ok {
			out[k.api] = s
		} else if patch {
			if _, was := get(prior, k.tf); was {
				out[k.api] = nil // null clears the field
			}
		}
	}
	_ = ctx
	return out, diags
}

func brandingFromAPI(m map[string]any) types.Object {
	vals := map[string]attr.Value{}
	any_ := false
	for _, k := range brandingKeys {
		if s, ok := m[k.api].(string); ok && s != "" {
			vals[k.tf] = types.StringValue(s)
			any_ = true
		} else {
			vals[k.tf] = types.StringNull()
		}
	}
	if !any_ {
		return types.ObjectNull(brandingAttrTypes())
	}
	o, _ := types.ObjectValue(brandingAttrTypes(), vals)
	return o
}

// fillFromAPI writes the server's answer into the model. The branding block keeps the planned value when given
// (so a server default never causes a plan difference); Read refreshes it from the API.
func fillFromAPI(m *tenantModel, t *client.Tenant, keepBranding bool) {
	m.ID = types.StringValue(t.Slug)
	m.Slug = types.StringValue(t.Slug)
	m.Name = types.StringValue(t.Name)
	m.Org = optString(t.Org)
	m.Providers = setOf(t.Providers)
	m.Tags = tagsToMap(t.Tags)
	m.Locked = types.BoolValue(t.Locked)
	m.Issuer = types.StringValue(t.Issuer)
	m.CreatedAt = types.StringValue(t.CreatedAt)
	if !keepBranding {
		m.Branding = brandingFromAPI(t.Branding)
	}
}

func (r *tenantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan tenantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org := orgOrDefault(plan.Org, r.d)
	slugSet := !plan.Slug.IsNull() && !plan.Slug.IsUnknown() && plan.Slug.ValueString() != ""
	if org == "" && !slugSet {
		resp.Diagnostics.AddError("No organization and no slug", "Set `org` (or the provider `org`) to have a name assigned, or set `slug` when using a platform token.")
		return
	}
	body := map[string]any{}
	if slugSet {
		body["slug"] = plan.Slug.ValueString()
	}
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		body["name"] = plan.Name.ValueString()
	}
	if !plan.Providers.IsNull() && !plan.Providers.IsUnknown() {
		ps, d := stringSet(ctx, plan.Providers)
		resp.Diagnostics.Append(d...)
		body["providers"] = ps
	}
	if plan.Locked.ValueBool() {
		body["locked"] = true
	}
	if tg, d := tagsFromMap(ctx, plan.Tags); len(tg) > 0 {
		body["tags"] = tg
	} else {
		resp.Diagnostics.Append(d...)
	}
	b, d := brandingBody(ctx, plan.Branding, types.ObjectNull(brandingAttrTypes()), false)
	resp.Diagnostics.Append(d...)
	if len(b) > 0 {
		body["branding"] = b
	}
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.d.Client.CreateTenant(ctx, org, body)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not create the tenant", err)
		return
	}
	fillFromAPI(&plan, t, true)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *tenantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st tenantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	t, err := r.d.Client.GetTenant(ctx, st.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not read the tenant", err)
		return
	}
	fillFromAPI(&st, t, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *tenantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, st tenantModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{}
	if !plan.Name.IsUnknown() && !plan.Name.Equal(st.Name) {
		body["name"] = plan.Name.ValueString()
	}
	if !plan.Providers.IsUnknown() && !plan.Providers.Equal(st.Providers) {
		ps, d := stringSet(ctx, plan.Providers)
		resp.Diagnostics.Append(d...)
		body["providers"] = ps
	}
	if !plan.Locked.IsUnknown() && !plan.Locked.Equal(st.Locked) {
		body["locked"] = plan.Locked.ValueBool()
	}
	if !plan.Tags.IsUnknown() && !plan.Tags.Equal(st.Tags) {
		tg, d := tagsFromMap(ctx, plan.Tags)
		resp.Diagnostics.Append(d...)
		if tg == nil {
			tg = map[string]string{}
		}
		body["tags"] = tg
	}
	if !plan.Branding.Equal(st.Branding) {
		b, d := brandingBody(ctx, plan.Branding, st.Branding, true)
		resp.Diagnostics.Append(d...)
		if len(b) > 0 {
			body["branding"] = b
		}
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if len(body) > 0 {
		t, err := r.d.Client.PatchTenant(ctx, st.ID.ValueString(), body)
		if err != nil {
			apiErrorDiag(&resp.Diagnostics, "Could not update the tenant", err)
			return
		}
		fillFromAPI(&plan, t, true)
	} else {
		plan.ID, plan.Slug, plan.Issuer, plan.CreatedAt = st.ID, st.Slug, st.Issuer, st.CreatedAt
		if plan.Name.IsUnknown() {
			plan.Name = st.Name
		}
		if plan.Org.IsUnknown() {
			plan.Org = st.Org
		}
		if plan.Providers.IsUnknown() {
			plan.Providers = st.Providers
		}
		if plan.Tags.IsUnknown() {
			plan.Tags = st.Tags
		}
		if plan.Locked.IsUnknown() {
			plan.Locked = st.Locked
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *tenantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st tenantModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.d.Client.DeleteTenant(ctx, st.ID.ValueString())
	if err == nil || client.IsNotFound(err) {
		return
	}
	if client.IsStatus(err, 409) {
		resp.Diagnostics.AddError("The tenant is locked",
			fmt.Sprintf("%s\n\nSet `locked = false` on the tenant, apply that change, then destroy it.", err))
		return
	}
	if client.IsStatus(err, 403) {
		resp.Diagnostics.AddError("The token may not delete this tenant",
			fmt.Sprintf("%s\n\nDeleting a tenant needs a platform token or an organization admin token (or an admin service account) of the tenant's own organization; read and tenant tokens cannot.", err))
		return
	}
	apiErrorDiag(&resp.Diagnostics, "Could not delete the tenant", err)
}

func (r *tenantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strings.TrimSpace(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("slug"), strings.TrimSpace(req.ID))...)
}

var _ = validator.String(nil)
