package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/elmerisllc/terraform-provider-algizian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = (*serviceAccountResource)(nil)
	_ resource.ResourceWithConfigure   = (*serviceAccountResource)(nil)
	_ resource.ResourceWithImportState = (*serviceAccountResource)(nil)
	_ resource.Resource                = (*serviceAccountTokenResource)(nil)
	_ resource.ResourceWithConfigure   = (*serviceAccountTokenResource)(nil)
)

func NewServiceAccountResource() resource.Resource      { return &serviceAccountResource{} }
func NewServiceAccountTokenResource() resource.Resource { return &serviceAccountTokenResource{} }

type serviceAccountResource struct{ d *Data }

type serviceAccountModel struct {
	ID   types.String `tfsdk:"id"`
	Org  types.String `tfsdk:"org"`
	Name types.String `tfsdk:"name"`
	Role types.String `tfsdk:"role"`
	Tags types.Map    `tfsdk:"tags"`
}

func (r *serviceAccountResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account"
}

func (r *serviceAccountResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A machine identity of an organization. Its credential (see `algizian_service_account_token`) keeps working when people leave. " +
			"Creating one needs an org admin token or a platform token, not a service account.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"org": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Organization slug; defaults to the provider `org`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{Required: true, Description: "1 to 80 characters."},
			"tags": schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: tagsSchemaDescription, PlanModifiers: []planmodifier.Map{mapplanmodifier.UseStateForUnknown()}},
			"role": schema.StringAttribute{Required: true, Description: "`admin` (limited: never creates tokens, service accounts or changes the owner) or `read`.", Validators: []validator.String{stringvalidator.OneOf("admin", "read")}},
		},
	}
}

func (r *serviceAccountResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.d = configure(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceAccountResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org := orgOrDefault(plan.Org, r.d)
	if org == "" {
		resp.Diagnostics.AddError("No organization", "Set `org` or the provider `org`.")
		return
	}
	body := map[string]any{"name": plan.Name.ValueString(), "role": plan.Role.ValueString()}
	tg, d := tagsFromMap(ctx, plan.Tags)
	resp.Diagnostics.Append(d...)
	if len(tg) > 0 {
		body["tags"] = tg
	}
	sa, err := r.d.Client.CreateServiceAccount(ctx, org, body)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not create the service account", err)
		return
	}
	plan.ID, plan.Org, plan.Tags = types.StringValue(sa.ID), types.StringValue(org), tagsToMap(sa.Tags)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serviceAccountResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st serviceAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	list, err := r.d.Client.ListServiceAccounts(ctx, st.Org.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not list the service accounts", err)
		return
	}
	for _, sa := range list {
		if sa.ID == st.ID.ValueString() {
			st.Name, st.Role, st.Tags = types.StringValue(sa.Name), types.StringValue(sa.Role), tagsToMap(sa.Tags)
			resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *serviceAccountResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, st serviceAccountModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]any{"name": plan.Name.ValueString(), "role": plan.Role.ValueString()}
	if !plan.Tags.IsUnknown() {
		tg, d := tagsFromMap(ctx, plan.Tags)
		resp.Diagnostics.Append(d...)
		if tg == nil {
			tg = map[string]string{}
		}
		body["tags"] = tg
	}
	sa, err := r.d.Client.PatchServiceAccount(ctx, st.Org.ValueString(), st.ID.ValueString(), body)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not update the service account", err)
		return
	}
	plan.ID, plan.Org, plan.Tags = st.ID, st.Org, tagsToMap(sa.Tags)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serviceAccountResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st serviceAccountModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.d.Client.DeleteServiceAccount(ctx, st.Org.ValueString(), st.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "Could not delete the service account (its credentials end with it)", err)
	}
}

