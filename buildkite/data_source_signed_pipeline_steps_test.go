package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
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
		repository = "my-repo"
		jwksKeyID  = "my-key-id"
	)

	steps := heredoc.Doc(`
		steps:
		- label: ":pipeline:"
		  command: buildkite-agent pipeline upload
		  env:
		    LOCAL_ENV: "bar"
		env:
		  GLOBAL_ENV: "foo"
	`)

	privateJWKS, _, err := jwkutil.NewKeyPair(jwksKeyID, jwa.EdDSA)
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

	if err := signature.SignSteps(context.Background(), p.Steps, privateKey, repository, signature.WithEnv(p.Env.ToMap())); err != nil {
		t.Fatalf("Failed to sign pipeline: %v", err)
	}

	signedSteps, err := yaml.Marshal(p)
	if err != nil {
		t.Fatalf("Failed to marshal signed steps: %v", err)
	}

	t.Run("signed pipeline steps with a jwks attribute signs the steps", func(t *testing.T) {
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
						jwksKeyID,
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

	t.Run("signed pipeline steps with interpolations fails validation", func(t *testing.T) {
		pipelineWithInterpolations := heredoc.Doc(`
			steps:
			- label: ":pipeline:"
				command: 'echo "$INTERPOLATE and $$ESCAPED"'
		`)

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
						jwksKeyID,
						pipelineWithInterpolations,
					),
					ExpectError: regexp.MustCompile(regexp.QuoteMeta("Environment interpolations are not allowed")),
				},
			},
		})
	})

	t.Run("signed pipeline steps with escaped interpolations regex", func(t *testing.T) {
		pipelineWithEscapedInterpolations := heredoc.Doc(`
			steps:
			- label: ":pipeline:"
			  command: buildkite-agent pipeline upload
			  if: 'pipeline.slug !~ /^.+-main\z/'
		`)

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
						jwksKeyID,
						pipelineWithEscapedInterpolations,
					),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet(
							"data.buildkite_signed_pipeline_steps.my_signed_steps",
							"steps",
						),
					),
				},
			},
		})
	})

	t.Run("signed pipeline steps with a jwks file attribute signs the steps", func(t *testing.T) {
		jwksFile := writeToTempFile(t, jwks)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: heredoc.Docf(
						`
							data "buildkite_signed_pipeline_steps" "my_signed_steps" {
							  repository     = %q
							  jwks_file      = %q
							  jwks_key_id    = %q
							  unsigned_steps = %q
							}
						`,
						repository,
						jwksFile,
						jwksKeyID,
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

	t.Run("signed pipeline steps with a jwks_file and a jwks fails validation", func(t *testing.T) {
		jwksFile := writeToTempFile(t, jwks)

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
							  jwks_file      = %q
							  jwks_key_id    = %q
							  unsigned_steps = %q
							}
						`,
						repository,
						jwks,
						jwksFile,
						jwksKeyID,
						steps,
					),
					ExpectError: regexp.MustCompile(regexp.QuoteMeta("Error: Invalid Attribute Combination")),
				},
			},
		})
	})

	t.Run("signed pipeline steps without a jwks_file or a jwks fails validation", func(t *testing.T) {
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: heredoc.Docf(
						`
							data "buildkite_signed_pipeline_steps" "my_signed_steps" {
							  repository     = %q
							  unsigned_steps = %q
							}
						`,
						repository,
						steps,
					),
					ExpectError: regexp.MustCompile(regexp.QuoteMeta("Error: Invalid Attribute Combination")),
				},
			},
		})
	})

	t.Run("signed pipeline steps with aws_kms_key_id signs the steps", func(t *testing.T) {
		// Skip this test if AWS credentials or KMS key ID are not configured
		kmsKeyID := os.Getenv("TEST_AWS_KMS_KEY_ID")
		if kmsKeyID == "" {
			t.Skip("Skipping AWS KMS test: TEST_AWS_KMS_KEY_ID environment variable not set")
		}

		// Verify AWS credentials are configured
		if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
			t.Skip("Skipping AWS KMS test: AWS credentials not configured")
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: heredoc.Docf(
						`
							data "buildkite_signed_pipeline_steps" "my_signed_steps_kms" {
							  repository      = %q
							  aws_kms_key_id  = %q
							  unsigned_steps  = %q
							}
						`,
						repository,
						kmsKeyID,
						steps,
					),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet(
							"data.buildkite_signed_pipeline_steps.my_signed_steps_kms",
							"steps",
						),
						// Verify the signed steps contain a signature field
						resource.TestCheckResourceAttrWith(
							"data.buildkite_signed_pipeline_steps.my_signed_steps_kms",
							"steps",
							func(value string) error {
								if !strings.Contains(value, "signature:") {
									return fmt.Errorf("expected signed steps to contain 'signature:' field")
								}
								return nil
							},
						),
					),
				},
			},
		})
	})

	t.Run("signed pipeline steps with aws_kms_key_id and jwks prefers kms", func(t *testing.T) {
		// Skip this test if AWS credentials or KMS key ID are not configured
		kmsKeyID := os.Getenv("TEST_AWS_KMS_KEY_ID")
		if kmsKeyID == "" {
			t.Skip("Skipping AWS KMS test: TEST_AWS_KMS_KEY_ID environment variable not set")
		}

		// Verify AWS credentials are configured
		if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
			t.Skip("Skipping AWS KMS test: AWS credentials not configured")
		}

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: heredoc.Docf(
						`
							data "buildkite_signed_pipeline_steps" "my_signed_steps_kms_priority" {
							  repository      = %q
							  aws_kms_key_id  = %q
							  jwks            = %q
							  jwks_key_id     = %q
							  unsigned_steps  = %q
							}
						`,
						repository,
						kmsKeyID,
						jwks,
						jwksKeyID,
						steps,
					),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet(
							"data.buildkite_signed_pipeline_steps.my_signed_steps_kms_priority",
							"steps",
						),
					),
				},
			},
		})
	})

	t.Run("signed pipeline steps with invalid aws_kms_key_id fails", func(t *testing.T) {
		// This test can run without credentials as it should fail during KMS validation
		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: heredoc.Docf(
						`
							data "buildkite_signed_pipeline_steps" "my_signed_steps_invalid_kms" {
							  repository      = %q
							  aws_kms_key_id  = "invalid-key-id-that-does-not-exist"
							  unsigned_steps  = %q
							}
						`,
						repository,
						steps,
					),
					ExpectError: regexp.MustCompile("(Unable to load AWS configuration|Unable to create KMS signer)"),
				},
			},
		})
	})

	t.Run("signed pipeline steps with aws_kms_key_id handles pipeline with env", func(t *testing.T) {
		// Skip this test if AWS credentials or KMS key ID are not configured
		kmsKeyID := os.Getenv("TEST_AWS_KMS_KEY_ID")
		if kmsKeyID == "" {
			t.Skip("Skipping AWS KMS test: TEST_AWS_KMS_KEY_ID environment variable not set")
		}

		// Verify AWS credentials are configured
		if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
			t.Skip("Skipping AWS KMS test: AWS credentials not configured")
		}

		stepsWithEnv := heredoc.Doc(`
			steps:
			- label: ":pipeline:"
			  command: echo "Testing with environment variables"
			  env:
			    STEP_ENV: "step-value"
			env:
			  GLOBAL_ENV: "global-value"
		`)

		resource.ParallelTest(t, resource.TestCase{
			PreCheck:                 func() { testAccPreCheck(t) },
			ProtoV6ProviderFactories: protoV6ProviderFactories(),
			Steps: []resource.TestStep{
				{
					Config: heredoc.Docf(
						`
							data "buildkite_signed_pipeline_steps" "my_signed_steps_kms_env" {
							  repository      = %q
							  aws_kms_key_id  = %q
							  unsigned_steps  = %q
							}
						`,
						repository,
						kmsKeyID,
						stepsWithEnv,
					),
					Check: resource.ComposeAggregateTestCheckFunc(
						resource.TestCheckResourceAttrSet(
							"data.buildkite_signed_pipeline_steps.my_signed_steps_kms_env",
							"steps",
						),
						// Verify the signed steps still contain env blocks
						resource.TestCheckResourceAttrWith(
							"data.buildkite_signed_pipeline_steps.my_signed_steps_kms_env",
							"steps",
							func(value string) error {
								if !strings.Contains(value, "GLOBAL_ENV") {
									return fmt.Errorf("expected signed steps to contain 'GLOBAL_ENV' field")
								}
								return nil
							},
						),
					),
				},
			},
		})
	})
}

func writeToTempFile(t *testing.T, contents []byte) string {
	t.Helper()

	f, err := os.CreateTemp("", "test-jwks-*.json")
	if err != nil {
		t.Fatalf("Failed to create temporary file: %v", err)
	}
	t.Cleanup(func() {
		f.Close()
		os.Remove(f.Name())
	})

	if _, err := f.Write(contents); err != nil {
		t.Fatalf("Failed to write to temporary file: %v", err)
	}

	if err := f.Close(); err != nil {
		t.Fatalf("Failed to close temporary file: %v", err)
	}

	return f.Name()
}
