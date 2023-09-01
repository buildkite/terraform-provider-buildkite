package validation

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/helpers/validatordiag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

type whenValidator struct {
	attr, value string
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
	// get the value from config
	path := path.Root(when.attr)
	var val string
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path, &val)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if when.value != val {
		resp.Diagnostics.Append(validatordiag.InvalidAttributeValueDiagnostic(path, fmt.Sprintf("%q", when.value), val))
	}
}

func WhenStringAttrIs(attr, value string) validator.Bool {
	return whenValidator{attr, value}
}
