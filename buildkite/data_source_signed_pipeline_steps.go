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
	Steps       types.String `tfsdk:"steps"`
	Repository  types.String `tfsdk:"repository"`
	JWKS        types.String `tfsdk:"jwks"`
	JWKSKeyID   types.String `tfsdk:"jwks_key_id"`
	SignedSteps types.String `tfsdk:"signed_steps"`
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
		MarkdownDescription: heredoc.Doc(`
			Use this data source to look up properties on a specific pipeline. This is particularly useful for looking up the webhook URL for each pipeline.

			More info in the Buildkite [documentation](https://buildkite.com/docs/pipelines).
		`),
		Attributes: map[string]schema.Attribute{
			"steps": schema.StringAttribute{
				Description: "The steps to sign in YAML format.",
				Required:    true,
			},
			"repository": schema.StringAttribute{
				Description: "The repository that will be checked out in a build of the pipeline.",
				Required:    true,
			},
			"jwks": schema.StringAttribute{
				Description: "The JWKS to use for signing. If the `jwks_key_id` is not specified, and the set contains exactly one key, that key will be used.",
				Required:    true,
			},
			"jwks_key_id": schema.StringAttribute{
				Description: "The ID of the key in the JWKS to use for signing.",
				Required:    false,
				Optional:    true,
			},
			"signed_steps": schema.StringAttribute{
				Description: "The signed steps",
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

	p, err := pipeline.Parse(strings.NewReader(data.Steps.ValueString()))
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

	data.SignedSteps = types.StringValue(string(signedSteps))
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}
