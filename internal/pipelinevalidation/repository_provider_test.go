package pipelinevalidation

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestWhenRepositoryProviderIs(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		expected    RepositoryProvider
		request     validator.BoolRequest
		expectError bool
	}{
		"valid when needs github": {
			expected: RepositoryProviderGitHub,
			request: validator.BoolRequest{
				ConfigValue: types.BoolValue(true),
				Config: tfsdk.Config{
					Raw: tftypes.NewValue(tftypes.Object{
						AttributeTypes: map[string]tftypes.Type{
							"repository": tftypes.String,
						},
					}, map[string]tftypes.Value{
						"repository": tftypes.NewValue(tftypes.String, "git@github.com:buildkite/terraform-provider-buildkite.git"),
					}),
					Schema: schema.Schema{
						Attributes: map[string]schema.Attribute{
							"repository": schema.StringAttribute{
								Required: true,
							},
						},
					},
				},
			},
		},
		"invalid when doesnt match": {
			expectError: true,
			expected:    RepositoryProviderBitbucket,
			request: validator.BoolRequest{
				ConfigValue: types.BoolValue(true),
				Config: tfsdk.Config{
					Raw: tftypes.NewValue(tftypes.Object{
						AttributeTypes: map[string]tftypes.Type{
							"repository": tftypes.String,
						},
					}, map[string]tftypes.Value{
						"repository": tftypes.NewValue(tftypes.String, "https://github.com/buildkite/terraform-provider-buildkite.git"),
					}),
					Schema: schema.Schema{
						Attributes: map[string]schema.Attribute{
							"repository": schema.StringAttribute{
								Required: true,
							},
						},
					},
				},
			},
		},
		"does nothing if not configured": {},
	}
	for name, testCase := range testCases {
		name, testCase := name, testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			resp := validator.BoolResponse{
				Diagnostics: diag.Diagnostics{},
			}

			WhenRepositoryProviderIs(testCase.expected).ValidateBool(context.Background(), testCase.request, &resp)

			if testCase.expectError != resp.Diagnostics.HasError() {
				t.Error(resp.Diagnostics[len(resp.Diagnostics)-1].Detail())
			}
		})
	}
}

func TestFindRepositoryProvider(t *testing.T) {
	t.Parallel()

	t.Run("resolve github", func(t *testing.T) {
		t.Parallel()
		testcases := map[string]string{
			"git url":                     "git@github.com:buildkite/terraform-provider-buildkite.git",
			"https url":                   "https://github.com/buildkite/terraform-provider-buildkite.git",
			"https url without extension": "https://github.com/user/bitbucket-git-repo",
			"https url with user":         "https://user@github.com/buildkite/terraform-provider-buildkite.git",
			"ssh url":                     "git://github.com/buildkite/terraform-provider-buildkite.git",
		}

		for name, input := range testcases {
			t.Run(name, func(t *testing.T) {
				input := input
				t.Parallel()

				provider, _ := findRepositoryProvider(input)

				if provider != RepositoryProviderGitHub {
					t.Errorf("provider does not match %s != %s", provider, RepositoryProviderGitHub)
				}
			})
		}
	})

	t.Run("resolve gitlab", func(t *testing.T) {
		t.Parallel()
		testcases := map[string]string{
			"git url":   "git@gitlab.com:foo/bar.git",
			"https url": "https://user@gitlab.com/user/gitlab-git-repo.git",
		}

		for name, input := range testcases {
			t.Run(name, func(t *testing.T) {
				input := input
				t.Parallel()

				provider, _ := findRepositoryProvider(input)

				if provider != RepositoryProviderGitLab {
					t.Errorf("provider does not match %s != %s", provider, RepositoryProviderGitHub)
				}
			})
		}
	})

	t.Run("resolve bitbucket urls", func(t *testing.T) {
		t.Parallel()
		testcases := map[string]string{
			"git url":   "git@bitbucket.org:foo/bar.git",
			"https url": "https://user@bitbucket.org/user/bitbucket-git-repo.git",
		}

		for name, input := range testcases {
			t.Run(name, func(t *testing.T) {
				input := input
				t.Parallel()

				provider, _ := findRepositoryProvider(input)

				if provider != RepositoryProviderBitbucket {
					t.Errorf("provider does not match %s != %s", provider, RepositoryProviderBitbucket)
				}
			})
		}
	})
}
