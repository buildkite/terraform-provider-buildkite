package stringvalidation

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestWhenStringAttrIsValidator(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		request     validator.StringRequest
		expectError bool
	}{
		"matching value": {
			request: validator.StringRequest{
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
			request: validator.StringRequest{
				ConfigValue: basetypes.NewStringValue("any"),
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
		"unknown value": {
			request: validator.StringRequest{
				ConfigValue: basetypes.NewStringUnknown(),
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
		"null value": {
			request: validator.StringRequest{
				ConfigValue: basetypes.NewStringPointerValue(nil),
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
			resp := validator.StringResponse{
				Diagnostics: diag.Diagnostics{},
			}

			WhenString(path.MatchRoot("name"), "value").ValidateString(context.Background(), testCase.request, &resp)

			if testCase.expectError != resp.Diagnostics.HasError() {
				t.Error("Expected error mismatch")
			}
		})
	}
}
