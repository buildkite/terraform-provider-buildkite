package pipelinevalidation

import (
	"context"
	"fmt"

	"github.com/cli/go-gh/pkg/repository"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
)

type RepositoryProvider string

const (
	RepositoryProviderGitHub    RepositoryProvider = "GitHub"
	RepositoryProviderGitLab    RepositoryProvider = "GitLab"
	RepositoryProviderBitbucket RepositoryProvider = "Bitbucket"
	RepositoryProviderPrivate   RepositoryProvider = "Private"
)

type Validator interface {
	validator.Bool
	validator.String
}

type repositoryProviderValidator struct {
	values []RepositoryProvider
}

type repositoryProviderValidatorRequest struct {
	Config      tfsdk.Config
	ConfigValue attr.Value
	Path        path.Path
}

type repositoryProviderValidatorResponse struct {
	Diagnostics diag.Diagnostics
}

// Description implements validator.Bool.
func (when repositoryProviderValidator) Description(ctx context.Context) string {
	return when.MarkdownDescription(ctx)
}

// MarkdownDescription implements validator.Bool.
func (when repositoryProviderValidator) MarkdownDescription(context.Context) string {
	return fmt.Sprintf("This attribute can only be set when the repository provider is one of: %s", when.values)
}

func (when repositoryProviderValidator) Validate(ctx context.Context, req repositoryProviderValidatorRequest, resp *repositoryProviderValidatorResponse) {
	// if the value is null or unknown then nothing to do
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	// get the value from config
	var repository string
	req.Config.GetAttribute(ctx, path.Root("repository"), &repository)
	repoProvider, err := findRepositoryProvider(repository)
	if err != nil {
		resp.Diagnostics.AddError("Could not parse repository URL", repository)
	}

	var found bool
	for _, match := range when.values {
		if repoProvider == match {
			found = true
			break
		}
	}

	if !found {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(req.Path, "Invalid attribute value", fmt.Sprintf("Invalid use when repository is: %s", repository)))
	}
}

func (when repositoryProviderValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	validateReq := repositoryProviderValidatorRequest{
		Config:      req.Config,
		ConfigValue: req.ConfigValue,
		Path:        req.Path,
	}
	validateResp := &repositoryProviderValidatorResponse{}
	when.Validate(ctx, validateReq, validateResp)

	resp.Diagnostics.Append(validateResp.Diagnostics...)
}

// ValidateBool adds a diagnostic error if the configured attribute is not set the the required value
func (when repositoryProviderValidator) ValidateBool(ctx context.Context, req validator.BoolRequest, resp *validator.BoolResponse) {
	validateReq := repositoryProviderValidatorRequest{
		Config:      req.Config,
		ConfigValue: req.ConfigValue,
		Path:        req.Path,
	}
	validateResp := &repositoryProviderValidatorResponse{}
	when.Validate(ctx, validateReq, validateResp)

	resp.Diagnostics.Append(validateResp.Diagnostics...)
}

func WhenRepositoryProviderIs(values ...RepositoryProvider) Validator {
	return repositoryProviderValidator{values}
}

func findRepositoryProvider(repo string) (RepositoryProvider, error) {
	r, err := repository.Parse(repo)
	if err != nil {
		return "", err
	}

	switch r.Host() {
	case "github.com":
		return RepositoryProviderGitHub, nil
	case "bitbucket.org":
		return RepositoryProviderBitbucket, nil
	case "gitlab.com":
		return RepositoryProviderGitLab, nil
	default:
		return RepositoryProviderPrivate, nil
	}
}
