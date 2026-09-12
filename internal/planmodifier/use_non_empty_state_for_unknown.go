package planmodifier

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

type useNonEmptyStateForUnknownModifier struct{}

// Description implements planmodifier.String.
func (useNonEmptyStateForUnknownModifier) Description(context.Context) string {
	return "Once set to a non-empty value, the value of this attribute in state will not change. An empty string in state is treated like null and planned as unknown."
}

// MarkdownDescription implements planmodifier.String.
func (useNonEmptyStateForUnknownModifier) MarkdownDescription(context.Context) string {
	return "Once set to a non-empty value, the value of this attribute in state will not change. An empty string in state is treated like null and planned as unknown."
}

// PlanModifyString implements planmodifier.String.
func (useNonEmptyStateForUnknownModifier) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// "" is what older versions stored when the API had no value, so it must not be copied into the plan
	// either or an apply that returns null would be inconsistent with it
	if req.StateValue.IsNull() || req.StateValue.ValueString() == "" {
		return
	}

	if !req.PlanValue.IsUnknown() {
		return
	}

	if req.ConfigValue.IsUnknown() {
		return
	}

	resp.PlanValue = req.StateValue
}

// UseNonEmptyStateForUnknown behaves like UseNonNullStateForUnknown but also leaves the plan unknown when the
// prior state holds an empty string.
func UseNonEmptyStateForUnknown() planmodifier.String {
	return useNonEmptyStateForUnknownModifier{}
}
