package validation

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestWhenStringAttrIsValidator(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		request     validator.BoolRequest
		expectError bool
	}{
		"matching value": {
			request: validator.BoolRequest{
				Config: tfsdk.Config{
					Raw: tftypes.NewValue(tftypes.Object{
						AttributeTypes: map[string]tftypes.Type{
							"name": tftypes.String,
						},
					}, map[string]tftypes.Value{
						"name": tftypes.NewValue(tftypes.String, "value"),
					}),
					Schema: schema.Schema{
						Attributes: map[string]schema.Attribute{
							"name": schema.StringAttribute{
								Required: true,
							},
						},
					},
				},
			},
		},
		"not matching value": {
			expectError: true,
			request: validator.BoolRequest{
				Config: tfsdk.Config{
					Raw: tftypes.NewValue(tftypes.Object{
						AttributeTypes: map[string]tftypes.Type{
							"name": tftypes.String,
						},
					}, map[string]tftypes.Value{
						"name": tftypes.NewValue(tftypes.String, "not value"),
					}),
					Schema: schema.Schema{
						Attributes: map[string]schema.Attribute{
							"name": schema.StringAttribute{
								Required: true,
							},
						},
					},
				},
			},
		},
	}

	for name, testCase := range testCases {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := validator.BoolResponse{
				Diagnostics: diag.Diagnostics{},
			}

			WhenStringAttrIs("name", "value").ValidateBool(context.Background(), testCase.request, &resp)

			if testCase.expectError != resp.Diagnostics.HasError() {
				t.Fail()
			}
		})
	}
}
