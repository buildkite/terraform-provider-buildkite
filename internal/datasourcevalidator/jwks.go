package datasourcevalidator

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

type JWKSValidator struct{}

func (v *JWKSValidator) Description(ctx context.Context) string {
	return "Validates JSON Web Key Set (JWKS)"
}

func (v *JWKSValidator) MarkdownDescription(ctx context.Context) string {
	return "Validates JSON Web Key Set (JWKS)"
}

func (v *JWKSValidator) ValidateString(
	ctx context.Context,
	req validator.StringRequest,
	resp *validator.StringResponse,
) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	if _, err := jwk.Parse([]byte(req.ConfigValue.ValueString())); err != nil {
		// we should not print the error as it may contain a sensitive value
		resp.Diagnostics.AddError(
			"Unable to parse JWKS",
			"Please provide a valid JSON Web Key Set per RFC 7517",
		)
		return
	}
}
