package provider

import (
	"context"
	"strings"

	"github.com/elmerisllc/terraform-provider-algizian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*clientResource)(nil)
	_ resource.ResourceWithConfigure   = (*clientResource)(nil)
	_ resource.ResourceWithImportState = (*clientResource)(nil)
)

func NewClientResource() resource.Resource { return &clientResource{} }

type clientResource struct{ d *Data }

type clientModel struct {
	ID           types.String `tfsdk:"id"`
	Tenant       types.String `tfsdk:"tenant"`
	Name         types.String `tfsdk:"name"`
	RedirectURIs types.List   `tfsdk:"redirect_uris"`
	Type         types.String `tfsdk:"type"`
	FirstParty   types.Bool   `tfsdk:"first_party"`
	GrantTypes   types.Set    `tfsdk:"grant_types"`
	Tags         types.Map    `tfsdk:"tags"`
	ClientSecret types.String `tfsdk:"client_secret"`
	Registration types.String `tfsdk:"registration"`
}

func (r *clientResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client"
}

func (r *clientResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An OAuth/OIDC client of a tenant. The authorization code flow with PKCE `S256` is required for every client. " +
			"The `client_secret` of a confidential client is returned once at creation and kept in the Terraform state (sensitive): protect the state.",
		Attributes: map[string]schema.Attribute{
			"id":     schema.StringAttribute{Computed: true, Description: "The client id (`c_...`), also the `client_id` of the OIDC flow.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"tenant": schema.StringAttribute{Required: true, Description: "Slug of the tenant.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":   schema.StringAttribute{Required: true, Description: "1 to 80 characters."},
			"redirect_uris": schema.ListAttribute{
				Required: true, ElementType: types.StringType,
				Description: "Allowed redirect URIs (1 to 20): https, or `http://localhost` / loopback for development, or a custom scheme for native apps.",
			},
			"type": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("public"),
				Description:   "`public` (browser and mobile apps, no secret) or `confidential` (server-side, has a secret). Cannot be changed.",
				Validators:    []validator.String{stringvalidator.OneOf("public", "confidential")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"first_party": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), Description: "A first-party client skips the consent screen."},
			"grant_types": schema.SetAttribute{
				Optional: true, Computed: true, ElementType: types.StringType,
				Default:     setdefault.StaticValue(setOf([]string{"authorization_code", "refresh_token"})),
				Description: "`authorization_code` and/or `refresh_token`.",
			},
			"tags": schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: tagsSchemaDescription, PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()}},
			"client_secret": schema.StringAttribute{
				Computed: true, Sensitive: true,
				Description:   "Shown once by the API for `confidential` clients; null for public clients, and null after an import.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"registration": schema.StringAttribute{Computed: true, Description: "How the client was registered: `api` for this resource.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *clientResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.d = configure(req.ProviderData, &resp.Diagnostics)
}

func (m *clientModel) fill(c *client.OIDCClient) {
	m.ID = types.StringValue(c.ID)
	m.Tenant = types.StringValue(c.Tenant)
	m.Name = types.StringValue(c.Name)
	m.RedirectURIs = listOf(c.RedirectURIs)
	m.Type = types.StringValue(c.Type)
	m.FirstParty = types.BoolValue(c.FirstParty)
	m.GrantTypes = setOf(c.GrantTypes)
	m.Tags = tagsToMap(c.Tags)
	m.Registration = types.StringValue(c.Registration)
}

func (r *clientResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clientModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	uris, d := stringList(ctx, plan.RedirectURIs)
	resp.Diagnostics.Append(d...)
	grants, d := stringSet(ctx, plan.GrantTypes)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{
		"name": plan.Name.ValueString(), "redirectUris": uris, "type": plan.Type.ValueString(),
		"firstParty": plan.FirstParty.ValueBool(), "grantTypes": grants,
	}
	tg, d := tagsFromMap(ctx, plan.Tags)
	resp.Diagnostics.Append(d...)
	if len(tg) > 0 {
		body["tags"] = tg
	}
	c, err := r.d.Client.CreateClient(ctx, plan.Tenant.ValueString(), body)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not create the client", err)
		return
	}
	plan.fill(c)
	plan.ClientSecret = optString(c.ClientSecret)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *clientResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st clientModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	c, err := r.d.Client.GetClient(ctx, st.Tenant.ValueString(), st.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not read the client", err)
		return
	}
	st.fill(c) // the secret is never returned again: the state keeps it
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *clientResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, st clientModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	uris, d := stringList(ctx, plan.RedirectURIs)
	resp.Diagnostics.Append(d...)
	grants, d := stringSet(ctx, plan.GrantTypes)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{
		"name": plan.Name.ValueString(), "redirectUris": uris, "firstParty": plan.FirstParty.ValueBool(), "grantTypes": grants,
	}
	if !plan.Tags.IsUnknown() {
		tg, d := tagsFromMap(ctx, plan.Tags)
		resp.Diagnostics.Append(d...)
		if tg == nil {
			tg = map[string]string{}
		}
		body["tags"] = tg
	}
	c, err := r.d.Client.PatchClient(ctx, st.Tenant.ValueString(), st.ID.ValueString(), body)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not update the client", err)
		return
	}
	plan.fill(c)
	plan.ClientSecret = st.ClientSecret
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *clientResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st clientModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.d.Client.DeleteClient(ctx, st.Tenant.ValueString(), st.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "Could not delete the client", err)
	}
}

// ImportState takes `<tenant>/<client id>`. The secret of a confidential client cannot be imported.
func (r *clientResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Bad import id", "Use `<tenant slug>/<client id>`, for example `acme/c_abc123`.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tenant"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
