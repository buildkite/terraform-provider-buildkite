package planmodifier

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type useDerivedPipelineSlugModifier struct{}

// Description implements planmodifier.String.
func (useDerivedPipelineSlugModifier) Description(context.Context) string {
	return "Once set, the value of this attribute in state will only change if the dependent attribute changes."
}

// MarkdownDescription implements planmodifier.String.
func (useDerivedPipelineSlugModifier) MarkdownDescription(context.Context) string {
	return "Once set, the value of this attribute in state will only change if the dependent attribute changes."
}

// PlanModifyString implements planmodifier.String.
func (m useDerivedPipelineSlugModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// Do nothing if there is no state value (new resource creation)
	if req.StateValue.IsNull() {
		return
	}

	// Retrieve slugSource from private state
	privateSlugSource, _ := req.Private.GetKey(ctx, "slugSource")

	var slugSource map[string]interface{}
	if err := json.Unmarshal(privateSlugSource, &slugSource); err != nil {
		// Return unknown if slugSource missing from private state AND not user-defined
		if req.ConfigValue.IsNull() {
			resp.PlanValue = types.StringUnknown()
		}
		return
	}
	slugSourceVal := slugSource["source"].(string)

	// Retrieve name from state, plan
	var planValueName, stateValueName string
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("name"), &stateValueName)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &planValueName)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Check if slug is user-provided attribute
	if req.ConfigValue.IsNull() {
		// Return unknown if slug not defined and previous slug source not API (re-generate slug from API)
		if slugSourceVal != "api" {
			resp.PlanValue = types.StringUnknown()
			return
		}
		// Return unknown if pipeline name is changing (re-generate slug from API)
		if planValueName != stateValueName {
			resp.PlanValue = types.StringUnknown()
			return
		}
	}
}

func UseDerivedPipelineSlug() planmodifier.String {
	return useDerivedPipelineSlugModifier{}
}
