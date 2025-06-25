package planmodifier

import (
	"context"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	slugInvalidChars = regexp.MustCompile(`[^a-z0-9]+`)
	slugRepeatHyphen = regexp.MustCompile(`-+`)
)

// Slugify generates a slug from a string, suitable for use in Buildkite URLs.
func Slugify(s string) string {
	s = strings.ToLower(s)
	s = slugInvalidChars.ReplaceAllString(s, "-")
	s = slugRepeatHyphen.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

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
	// If the user has explicitly set a slug in their configuration, we don't need to do anything.
	if !req.ConfigValue.IsNull() {
		return
	}

	// If the plan already has a known slug for any other reason, do nothing.
	if !req.PlanValue.IsUnknown() {
		return
	}

	// Get pipeline name from plan
	var planName types.String
	diags := req.Plan.GetAttribute(ctx, path.Root("name"), &planName)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() || planName.IsUnknown() || planName.IsNull() {
		return
	}

	// If the user has not specified a slug, we will derive it from the name.
	// This handles creation, updates where the name changes, and updates
	// where a user-defined slug is removed.
	resp.PlanValue = types.StringValue(Slugify(planName.ValueString()))
}

func UseDerivedPipelineSlug() planmodifier.String {
	return useDerivedPipelineSlugModifier{}
}
