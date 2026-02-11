package kmssigner

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"github.com/lestrrat-go/jwx/v2/jwa"
)

// mockKMSClient is a mock implementation of the KMS client for testing
type mockKMSClient struct {
	getPublicKeyFunc func(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error)
	signFunc         func(ctx context.Context, params *kms.SignInput, optFns ...func(*kms.Options)) (*kms.SignOutput, error)
}

func (m *mockKMSClient) GetPublicKey(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
	if m.getPublicKeyFunc != nil {
		return m.getPublicKeyFunc(ctx, params, optFns...)
	}
	return nil, nil
}

func (m *mockKMSClient) Sign(ctx context.Context, params *kms.SignInput, optFns ...func(*kms.Options)) (*kms.SignOutput, error) {
	if m.signFunc != nil {
		return m.signFunc(ctx, params, optFns...)
	}
	return nil, nil
}

// generateTestECKey generates a test ECDSA P-256 key for mocking
func generateTestECKey(t *testing.T) (crypto.PrivateKey, []byte) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate test key: %v", err)
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("Failed to marshal public key: %v", err)
	}

	return privateKey, publicKeyBytes
}

func TestNewKMS(t *testing.T) {
	t.Run("fails with empty key ID", func(t *testing.T) {
		mockClient := &mockKMSClient{}

		_, err := newKMSWithMock(mockClient, "")
		if err != ErrInvalidKeyID {
			t.Errorf("Expected ErrInvalidKeyID, got: %v", err)
		}
	})

	t.Run("fails with non-SIGN_VERIFY key usage", func(t *testing.T) {
		_, publicKeyBytes := generateTestECKey(t)

		mockClient := &mockKMSClient{
			getPublicKeyFunc: func(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
				return &kms.GetPublicKeyOutput{
					KeyId:     aws.String("test-key-id"),
					KeyUsage:  types.KeyUsageTypeEncryptDecrypt,
					KeySpec:   types.KeySpecEccNistP256,
					PublicKey: publicKeyBytes,
				}, nil
			},
		}

		_, err := newKMSWithMock(mockClient, "test-key-id")
		if err == nil {
			t.Fatal("Expected error for non-SIGN_VERIFY key usage, got nil")
		}
		if err.Error() != "invalid key usage. expected SIGN_VERIFY, got \"ENCRYPT_DECRYPT\"" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("fails with unsupported key spec", func(t *testing.T) {
		_, publicKeyBytes := generateTestECKey(t)

		mockClient := &mockKMSClient{
			getPublicKeyFunc: func(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
				return &kms.GetPublicKeyOutput{
					KeyId:     aws.String("test-key-id"),
					KeyUsage:  types.KeyUsageTypeSignVerify,
					KeySpec:   types.KeySpecRsa2048,
					PublicKey: publicKeyBytes,
				}, nil
			},
		}

		_, err := newKMSWithMock(mockClient, "test-key-id")
		if err == nil {
			t.Fatal("Expected error for unsupported key spec, got nil")
		}
	})

	t.Run("creates KMS signer with valid ECC_NIST_P256 key", func(t *testing.T) {
		_, publicKeyBytes := generateTestECKey(t)

		mockClient := &mockKMSClient{
			getPublicKeyFunc: func(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
				return &kms.GetPublicKeyOutput{
					KeyId:     aws.String("test-key-id"),
					KeyUsage:  types.KeyUsageTypeSignVerify,
					KeySpec:   types.KeySpecEccNistP256,
					PublicKey: publicKeyBytes,
					SigningAlgorithms: []types.SigningAlgorithmSpec{
						types.SigningAlgorithmSpecEcdsaSha256,
					},
				}, nil
			},
		}

		signer, err := newKMSWithMock(mockClient, "test-key-id")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if signer.kid != "test-key-id" {
			t.Errorf("Expected key ID 'test-key-id', got: %s", signer.kid)
		}

		if signer.jwaAlg != jwa.ES256 {
			t.Errorf("Expected algorithm ES256, got: %s", signer.jwaAlg)
		}

		if signer.alg != types.SigningAlgorithmSpecEcdsaSha256 {
			t.Errorf("Expected signing algorithm EcdsaSha256, got: %s", signer.alg)
		}
	})
}

func TestKMS_Algorithm(t *testing.T) {
	_, publicKeyBytes := generateTestECKey(t)

	mockClient := &mockKMSClient{
		getPublicKeyFunc: func(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
			return &kms.GetPublicKeyOutput{
				KeyId:     aws.String("test-key-id"),
				KeyUsage:  types.KeyUsageTypeSignVerify,
				KeySpec:   types.KeySpecEccNistP256,
				PublicKey: publicKeyBytes,
			}, nil
		},
	}

	signer, err := newKMSWithMock(mockClient, "test-key-id")
	if err != nil {
		t.Fatalf("Failed to create KMS signer: %v", err)
	}

	if alg := signer.Algorithm(); alg != jwa.ES256 {
		t.Errorf("Expected algorithm ES256, got: %s", alg)
	}
}

func TestKMS_Public(t *testing.T) {
	_, publicKeyBytes := generateTestECKey(t)

	mockClient := &mockKMSClient{
		getPublicKeyFunc: func(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
			return &kms.GetPublicKeyOutput{
				KeyId:     aws.String("test-key-id"),
				KeyUsage:  types.KeyUsageTypeSignVerify,
				KeySpec:   types.KeySpecEccNistP256,
				PublicKey: publicKeyBytes,
			}, nil
		},
	}

	signer, err := newKMSWithMock(mockClient, "test-key-id")
	if err != nil {
		t.Fatalf("Failed to create KMS signer: %v", err)
	}

	publicKey := signer.Public()
	if publicKey == nil {
		t.Fatal("Expected public key, got nil")
	}

	_, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		t.Error("Expected ECDSA public key")
	}
}

func TestKMS_Sign(t *testing.T) {
	_, publicKeyBytes := generateTestECKey(t)

	t.Run("signs digest successfully", func(t *testing.T) {
		expectedSignature := []byte("mock-signature")

		mockClient := &mockKMSClient{
			getPublicKeyFunc: func(ctx context.Context, params *kms.GetPublicKeyInput, optFns ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
				return &kms.GetPublicKeyOutput{
					KeyId:     aws.String("test-key-id"),
					KeyUsage:  types.KeyUsageTypeSignVerify,
					KeySpec:   types.KeySpecEccNistP256,
					PublicKey: publicKeyBytes,
				}, nil
			},
			signFunc: func(ctx context.Context, params *kms.SignInput, optFns ...func(*kms.Options)) (*kms.SignOutput, error) {
				if *params.KeyId != "test-key-id" {
					t.Errorf("Expected key ID 'test-key-id', got: %s", *params.KeyId)
				}
				if params.SigningAlgorithm != types.SigningAlgorithmSpecEcdsaSha256 {
					t.Errorf("Expected signing algorithm EcdsaSha256, got: %s", params.SigningAlgorithm)
				}
				if params.MessageType != types.MessageTypeDigest {
					t.Errorf("Expected message type Digest, got: %s", params.MessageType)
				}
				return &kms.SignOutput{
					Signature: expectedSignature,
				}, nil
			},
		}

		signer, err := newKMSWithMock(mockClient, "test-key-id")
		if err != nil {
			t.Fatalf("Failed to create KMS signer: %v", err)
		}

		digest := []byte("test-digest")
		signature, err := signer.Sign(nil, digest, crypto.SHA256)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if string(signature) != string(expectedSignature) {
			t.Errorf("Expected signature %s, got: %s", expectedSignature, signature)
		}
	})
}

// Helper function to create KMS with a mock client (for testing purposes)
func newKMSWithMock(client kmsClient, kmsKeyID string) (*KMS, error) {
	if kmsKeyID == "" {
		return nil, ErrInvalidKeyID
	}

	keyDesc, err := client.GetPublicKey(context.Background(), &kms.GetPublicKeyInput{KeyId: aws.String(kmsKeyID)})
	if err != nil {
		return nil, fmt.Errorf("failed to describe key %q: %w", kmsKeyID, err)
	}

	if keyDesc.KeyUsage != types.KeyUsageTypeSignVerify {
		return nil, fmt.Errorf("invalid key usage. expected SIGN_VERIFY, got %q", keyDesc.KeyUsage)
	}

	switch keyDesc.KeySpec {
	case types.KeySpecEccNistP256:
		pubKey, err := x509.ParsePKIXPublicKey(keyDesc.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public key: %w", err)
		}
	case types.KeySpecEccNistP256:
	    pubKey, err := x509.ParsePKIXPublicKey(keyDesc.PublicKey)
            if err != nil {
                return nil, fmt.Errorf("failed to parse public key: %w", err)
            }
	    return &KMS{
		client: client,
		kid:    kmsKeyID,
		jwaAlg: jwa.ES256,
		alg:    types.SigningAlgorithmSpecEcdsaSha256,
		pubKey: pubKey,
	    }, nil
	default:
		return nil, fmt.Errorf("unsupported key spec: %q, supported key specs are %q", keyDesc.KeySpec,
			[]types.KeySpec{types.KeySpecEccNistP256})
	}
}
