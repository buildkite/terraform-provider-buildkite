package planmodifier

import (
	"context"
	"fmt"

	"github.com/cli/go-gh/pkg/repository"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type RepositoryProvider string

const (
	RepositoryProviderGitHub    RepositoryProvider = "GitHub"
	RepositoryProviderGitLab    RepositoryProvider = "GitLab"
	RepositoryProviderBitbucket RepositoryProvider = "Bitbucket"
	RepositoryProviderPrivate   RepositoryProvider = "Private"
)

type PlanModifier interface {
	planmodifier.Bool
	planmodifier.String
}

type repositoryProviderPlanModifier struct {
	values []RepositoryProvider
}

// PlanModifyString implements PlanModifier.
func (mod repositoryProviderPlanModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	var response repositoryProviderPlanModifierResponse
	mod.PlanModify(ctx, repositoryProviderPlanModifierRequest{
		ConfigValue: req.ConfigValue,
		Plan:        req.Plan,
		Path:        req.Path,
	}, &response)

	resp.Diagnostics.Append(response.Diagnostics...)
}

func (mod repositoryProviderPlanModifier) PlanModify(ctx context.Context, req repositoryProviderPlanModifierRequest, resp *repositoryProviderPlanModifierResponse) {
	// if the value is null or unknown then nothing to do
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}

	// get the value from config
	var repository types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("repository"), &repository)...)
	repoProvider, err := findRepositoryProvider(repository.ValueString())

	if err != nil {
		resp.Diagnostics.AddWarning("Could not parse repository URL to validate configuration", fmt.Sprintf("repository value: %s", repository.ValueString()))
		return
	}

	if repoProvider == RepositoryProviderPrivate {
		return
	}

	var found bool
	for _, match := range mod.values {
		if repoProvider == match {
			found = true
			break
		}
	}

	if !found {
		resp.Diagnostics.Append(diag.NewAttributeErrorDiagnostic(req.Path, "Invalid attribute value", fmt.Sprintf("`%s` cannot be set to %v when repository provider is: %s", req.Path.String(), req.ConfigValue, repoProvider)))
	}
}

type repositoryProviderPlanModifierRequest struct {
	ConfigValue attr.Value
	Plan        tfsdk.Plan
	Path        path.Path
}

type repositoryProviderPlanModifierResponse struct {
	Diagnostics diag.Diagnostics
}

func (when repositoryProviderPlanModifier) Description(ctx context.Context) string {
	return when.MarkdownDescription(ctx)
}

func (when repositoryProviderPlanModifier) MarkdownDescription(context.Context) string {
	return fmt.Sprintf("This attribute can only be set when the repository provider is one of: %s", when.values)
}

func (mod repositoryProviderPlanModifier) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	var response repositoryProviderPlanModifierResponse
	mod.PlanModify(ctx, repositoryProviderPlanModifierRequest{
		ConfigValue: req.ConfigValue,
		Plan:        req.Plan,
		Path:        req.Path,
	}, &response)

	resp.Diagnostics.Append(response.Diagnostics...)
}

func WhenRepositoryProviderIs(values ...RepositoryProvider) PlanModifier {
	return repositoryProviderPlanModifier{values}
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
		return discoverRemoteProvider(repo)
	}
}

// discoverRemoteProvider attempts to finds a matching remote provider URL from the API (eg for on-prem instances)
func discoverRemoteProvider(repo string) (RepositoryProvider, error) {
	return RepositoryProviderPrivate, nil
}
