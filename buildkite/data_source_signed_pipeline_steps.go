package buildkite

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/go-pipeline"
	"github.com/buildkite/go-pipeline/signature"
	"github.com/buildkite/terraform-provider-buildkite/internal/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"gopkg.in/yaml.v3"
)

type signedPipelineStepsDataSource struct {
	UnsignedSteps types.String `tfsdk:"unsigned_steps"`
	Repository    types.String `tfsdk:"repository"`
	JWKS          types.String `tfsdk:"jwks"`
	JWKSFile      types.String `tfsdk:"jwks_file"`
	JWKSKeyID     types.String `tfsdk:"jwks_key_id"`
	Steps         types.String `tfsdk:"steps"`
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
				Use this data source to sign pipeline steps with a JWKS key. You will need to have
				the corresponding verification key present on the agents that run this the steps in
				this pipeline. You can then use these steps in a %s resource.

				See [RFC 7517](https://datatracker.ietf.org/doc/html/rfc7517) for more information
				about the JWKS format.

				See the Buildkite [documentation](https://buildkite.com/docs/agent/v3/signed_pipelines)
				for more info about signed pipelines.
			`,
			"`buildkite_pipeline`",
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
			"jwks_file": schema.StringAttribute{
				Description: "The path to a local file containing the JWKS to use for signing. This will be ignored if `jwks` is set. If `jwks_key_id` is not specified, and the set contains exactly one key, that key will be used.",
				MarkdownDescription: heredoc.Docf(
					`
						The path to a file containing the JSON Web Key Set (JWKS) to use for
						signing. Users will have to ensure that the JWKS file is present on systems
						running Terraform. If %s is specified, this will be ignored and the
						JWKS will be parsed from that value instead. If %s is not specified, and the
						set contains exactly one key, that key will be used.

						~> **Security Notice** The secret key referenced in the %s attribute is
						expected to be stored *unencrypted* as a file on the system running
						Terraform. You are responsible for securing it on this system while
						Terraform is running, and cleaning it up after it has finished running.
					`,
					"`jwks`",
					"`jwks_key_id`",
					"`jwks_file`",
				),
				Optional: true,
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(path.MatchRoot("jwks"), path.MatchRoot("jwks_file")),
					stringvalidator.LengthAtLeast(1),
				},
			},
			"jwks": schema.StringAttribute{
				Description: "The JSON Web Key Set (JWKS) to use for signing. If the `jwks_key_id` is not specified, and the set contains exactly one key, that key will be used.",
				MarkdownDescription: heredoc.Docf(
					`
						The JSON Web Key Set (JWKS) to use for signing.
						If %s is not specified, and the set contains exactly one key, that key will
						be used.

						~> **Security Notice** The secret key in the %s attribute will be stored
						*unencrypted* in your Terraform state file. This attribute is designed for
						users that have systems to to securely manage their state files. If you wish
						to avoid this, use the %s attribute instead.
					`,
					"`jwks_key_id`",
					"`jwks`",
					"`jwks_file`",
				),
				Optional:  true,
				Sensitive: true,
				Validators: []validator.String{
					&datasourcevalidator.JWKSValidator{},
				},
			},
			"jwks_key_id": schema.StringAttribute{
				Description: "The ID of the key in the JSON Web Key Set (JWKS) to use for signing. If this is not specified, and the JWKS contains exactly one key, that key will be used.",
				MarkdownDescription: heredoc.Doc(
					`
						The ID of the key in the JSON Web Key Set (JWKS) to use for signing.
						If this is not specified, and the key set contains exactly one key, that key
						will be used.
					`,
				),
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

	p, err := pipeline.Parse(strings.NewReader(data.UnsignedSteps.ValueString()))
	if err != nil {
		resp.Diagnostics.AddError("Unable to parse pipeline steps", err.Error())
		return
	}

	// validators ensure that only one of `jwks` or `jwks_file` is set
	jwksContents := []byte(data.JWKS.ValueString())
	if len(jwksContents) == 0 {
		jwksContents, err = os.ReadFile(data.JWKSFile.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read JWKS file", err.Error())
			return
		}
	}

	jwks, err := jwk.Parse(jwksContents)
	if err != nil {
		resp.Diagnostics.AddError("Unable to parse JWKS", err.Error())
		return
	}

	var key jwk.Key
	if data.JWKSKeyID.IsNull() {
		if jwks.Len() != 1 {
			resp.Diagnostics.AddError(
				"Cannot find key",
				"JWKS does not contain exactly one key, but no key ID was specified",
			)
			return
		}
		key, _ = jwks.Key(0)
	} else {
		ok := false
		keyID := data.JWKSKeyID.ValueString()
		if key, ok = jwks.LookupKeyID(keyID); !ok {
			resp.Diagnostics.AddError(
				"Cannot find key",
				fmt.Sprintf("The key with ID %q was not found in the JWKS", keyID),
			)
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
