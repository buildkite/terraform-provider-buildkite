package planmodifier

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUseNonEmptyStateForUnknownModifier(t *testing.T) {
	t.Parallel()

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
			// a legacy "" in state stays unknown so an apply that reads null is accepted
			request: planmodifier.StringRequest{
				StateValue:  types.StringValue(""),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			},
		},
		"non-empty-state": {
			// a real value in state is kept, like UseNonNullStateForUnknown
			request: planmodifier.StringRequest{
				StateValue:  types.StringValue("exact"),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringValue("exact"),
			},
		},
		"known-plan": {
			// a configured or already planned value is left alone
			request: planmodifier.StringRequest{
				StateValue:  types.StringValue("exact"),
				PlanValue:   types.StringValue("contains"),
				ConfigValue: types.StringValue("contains"),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringValue("contains"),
			},
		},
		"empty-state-no-change": {
			// with no pending change the plan already equals the state and is not touched
			request: planmodifier.StringRequest{
				StateValue:  types.StringValue(""),
				PlanValue:   types.StringValue(""),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringValue(""),
			},
		},
		"unknown-config": {
			request: planmodifier.StringRequest{
				StateValue:  types.StringValue("exact"),
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

			UseNonEmptyStateForUnknown().PlanModifyString(context.Background(), testCase.request, resp)

			if diff := cmp.Diff(testCase.expected, resp); diff != "" {
				t.Errorf("unexpected difference: %s", diff)
			}
		})
	}
}
