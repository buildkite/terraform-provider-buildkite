package buildkite

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/go-pipeline"
	"github.com/buildkite/go-pipeline/jwkutil"
	"github.com/buildkite/go-pipeline/signature"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/lestrrat-go/jwx/v2/jwa"
	"gopkg.in/yaml.v3"
)

func TestAccBuildkiteSignedPipelineStepsDataSource(t *testing.T) {
	const (
		repository  = "my-repo"
		jwks_key_id = "my-key"
	)

	steps := heredoc.Doc(`
		steps:
		- label: ":pipeline:"
		  command: buildkite-agent pipeline upload
	`)

	privateJWKS, _, err := jwkutil.NewKeyPair(jwks_key_id, jwa.EdDSA)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	privateKey, ok := privateJWKS.Key(0)
	if !ok {
		t.Fatalf("Failed to get private key from JWKS")
	}

	jwks, err := json.Marshal(privateKey)
	if err != nil {
		t.Fatalf("Failed to marshal private key: %v", err)
	}

	p, err := pipeline.Parse(strings.NewReader(steps))
	if err != nil {
		t.Fatalf("Failed to parse pipeline: %v", err)
	}

	if err != signature.SignPipeline(p, privateKey, repository) {
		t.Fatalf("Failed to sign pipeline: %v", err)
	}

	signedSteps, err := yaml.Marshal(p)
	if err != nil {
		t.Fatalf("Failed to marshal signed steps: %v", err)
	}

	t.Run("signed pipeline steps datasource signs the steps", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: heredoc.Docf(
						`
							data "buildkite_signed_pipeline_steps" "my_signed_steps" {
							  repository     = %q
							  jwks           = %q
							  jwks_key_id    = %q
							  unsigned_steps = %q
							}
						`,
						repository,
						jwks,
						jwks_key_id,
						steps,
					),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttr(
							"data.buildkite_signed_pipeline_steps.my_signed_steps",
							"steps",
							string(signedSteps),
						),
					),
				},
			},
		})
	})
}
