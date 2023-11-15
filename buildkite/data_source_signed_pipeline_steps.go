package buildkite

import (
	"context"
	"fmt"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/go-pipeline"
	"github.com/buildkite/go-pipeline/signature"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"gopkg.in/yaml.v3"
)

type signedPipelineStepsDataSource struct {
	UnisignedSteps types.String `tfsdk:"unsigned_steps"`
	Repository     types.String `tfsdk:"repository"`
	JWKS           types.String `tfsdk:"jwks"`
	JWKSKeyID      types.String `tfsdk:"jwks_key_id"`
	Steps          types.String `tfsdk:"steps"`
}

func newSignedPipelineStepsDataSource() datasource.DataSource {
	return &signedPipelineStepsDataSource{}
}

func (s *signedPipelineStepsDataSource) Metadata(
	ctx context.Context,
	req datasource.MetadataRequest,
	resp *datasource.MetadataResponse,
) {
	resp.TypeName = req.ProviderTypeName + "_signed_pipeline_steps"
}

func (s *signedPipelineStepsDataSource) Schema(
	ctx context.Context,
	req datasource.SchemaRequest,
	resp *datasource.SchemaResponse,
) {
	resp.Schema = schema.Schema{
		Description: "A data source that can be used to sign pipeline steps with a JWKS key",
		MarkdownDescription: heredoc.Docf(
			`
				Use this data source to sign pipeline steps with a JWKS key. You will need to have the
				corresponding verification key present on the agents that run this the steps in this
				pipeline. You can use then use these steps in a buildkite_pipeline resource.

				~> **Security Notice** The secret key required to use this data source will be stored
				*unencrypted* in your Terraform state file. If you wish to avoid this, you can manually
				sign the pipeline line steps using the [buildkite agent CLI](https://buildkite.com/docs/agent/v3/cli-tool#sign-a-step).

				## Example Usage
				%s
				locals {
				  repository = "git@github.com:my-org/my-repo.git"
				}

				resource "buildkite_pipeline" "my-pipeline" {
				  name       = "my-pipeline"
				  repository = local.repository
				  steps      = data.buildkite_signed_pipeline_steps.my-signed-steps.steps
				}

				data "buildkite_signed_pipeline_steps" "my-signed-steps" {
				  repository  = local.repository
				  jwks        = file(var.jwks_file)
				  jwks_key_id = var.jwks_key_id

				  unsigned_steps = <<YAML
				steps:
				- label: ":pipeline:"
				  command: buildkite-agent pipeline upload
				YAML
				}
				%s

				More info in the Buildkite [documentation](https://buildkite.com/docs/agent/v3/signed_pipelines).
			`,
			"```terraform",
			"```",
		),
		Attributes: map[string]schema.Attribute{
			"unsigned_steps": schema.StringAttribute{
				Description: "The steps to sign in YAML format.",
				Required:    true,
			},
			"repository": schema.StringAttribute{
				Description: "The repository that will be checked out in a build of the pipeline.",
				Required:    true,
			},
			"jwks": schema.StringAttribute{
				Description: "The JSON Web Key Set (JWKS) to use for signing. If the `jwks_key_id` is not specified, and the set contains exactly one key, that key will be used.",
				MarkdownDescription: heredoc.Docf(
					`
						The JSON Web Key Set (JWKS) to use for signing. See [RFC 7517](https://datatracker.ietf.org/doc/html/rfc7517) for more information.
						If the %s is not specified, and the set contains exactly one key, that key will be used.
					`,
					"`jwks_key_id`",
				),
				Required:  true,
				Sensitive: true,
			},
			"jwks_key_id": schema.StringAttribute{
				Description: "The ID of the key in the JSON Web Key Set (JWKS) to use for signing. If this is not specified, and the JWKS contains exactly one key, that key will be used.",

				MarkdownDescription: heredoc.Docf(
					`
						The ID of the key in the JSON Web Key Set (JWKS) to use for signing.
						See [RFC 7517](https://datatracker.ietf.org/doc/html/rfc7517) for more information.

						If this is not specified, and the key set in %s contains exactly one key, that key will be used.
					`,
					"`jwks`",
				),
				Required: false,
				Optional: true,
			},
			"steps": schema.StringAttribute{
				Description: "The signed steps in YAML format.",
				Computed:    true,
			},
		},
	}
}

func (s *signedPipelineStepsDataSource) Read(
	ctx context.Context,
	req datasource.ReadRequest,
	resp *datasource.ReadResponse,
) {
	data := &signedPipelineStepsDataSource{}
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	p, err := pipeline.Parse(strings.NewReader(data.UnisignedSteps.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Unable to parse pipeline steps", err.Error())
		return
	}

	jwks, err := jwk.Parse([]byte(data.JWKS.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Unable to parse JWKS", err.Error())
		return
	}

	var key jwk.Key
	if data.JWKSKeyID.IsNull() {
		if jwks.Len() != 1 {
			resp.Diagnostics.AddError("Cannot find key", "JWKS does not contain exactly one key, but no key ID was specified")
			return
		}
		key, _ = jwks.Key(0)
	} else {
		ok := false
		keyID := data.JWKSKeyID.ValueString()
		if key, ok = jwks.LookupKeyID(keyID); !ok {
			resp.Diagnostics.AddError("Cannot find key", fmt.Sprintf("The key with ID %q was not found in the JWKS", keyID))
			return
		}
	}

	if err := signature.SignPipeline(p, key, data.Repository.ValueString()); err != nil {
		resp.Diagnostics.AddError("Failed to sign pipeline", err.Error())
		return
	}

	signedSteps, err := yaml.Marshal(p)
	if err != nil {
		resp.Diagnostics.AddError("Failed to marshal pipeline", err.Error())
		return
	}

	data.Steps = types.StringValue(string(signedSteps))
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}
