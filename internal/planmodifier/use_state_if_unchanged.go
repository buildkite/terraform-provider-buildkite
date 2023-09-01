package planmodifier

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type useStateIfUnchangedModifier struct {
	attr string
}

// Description implements planmodifier.String.
func (useStateIfUnchangedModifier) Description(context.Context) string {
	return "Once set, the value of this attribute in state will only change if the dependent attribute changes."
}

// MarkdownDescription implements planmodifier.String.
func (useStateIfUnchangedModifier) MarkdownDescription(context.Context) string {
	return "Once set, the value of this attribute in state will only change if the dependent attribute changes."
}

// PlanModifyString implements planmodifier.String.
func (mod useStateIfUnchangedModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Do nothing if there is no state value.
	if req.StateValue.IsNull() {
		return
	}

	// Do nothing if there is a known planned value.
	if !req.PlanValue.IsUnknown() {
		return
	}

	// Do nothing if there is an unknown configuration value, otherwise interpolation gets messed up.
	if req.ConfigValue.IsUnknown() {
		return
	}

	var planValue, stateValue string
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root(mod.attr), &stateValue)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root(mod.attr), &planValue)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Set the response plan as unknown if the dependent attribute is changing
	if planValue != stateValue {
		resp.PlanValue = types.StringUnknown()
		return
	}

	resp.PlanValue = req.StateValue
}

func UseStateIfUnchanged(attr string) planmodifier.String {
	return useStateIfUnchangedModifier{attr}
}
