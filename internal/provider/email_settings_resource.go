package provider

import (
	"context"

	"github.com/elmerisllc/terraform-provider-algizian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
	_ resource.Resource                = (*emailSettingsResource)(nil)
	_ resource.ResourceWithConfigure   = (*emailSettingsResource)(nil)
	_ resource.ResourceWithImportState = (*emailSettingsResource)(nil)
)

func NewEmailSettingsResource() resource.Resource { return &emailSettingsResource{} }

type emailSettingsResource struct{ d *Data }

type emailModel struct {
	ID                  types.String `tfsdk:"id"`
	Tenant              types.String `tfsdk:"tenant"`
	Sender              types.String `tfsdk:"sender"`
	Host                types.String `tfsdk:"host"`
	Port                types.Int64  `tfsdk:"port"`
	Security            types.String `tfsdk:"security"`
	Username            types.String `tfsdk:"username"`
	Password            types.String `tfsdk:"password"`
	FromAddress         types.String `tfsdk:"from_address"`
	FromName            types.String `tfsdk:"from_name"`
	Template            types.String `tfsdk:"template"`
	DailyCap            types.Int64  `tfsdk:"daily_cap"`
	MonthlyCap          types.Int64  `tfsdk:"monthly_cap"`
	Ready               types.Bool   `tfsdk:"ready"`
	Status              types.String `tfsdk:"status"`
	EffectiveDailyCap   types.Int64  `tfsdk:"effective_daily_cap"`
	EffectiveMonthlyCap types.Int64  `tfsdk:"effective_monthly_cap"`
}

func (r *emailSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_email_settings"
}

func (r *emailSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Email-code sign-in of a tenant. Creating it turns the `email` provider on; destroying it deletes the settings and the stored password and turns the provider off. " +
			"Either send through your own SMTP server (`sender = \"own\"`, TLS or STARTTLS only) or through the Algizian sender (`sender = \"platform\"`, where offered, with caps).",
		Attributes: map[string]schema.Attribute{
			"id":     schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"tenant": schema.StringAttribute{Required: true, Description: "Slug of the tenant.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"sender": schema.StringAttribute{Required: true, Description: "`own` or `platform`.", Validators: []validator.String{stringvalidator.OneOf("own", "platform")}},
			"host":   schema.StringAttribute{Optional: true, Description: "SMTP host (own sender)."},
			"port":   schema.Int64Attribute{Optional: true, Description: "465 or 587 (own sender).", Validators: []validator.Int64{int64validator.OneOf(465, 587)}},
			"security": schema.StringAttribute{
				Optional: true, Description: "`tls` (port 465) or `starttls` (port 587); never plain text.",
				Validators: []validator.String{stringvalidator.OneOf("tls", "starttls")},
			},
			"username":     schema.StringAttribute{Optional: true, Description: "SMTP user (own sender)."},
			"password":     schema.StringAttribute{Optional: true, Sensitive: true, Description: "SMTP password (own sender). Write-only for the API; kept in the state."},
			"from_address": schema.StringAttribute{Optional: true, Computed: true, Description: "Sender address (own sender). With the platform sender it is assigned.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"from_name":    schema.StringAttribute{Optional: true, Description: "Sender name; default is the tenant's display name."},
			"template": schema.StringAttribute{
				Optional: true, Computed: true, Description: "`minimal` or `graphic`.",
				Validators:    []validator.String{stringvalidator.OneOf("minimal", "graphic")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"daily_cap":             schema.Int64Attribute{Optional: true, Description: "Mails per day, 1 to 10000.", Validators: []validator.Int64{int64validator.Between(1, 10000)}},
			"monthly_cap":           schema.Int64Attribute{Optional: true, Description: "Platform sender only: mails per month (tenant tokens up to the platform default).", Validators: []validator.Int64{int64validator.Between(1, 1000000)}},
			"ready":                 schema.BoolAttribute{Computed: true, Description: "The settings are complete and email sign-in works."},
			"status":                schema.StringAttribute{Computed: true, Description: "Result of the last test mail: `untested`, ..."},
			"effective_daily_cap":   schema.Int64Attribute{Computed: true},
			"effective_monthly_cap": schema.Int64Attribute{Computed: true},
		},
	}
}

func (r *emailSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.d = configure(req.ProviderData, &resp.Diagnostics)
}

func (m emailModel) body() map[string]any {
	b := map[string]any{"sender": m.Sender.ValueString()}
	s := func(k string, v types.String) {
		if !v.IsNull() && !v.IsUnknown() {
			b[k] = v.ValueString()
		}
	}
	n := func(k string, v types.Int64) {
		if !v.IsNull() && !v.IsUnknown() {
			b[k] = v.ValueInt64()
		}
	}
	s("host", m.Host)
	n("port", m.Port)
	s("security", m.Security)
	s("username", m.Username)
	s("password", m.Password)
	s("fromAddress", m.FromAddress)
	s("fromName", m.FromName)
	s("template", m.Template)
	n("dailyCap", m.DailyCap)
	n("monthlyCap", m.MonthlyCap)
	return b
}

// fillComputed writes what the server decides; the settings the user wrote stay as planned.
func (m *emailModel) fillComputed(e *client.EmailSettings) {
	m.ID = m.Tenant
	m.FromAddress = optString(e.FromAddress)
	m.Template = optString(e.Template)
	m.Ready = types.BoolValue(e.Ready)
	m.Status = types.StringValue(e.Status)
	m.EffectiveDailyCap = types.Int64Value(e.EffectiveDailyCap)
	m.EffectiveMonthlyCap = types.Int64Value(e.EffectiveMonthlyCap)
}

func (r *emailSettingsResource) apply(ctx context.Context, m *emailModel) error {
	if err := r.d.Client.SetEmail(ctx, m.Tenant.ValueString(), m.body()); err != nil {
		return err
	}
	e, err := r.d.Client.GetEmail(ctx, m.Tenant.ValueString())
	if err != nil {
		return err
	}
	m.fillComputed(e)
	return nil
}

func (r *emailSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan emailModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not set the email sign-in settings", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *emailSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st emailModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	e, err := r.d.Client.GetEmail(ctx, st.Tenant.ValueString())
	if client.IsNotFound(err) || (err == nil && !e.Configured) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not read the email sign-in settings", err)
		return
	}
	st.Sender = types.StringValue(e.Sender)
	st.Host = optString(e.Host)
	st.Security = optString(e.Security)
	st.Username = optString(e.Username)
	st.FromName = optString(e.FromName)
	if e.Port != 0 {
		st.Port = types.Int64Value(e.Port)
	} else {
		st.Port = types.Int64Null()
	}
	st.DailyCap, st.MonthlyCap = types.Int64Null(), types.Int64Null()
	if e.DailyCap != nil {
		st.DailyCap = types.Int64Value(*e.DailyCap)
	}
	if e.MonthlyCap != nil {
		st.MonthlyCap = types.Int64Value(*e.MonthlyCap)
	}
	st.fillComputed(e)
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *emailSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan emailModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.apply(ctx, &plan); err != nil {
		apiErrorDiag(&resp.Diagnostics, "Could not update the email sign-in settings", err)
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *emailSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st emailModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.d.Client.DeleteEmail(ctx, st.Tenant.ValueString()); err != nil && !client.IsNotFound(err) {
		apiErrorDiag(&resp.Diagnostics, "Could not delete the email sign-in settings", err)
	}
}

// ImportState takes the tenant slug; the password cannot be read back.
func (r *emailSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tenant"), req.ID)...)
}
