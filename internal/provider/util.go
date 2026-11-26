package provider

import (
	"context"
	"fmt"

	"github.com/elmerisllc/terraform-provider-algizian/internal/client"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// configure extracts the provider data in a resource or data source Configure method.
func configure(providerData any, diags *diag.Diagnostics) *Data {
	if providerData == nil {
		return nil // the provider is not configured yet (validate and plan before configure)
	}
	d, ok := providerData.(*Data)
	if !ok {
		diags.AddError("Unexpected provider data", fmt.Sprintf("Expected *provider.Data, got %T. This is a bug in the provider.", providerData))
		return nil
	}
	return d
}

// orgOrDefault returns the configured org of a resource, else the provider default.
func orgOrDefault(v types.String, d *Data) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	return d.DefaultOrg
}

func stringList(ctx context.Context, v types.List) ([]string, diag.Diagnostics) {
	var out []string
	if v.IsNull() || v.IsUnknown() {
		return out, nil
	}
	return out, v.ElementsAs(ctx, &out, false)
}

func stringSet(ctx context.Context, v types.Set) ([]string, diag.Diagnostics) {
	var out []string
	if v.IsNull() || v.IsUnknown() {
		return out, nil
	}
	return out, v.ElementsAs(ctx, &out, false)
}

func listOf(vals []string) types.List {
	elems := make([]attr.Value, len(vals))
	for i, s := range vals {
		elems[i] = types.StringValue(s)
	}
	l, _ := types.ListValue(types.StringType, elems)
	return l
}

func setOf(vals []string) types.Set {
	elems := make([]attr.Value, len(vals))
	for i, s := range vals {
		elems[i] = types.StringValue(s)
	}
	l, _ := types.SetValue(types.StringType, elems)
	return l
}

// optString maps "" to null so an attribute the API does not return stays unset.
func optString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func apiErrorDiag(diags *diag.Diagnostics, summary string, err error) {
	diags.AddError(summary, err.Error())
}

var _ = client.IsNotFound

// tagsFromMap reads a `tags` attribute (null and unknown are "not set").
func tagsFromMap(ctx context.Context, v types.Map) (map[string]string, diag.Diagnostics) {
	if v.IsNull() || v.IsUnknown() {
		return nil, nil
	}
	out := map[string]string{}
	return out, v.ElementsAs(ctx, &out, false)
}

// tagsToMap writes the API's tags (an empty object when none) into an attribute value.
func tagsToMap(tags map[string]string) types.Map {
	elems := make(map[string]attr.Value, len(tags))
	for k, v := range tags {
		elems[k] = types.StringValue(v)
	}
	m, _ := types.MapValue(types.StringType, elems)
	return m
}

// tagsSchemaDescription is shared by every resource that has tags.
const tagsSchemaDescription = "Free labels (at most 20; keys start with a letter and use `a-z 0-9 . _ : -`, values up to 256 characters). Algizian never reads them. The map replaces the object's tags as a whole; set `{}` to clear them."
