package planmodifier

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestUseNonEmptyStateIfUnchangedModifier(t *testing.T) {
	t.Parallel()

	const url = "https://webhook.example.com/deliver/abc"

	dependentSchema := schema.Schema{
		Attributes: map[string]schema.Attribute{
			"repository": schema.StringAttribute{
				Required: true,
			},
		},
	}
	repository := func(value any) tftypes.Value {
		return tftypes.NewValue(tftypes.Object{
			AttributeTypes: map[string]tftypes.Type{
				"repository": tftypes.String,
			},
		}, map[string]tftypes.Value{
			"repository": tftypes.NewValue(tftypes.String, value),
		})
	}
	state := func(value any) tfsdk.State {
		return tfsdk.State{Schema: dependentSchema, Raw: repository(value)}
	}
	plan := func(value any) tfsdk.Plan {
		return tfsdk.Plan{Schema: dependentSchema, Raw: repository(value)}
	}

	testCases := map[string]struct {
		request  planmodifier.StringRequest
		expected *planmodifier.StringResponse
	}{
		"null-state": {
			// on create there is nothing to copy
			request: planmodifier.StringRequest{
				StateValue:  types.StringNull(),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			},
		},
		"empty-state": {
			// a value withheld from the refreshing token stays unknown so the apply can fill it in
			request: planmodifier.StringRequest{
				State:       state("https://github.com/example/repo.git"),
				Plan:        plan("https://github.com/example/repo.git"),
				StateValue:  types.StringValue(""),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			},
		},
		"non-empty-state-same-dependent": {
			// this is the situation we want to preserve the state in
			request: planmodifier.StringRequest{
				State:       state("https://github.com/example/repo.git"),
				Plan:        plan("https://github.com/example/repo.git"),
				StateValue:  types.StringValue(url),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringValue(url),
			},
		},
		"non-empty-state-changed-dependent": {
			request: planmodifier.StringRequest{
				State:       state("https://github.com/example/repo.git"),
				Plan:        plan("https://gitlab.com/example/repo.git"),
				StateValue:  types.StringValue(url),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			},
		},
		"non-empty-state-unknown-dependent": {
			// a dependent attribute derived from another resource may still change
			request: planmodifier.StringRequest{
				State:       state("https://github.com/example/repo.git"),
				Plan:        plan(tftypes.UnknownValue),
				StateValue:  types.StringValue(url),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			},
		},
		"known-plan": {
			// a value already planned by an earlier modifier is left alone
			request: planmodifier.StringRequest{
				State:       state("https://github.com/example/repo.git"),
				Plan:        plan("https://gitlab.com/example/repo.git"),
				StateValue:  types.StringValue(url),
				PlanValue:   types.StringValue("planned"),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringValue("planned"),
			},
		},
		"unknown-config": {
			request: planmodifier.StringRequest{
				State:       state("https://github.com/example/repo.git"),
				Plan:        plan("https://github.com/example/repo.git"),
				StateValue:  types.StringValue(url),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringUnknown(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			},
		},
	}

	for name, testCase := range testCases {
		name, testCase := name, testCase

		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := &planmodifier.StringResponse{
				PlanValue: testCase.request.PlanValue,
			}

			UseNonEmptyStateIfUnchanged("repository").PlanModifyString(context.Background(), testCase.request, resp)

			if diff := cmp.Diff(testCase.expected, resp); diff != "" {
				t.Errorf("unexpected difference: %s", diff)
			}
		})
	}
}