// ImportState takes `<org>/<service account id>`.
func (r *serviceAccountResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError("Bad import id", "Use `<org>/<service account id>`, for example `acme/sa_abc123`.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("org"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}

// ---- credential of a service account

type serviceAccountTokenResource struct{ d *Data }

type serviceAccountTokenModel struct {
	ID               types.String `tfsdk:"id"`
	Org              types.String `tfsdk:"org"`
	ServiceAccountID types.String `tfsdk:"service_account_id"`
	Name             types.String `tfsdk:"name"`
	ExpiresAt        types.String `tfsdk:"expires_at"`
	Tags             types.Map    `tfsdk:"tags"`
	Token            types.String `tfsdk:"token"`
	Prefix           types.String `tfsdk:"prefix"`
	CreatedAt        types.String `tfsdk:"created_at"`
}

func (r *serviceAccountTokenResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_service_account_token"
}

func (r *serviceAccountTokenResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	rr := []planmodifier.String{stringplanmodifier.RequiresReplace()}
	resp.Schema = schema.Schema{
		Description: "A credential (`ag_...`) for a service account. The token is shown once by the API and stored in the Terraform state (sensitive): " +
			"write it to a secret store from the configuration, and protect the state. Any change replaces the token. Creating one needs an org admin token or a platform token.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, Description: "The token id (not the secret).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"org": schema.StringAttribute{
				Optional: true, Computed: true, Description: "Organization slug; defaults to the provider `org`.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace(), stringplanmodifier.UseStateForUnknown()},
			},
			"service_account_id": schema.StringAttribute{Required: true, PlanModifiers: rr},
			"name":               schema.StringAttribute{Required: true, Description: "1 to 80 characters.", PlanModifiers: rr},
			"tags":               schema.MapAttribute{Optional: true, Computed: true, ElementType: types.StringType, Description: tagsSchemaDescription + " Tokens take their tags at creation only, so a change replaces the token.", PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplaceIfConfigured(), mapplanmodifier.UseStateForUnknown()}},
			"expires_at":         schema.StringAttribute{Optional: true, Description: "RFC 3339 time after which the token stops working; default: never.", PlanModifiers: rr},
			"token":              schema.StringAttribute{Computed: true, Sensitive: true, Description: "The secret.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"prefix":             schema.StringAttribute{Computed: true, Description: "First characters of the token, as the API lists them.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"created_at":         schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *serviceAccountTokenResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.d = configure(req.ProviderData, &resp.Diagnostics)
}

func (r *serviceAccountTokenResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan serviceAccountTokenModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	org := orgOrDefault(plan.Org, r.d)
	if org == "" {
		resp.Diagnostics.AddError("No organization", "Set `org` or the provider `org`.")
		return
	}
	body := map[string]any{"name": plan.Name.ValueString()}
	if !plan.ExpiresAt.IsNull() {
		body["expiresAt"] = plan.ExpiresAt.ValueString()
	}
	if tg, d := tagsFromMap(ctx, plan.Tags); len(tg) > 0 {
		body["tags"] = tg
	} else {
		resp.Diagnostics.Append(d...)
	}
	t, err := r.d.Client.CreateServiceAccountToken(ctx, org, plan.ServiceAccountID.ValueString(), body)
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not create the token", err)
		return
	}
	plan.ID, plan.Org = types.StringValue(t.ID), types.StringValue(org)
	plan.Tags = tagsToMap(t.Tags)
	plan.Token, plan.Prefix, plan.CreatedAt = types.StringValue(t.Token), types.StringValue(t.Prefix), types.StringValue(t.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *serviceAccountTokenResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st serviceAccountTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	list, err := r.d.Client.ListOrgTokens(ctx, st.Org.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not list the tokens", err)
		return
	}
	for _, t := range list {
		if t.ID == st.ID.ValueString() {
			st.Prefix, st.Tags = types.StringValue(t.Prefix), tagsToMap(t.Tags)
			resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
			return
		}
	}
	resp.State.RemoveResource(ctx) // revoked or expired and removed: Terraform plans a new one
}

func (r *serviceAccountTokenResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Tokens are replaced, not updated", fmt.Sprintf("Every attribute of %s forces a new token. This is a bug in the provider.", "algizian_service_account_token"))
}

func (r *serviceAccountTokenResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st serviceAccountTokenModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.d.Client.DeleteOrgToken(ctx, st.Org.ValueString(), st.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "Could not revoke the token", err)
	}
}
