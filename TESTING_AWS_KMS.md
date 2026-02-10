# AWS KMS Support for Signed Pipeline Steps - Testing Documentation

## Overview

This document describes the test suite for the AWS KMS signing functionality added to the `buildkite_signed_pipeline_steps` data source.

## Test Coverage

### Unit Tests

#### KMS Signer Package (`internal/kmssigner/kms_test.go`)

These tests verify the core AWS KMS signing functionality using mock clients:

1. **TestNewKMS**
   - `fails_with_empty_key_ID`: Ensures proper error handling when no key ID is provided
   - `fails_with_non-SIGN_VERIFY_key_usage`: Validates key usage type checking
   - `fails_with_unsupported_key_spec`: Verifies only supported key specs (ECC_NIST_P256) are accepted
   - `creates_KMS_signer_with_valid_ECC_NIST_P256_key`: Tests successful signer creation

2. **TestKMS_Algorithm**
   - Verifies the Algorithm() method returns the correct JWA algorithm (ES256)

3. **TestKMS_Public**
   - Tests retrieval of the public key from KMS
   - Validates the returned key is an ECDSA public key

4. **TestKMS_Sign**
   - `signs_digest_successfully`: Verifies the Sign() method correctly signs data
   - Validates proper parameters are passed to the KMS service

**Running Unit Tests:**
```bash
go test ./internal/kmssigner/... -v
```

### Integration Tests

#### Data Source Tests (`buildkite/data_source_signed_pipeline_steps_test.go`)

These tests verify the data source integration with AWS KMS:

1. **signed pipeline steps with aws_kms_key_id signs the steps**
   - Tests basic AWS KMS signing functionality
   - Requires: `TEST_AWS_KMS_KEY_ID` environment variable
   - Requires: AWS credentials configured
   - Verifies: Signed output contains signature field

2. **signed pipeline steps with aws_kms_key_id and jwks prefers kms**
   - Tests that AWS KMS takes priority when both KMS and JWKS are provided
   - Requires: `TEST_AWS_KMS_KEY_ID` environment variable
   - Requires: AWS credentials configured

3. **signed pipeline steps with invalid aws_kms_key_id fails**
   - Tests error handling for invalid KMS key IDs
   - Does not require actual AWS credentials (fails early)

4. **signed pipeline steps with aws_kms_key_id handles pipeline with env**
   - Tests signing pipelines with environment variables
   - Requires: `TEST_AWS_KMS_KEY_ID` environment variable
   - Requires: AWS credentials configured
   - Verifies: Environment variables are preserved in signed output

## Running Integration Tests

### Prerequisites

1. **AWS Credentials**: Configure using one of:
   - Environment variables: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`
   - AWS profile: `AWS_PROFILE`
   - IAM role (if running on EC2)

2. **KMS Key**: Create an ECC_NIST_P256 key in AWS KMS with SIGN_VERIFY usage

3. **Set Environment Variable**:
   ```bash
   export TEST_AWS_KMS_KEY_ID="arn:aws:kms:us-east-1:123456789012:key/your-key-id"
   # or use key ID, alias name, or alias ARN
   ```

### Running Tests

```bash
# Run all data source tests (including KMS)
go test ./buildkite -v -run TestAccBuildkiteSignedPipelineStepsDataSource

# Run only KMS-related tests
go test ./buildkite -v -run TestAccBuildkiteSignedPipelineStepsDataSource/aws_kms

# Run without AWS tests (they will be skipped)
unset TEST_AWS_KMS_KEY_ID
go test ./buildkite -v -run TestAccBuildkiteSignedPipelineStepsDataSource
```

## Test Behavior

### Automatic Skipping

The AWS KMS integration tests automatically skip if:
- `TEST_AWS_KMS_KEY_ID` environment variable is not set
- AWS credentials are not configured (`AWS_ACCESS_KEY_ID` or `AWS_PROFILE`)

This allows the test suite to run in environments without AWS access.

### Example Output

```
=== RUN   TestAccBuildkiteSignedPipelineStepsDataSource/signed_pipeline_steps_with_aws_kms_key_id_signs_the_steps
--- SKIP: TestAccBuildkiteSignedPipelineStepsDataSource/signed_pipeline_steps_with_aws_kms_key_id_signs_the_steps (0.00s)
    data_source_signed_pipeline_steps_test.go:263: Skipping AWS KMS test: TEST_AWS_KMS_KEY_ID environment variable not set
```

## CI/CD Integration

### GitHub Actions Example

```yaml
- name: Run Tests with AWS KMS
  env:
    TEST_AWS_KMS_KEY_ID: ${{ secrets.TEST_AWS_KMS_KEY_ID }}
    AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
    AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
    AWS_REGION: us-east-1
  run: go test ./buildkite/... -v
```

### Creating a Test KMS Key

```bash
# Create a KMS key for testing
aws kms create-key \
  --key-usage SIGN_VERIFY \
  --key-spec ECC_NIST_P256 \
  --description "Test key for terraform-provider-buildkite"

# Create an alias for easier reference
aws kms create-alias \
  --alias-name alias/terraform-provider-buildkite-test \
  --target-key-id <key-id-from-above>
```

## Test Maintenance

### Adding New Tests

When adding new AWS KMS tests:

1. Add skip conditions for missing AWS configuration
2. Use descriptive test names that explain what's being tested
3. Verify both success and failure cases
4. Include checks for expected output format

### Mock Updates

If the AWS KMS API changes:

1. Update the mock client in `kms_test.go`
2. Update expected behavior in unit tests
3. Verify integration tests still pass with real AWS KMS

## Troubleshooting

### Tests Fail with "Unable to load AWS configuration"

- Ensure AWS credentials are properly configured
- Check AWS region is set (via `AWS_REGION` environment variable)
- Verify IAM permissions for KMS operations

### Tests Fail with "Unable to create KMS signer"

- Verify the KMS key exists and is accessible
- Check the key is configured with SIGN_VERIFY usage
- Ensure the key spec is ECC_NIST_P256

### Tests Skip Unexpectedly

- Check `TEST_AWS_KMS_KEY_ID` is set
- Verify at least one AWS credential source is available
- Look for skip messages in test output

## Additional Resources

- [AWS KMS Key Specs](https://docs.aws.amazon.com/kms/latest/developerguide/asymmetric-key-specs.html)
- [Buildkite Signed Pipelines](https://buildkite.com/docs/agent/v3/signed_pipelines)
- [Terraform Acceptance Testing](https://developer.hashicorp.com/terraform/plugin/sdkv2/testing/acceptance-tests)
