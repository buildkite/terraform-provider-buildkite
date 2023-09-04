package validation

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/helpers/validatordiag"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type whenValidator struct {
	expression path.Expression
	value      string
}

// Description implements validator.Bool.
func (when whenValidator) Description(ctx context.Context) string {
	return when.MarkdownDescription(ctx)
}

// MarkdownDescription implements validator.Bool.
func (when whenValidator) MarkdownDescription(context.Context) string {
	return fmt.Sprintf("This attribute can only be set when the dependent attribute is set to: %q", when.value)
}

// ValidateBool adds a diagnostic error if the configured attribute is not set the the required value
func (when whenValidator) ValidateBool(ctx context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	// if the value is null or unknown then nothing to do
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	// get the value from config
	matchedPaths, diags := req.Config.PathMatches(ctx, when.expression)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	for _, matchedPath := range matchedPaths {
		var matchedPathValue attr.Value

		diags := req.Config.GetAttribute(ctx, matchedPath, &matchedPathValue)
		resp.Diagnostics.Append(diags...)
		// Collect all errors
		if diags.HasError() {
			continue
		}

		// If the matched path value is null or unknown, we cannot compare
		// values, so continue to other matched paths.
		if matchedPathValue.IsNull() || matchedPathValue.IsUnknown() {
			continue
		}

		var val types.String
		diags = tfsdk.ValueAs(ctx, matchedPathValue, &val)
		resp.Diagnostics.Append(diags...)
		if diags.HasError() {
			continue
		}

		if when.value != val.ValueString() {
			resp.Diagnostics.Append(validatordiag.InvalidAttributeValueDiagnostic(matchedPath, fmt.Sprintf("%q", when.value), val.ValueString()))
		}
	}
}

func WhenStringAttrIs(attr path.Expression, value string) validator.Bool {
	return whenValidator{attr, value}
}
