package provider

import (
	"context"
	"strings"

	"github.com/elmerisllc/terraform-provider-algizian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                   = (*providerCredentialsResource)(nil)
	_ resource.ResourceWithConfigure      = (*providerCredentialsResource)(nil)
	_ resource.ResourceWithImportState    = (*providerCredentialsResource)(nil)
	_ resource.ResourceWithValidateConfig = (*providerCredentialsResource)(nil)
)

func NewProviderCredentialsResource() resource.Resource { return &providerCredentialsResource{} }

type providerCredentialsResource struct{ d *Data }

type providerCredentialsModel struct {
	ID                types.String `tfsdk:"id"`
	Tenant            types.String `tfsdk:"tenant"`
	Provider          types.String `tfsdk:"provider_name"`
	ClientID          types.String `tfsdk:"client_id"`
	ClientSecret      types.String `tfsdk:"client_secret"`
	TeamID            types.String `tfsdk:"team_id"`
	KeyID             types.String `tfsdk:"key_id"`
	PrivateKey        types.String `tfsdk:"private_key"`
	DomainAssociation types.String `tfsdk:"domain_association"`
}

func (r *providerCredentialsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_provider_credentials"
}

func (r *providerCredentialsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Use the tenant's own Google, Microsoft or Apple app, so the consent screen shows your name. " +
			"**The provider's `sub` changes with the app** (Microsoft per app registration, Apple per team, Google not at all): " +
			"after switching, existing users re-match only by verified email. Decide before users exist. " +
			"Register the redirect URIs that `GET /tenants/{t}/providers` lists (every sign-in host of the tenant). " +
			"Secrets are stored encrypted by Algizian and never returned, so they live in the Terraform state (sensitive).",
		Attributes: map[string]schema.Attribute{
			"id":     schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"tenant": schema.StringAttribute{Required: true, PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"provider_name": schema.StringAttribute{
				Required: true, Description: "`google`, `microsoft` or `apple`.",
				Validators:    []validator.String{stringvalidator.OneOf("google", "microsoft", "apple")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"client_id":          schema.StringAttribute{Required: true, Description: "Google: `123-abc.apps.googleusercontent.com`; Microsoft: the application (client) id; Apple: the Services ID."},
			"client_secret":      schema.StringAttribute{Optional: true, Sensitive: true, Description: "Google and Microsoft."},
			"team_id":            schema.StringAttribute{Optional: true, Description: "Apple."},
			"key_id":             schema.StringAttribute{Optional: true, Description: "Apple."},
			"private_key":        schema.StringAttribute{Optional: true, Sensitive: true, Description: "Apple: the `.p8` key (PEM text)."},
			"domain_association": schema.StringAttribute{Optional: true, Description: "Apple: optional domain association file content."},
		},
	}
}

func (r *providerCredentialsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.d = configure(req.ProviderData, &resp.Diagnostics)
}

func (r *providerCredentialsResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m providerCredentialsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Provider.IsUnknown() || m.Provider.IsNull() {
		return
	}
	need := map[string][]struct {
		name string
		v    types.String
	}{
		"google":    {{"client_secret", m.ClientSecret}},
		"microsoft": {{"client_secret", m.ClientSecret}},
		"apple":     {{"team_id", m.TeamID}, {"key_id", m.KeyID}, {"private_key", m.PrivateKey}},
	}
	for _, f := range need[m.Provider.ValueString()] {
		if f.v.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root(f.name), "Missing attribute", "`"+f.name+"` is required for the "+m.Provider.ValueString()+" provider.")
		}
	}
	if m.Provider.ValueString() != "apple" {
		for _, f := range []struct {
			name string
			v    types.String
		}{{"team_id", m.TeamID}, {"key_id", m.KeyID}, {"private_key", m.PrivateKey}, {"domain_association", m.DomainAssociation}} {
			if !f.v.IsNull() {
				resp.Diagnostics.AddAttributeError(path.Root(f.name), "Unexpected attribute", "`"+f.name+"` is only used for the apple provider.")
			}
		}
	} else if !m.ClientSecret.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("client_secret"), "Unexpected attribute", "Apple uses `team_id`, `key_id` and `private_key`; the client secret is derived from them.")
	}
}

func (m providerCredentialsModel) body() map[string]any {
	b := map[string]any{"clientId": m.ClientID.ValueString()}
	set := func(k string, v types.String) {
		if !v.IsNull() && !v.IsUnknown() {
			b[k] = v.ValueString()
		}
	}
	set("clientSecret", m.ClientSecret)
	set("teamId", m.TeamID)
	set("keyId", m.KeyID)
	set("privateKey", m.PrivateKey)
	set("domainAssociation", m.DomainAssociation)
	return b
}

func (r *providerCredentialsResource) put(ctx context.Context, m providerCredentialsModel) error {
	return r.d.Client.SetCredentials(ctx, m.Tenant.ValueString(), m.Provider.ValueString(), m.body())
}

func (r *providerCredentialsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan providerCredentialsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, plan); err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not set the provider credentials", err)
		return
	}
	plan.ID = types.StringValue(plan.Tenant.ValueString() + "/" + plan.Provider.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *providerCredentialsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st providerCredentialsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	list, err := r.d.Client.ListProviders(ctx, st.Tenant.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not read the providers", err)
		return
	}
	for _, p := range list {
		if p.Provider == st.Provider.ValueString() {
			if p.Source != "own" {
				resp.State.RemoveResource(ctx)
				return
			}
			if p.ClientID != "" {
				st.ClientID = types.StringValue(p.ClientID)
			}
			resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *providerCredentialsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan providerCredentialsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.put(ctx, plan); err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not update the provider credentials", err)
		return
	}
	plan.ID = types.StringValue(plan.Tenant.ValueString() + "/" + plan.Provider.ValueString())
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *providerCredentialsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st providerCredentialsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.d.Client.DeleteCredentials(ctx, st.Tenant.ValueString(), st.Provider.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "Could not remove the provider credentials", err)
	}
}

// ImportState takes `<tenant>/<provider>`; the secrets cannot be read back, so the next plan shows them as a change.
func (r *providerCredentialsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Bad import id", "Use `<tenant slug>/<provider>`, for example `acme/google`.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tenant"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("provider_name"), parts[1])...)
}
