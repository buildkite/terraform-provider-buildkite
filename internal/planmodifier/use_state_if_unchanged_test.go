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

func TestUseStateIfUnchangedModifier(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		request  planmodifier.StringRequest
		expected *planmodifier.StringResponse
	}{
		"null-state": {
			// when we first create the resource, use the unknown
			// value
			request: planmodifier.StringRequest{
				StateValue:  types.StringNull(),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringUnknown(),
			},
		},
		"known-plan": {
			// this would really only happen if we had a plan
			// modifier setting the value before this plan modifier
			// got to it
			//
			// but we still want to preserve that value, in this
			// case
			request: planmodifier.StringRequest{
				StateValue:  types.StringValue("other"),
				PlanValue:   types.StringValue("test"),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringValue("test"),
			},
		},
		"non-null-state-same-plan": {
			// this is the situation we want to preserve the state
			// in
			request: planmodifier.StringRequest{
				State: tfsdk.State{
					Raw: tftypes.NewValue(tftypes.Object{
						AttributeTypes: map[string]tftypes.Type{
							"name": tftypes.String,
						},
					}, map[string]tftypes.Value{
						"name": tftypes.NewValue(tftypes.String, "abcd"),
					}),
					Schema: schema.Schema{
						Attributes: map[string]schema.Attribute{
							"name": schema.StringAttribute{
								Required: true,
							},
						},
					},
				},
				Plan: tfsdk.Plan{
					Raw: tftypes.NewValue(tftypes.Object{
						AttributeTypes: map[string]tftypes.Type{
							"name": tftypes.String,
						},
					}, map[string]tftypes.Value{
						"name": tftypes.NewValue(tftypes.String, "abcd"),
					}),
					Schema: schema.Schema{
						Attributes: map[string]schema.Attribute{
							"name": schema.StringAttribute{
								Required: true,
							},
						},
					},
				},
				StateValue:  types.StringValue("test"),
				PlanValue:   types.StringValue("test"),
				ConfigValue: types.StringNull(),
			},
			expected: &planmodifier.StringResponse{
				PlanValue: types.StringValue("test"),
			},
		},
		"non-null-state-different-plan": {
			// this is the situation we want to mark the plan unknown
			request: planmodifier.StringRequest{
				State: tfsdk.State{
					Raw: tftypes.NewValue(tftypes.Object{
						AttributeTypes: map[string]tftypes.Type{
							"name": tftypes.String,
						},
					}, map[string]tftypes.Value{
						"name": tftypes.NewValue(tftypes.String, "abcd"),
					}),
					Schema: schema.Schema{
						Attributes: map[string]schema.Attribute{
							"name": schema.StringAttribute{
								Required: true,
							},
						},
					},
				},
				Plan: tfsdk.Plan{
					Raw: tftypes.NewValue(tftypes.Object{
						AttributeTypes: map[string]tftypes.Type{
							"name": tftypes.String,
						},
					}, map[string]tftypes.Value{
						"name": tftypes.NewValue(tftypes.String, "xyz"),
					}),
					Schema: schema.Schema{
						Attributes: map[string]schema.Attribute{
							"name": schema.StringAttribute{
								Required: true,
							},
						},
					},
				},
				StateValue:  types.StringValue("test"),
				PlanValue:   types.StringUnknown(),
				ConfigValue: types.StringNull(),
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

			UseStateIfUnchanged("name").PlanModifyString(context.Background(), testCase.request, resp)

			if diff := cmp.Diff(testCase.expected, resp); diff != "" {
				t.Errorf("unexpected difference: %s", diff)
			}
		})
	}
}
