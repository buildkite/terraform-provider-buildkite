package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type pipelineWebhookTriggerResource struct{ client *Client }

type pipelineWebhookTriggerModel struct {
	ID             types.String                               `tfsdk:"id"`
	PipelineID     types.String                               `tfsdk:"pipeline_id"`
	Label          types.String                               `tfsdk:"label"`
	Enabled        types.Bool                                 `tfsdk:"enabled"`
	Build          *pipelineWebhookTriggerBuildModel          `tfsdk:"build"`
	EffectiveBuild *pipelineWebhookTriggerEffectiveBuildModel `tfsdk:"effective_build"`
	Filter         types.String                               `tfsdk:"filter"`
	EndpointURL    types.String                               `tfsdk:"endpoint_url"`
}

type pipelineWebhookTriggerBuildModel struct {
	Branch      types.String `tfsdk:"branch"`
	Commit      types.String `tfsdk:"commit"`
	Message     types.String `tfsdk:"message"`
	Environment types.Map    `tfsdk:"environment"`
}

type pipelineWebhookTriggerEffectiveBuildModel struct {
	Branch  types.String `tfsdk:"branch"`
	Commit  types.String `tfsdk:"commit"`
	Message types.String `tfsdk:"message"`
}

type pipelineWebhookTriggerResponse struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Label       string `json:"label"`
	Enabled     bool   `json:"enabled"`
	EndpointURL string `json:"endpoint_url"`
	Build       *struct {
		Branch  *string `json:"branch"`
		Commit  *string `json:"commit"`
		Message *string `json:"message"`
	} `json:"build"`
	Filter *struct {
		Expression string `json:"expression"`
	} `json:"filter"`
}

var (
	_ resource.Resource                = (*pipelineWebhookTriggerResource)(nil)
	_ resource.ResourceWithConfigure   = (*pipelineWebhookTriggerResource)(nil)
	_ resource.ResourceWithImportState = (*pipelineWebhookTriggerResource)(nil)
)

func newPipelineWebhookTriggerResource() resource.Resource { return &pipelineWebhookTriggerResource{} }

func (r *pipelineWebhookTriggerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_pipeline_webhook_trigger"
}

func (r *pipelineWebhookTriggerResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*Client)
	}
}

func (r *pipelineWebhookTriggerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Manages a generic incoming webhook trigger that creates builds for a Buildkite pipeline.
			This uses the [pipeline triggers REST API](https://buildkite.com/docs/apis/rest-api/pipeline-triggers),
			which is in public preview. The API token needs read_pipelines and write_pipelines scopes
			and Full Access to the receiving pipeline.

			This resource manages triggers of type webhook. It is separate from buildkite_pipeline_webhook,
			which provisions a source repository webhook. Generic triggers do not support signature verification.

			Buildkite returns the full endpoint URL only at creation. Terraform retains it as a sensitive
			attribute in state; refresh and updates cannot recover it. Import leaves endpoint_url unset.
			Replace the resource to generate a new URL. Protect state and plan files containing credentials.
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The trigger UUID.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"pipeline_id": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "GraphQL ID of the receiving pipeline in the provider's organization. Changing the pipeline replaces the trigger; renaming it does not.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"label": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A non-empty label describing the trigger.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether the trigger accepts deliveries. Defaults to true.",
			},
			"filter": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "CEL expression selecting deliveries that create builds. Requires webhook filtering enabled for the organization. Removing this attribute clears the filter.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"endpoint_url": schema.StringAttribute{
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "Credential-bearing URL returned only when the trigger is created. Retained in Terraform state; unavailable after import.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"build": schema.SingleNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Build overrides. Omitted fields use pipeline defaults when creating a trigger. Removing a managed override restores its default.",
				Attributes: map[string]schema.Attribute{
					"branch": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Branch to build.",
					},
					"commit": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Commit to build, such as HEAD.",
					},
					"message": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "Build message.",
					},
					"environment": schema.MapAttribute{
						Optional:            true,
						Sensitive:           true,
						ElementType:         types.StringType,
						MarkdownDescription: "Build environment variables. Values are stored in state and are not returned by the API, so external value changes cannot be detected. Removing this map clears previously managed variables.",
					},
				},
			},
			"effective_build": schema.SingleNestedAttribute{
				Computed:            true,
				MarkdownDescription: "Build settings reported by Buildkite, including defaults and overrides made outside Terraform. These values are recovered during import.",
				Attributes: map[string]schema.Attribute{
					"branch":  schema.StringAttribute{Computed: true, MarkdownDescription: "Effective branch."},
					"commit":  schema.StringAttribute{Computed: true, MarkdownDescription: "Effective commit."},
					"message": schema.StringAttribute{Computed: true, MarkdownDescription: "Effective build message."},
				},
			},
		},
	}
}

func (r *pipelineWebhookTriggerResource) apiPath(pipeline, id string) string {
	p := fmt.Sprintf("/v2/organizations/%s/pipelines/%s/triggers", url.PathEscape(r.client.organization), url.PathEscape(pipeline))
	if id != "" {
		p += "/" + url.PathEscape(id)
	}
	return p
}

func (m pipelineWebhookTriggerModel) payload(ctx context.Context, previous *pipelineWebhookTriggerModel) (map[string]any, diag.Diagnostics) {
	payload := map[string]any{"label": m.Label.ValueString(), "enabled": m.Enabled.ValueBool()}
	if previous == nil {
		payload["type"] = "webhook"
	}
	if !m.Filter.IsNull() {
		payload["filter"] = map[string]any{"expression": m.Filter.ValueString()}
	} else if previous != nil && !previous.Filter.IsNull() {
		payload["filter"] = nil
	}
	var diags diag.Diagnostics
	var before *pipelineWebhookTriggerBuildModel
	if previous != nil {
		before = previous.Build
	}
	if m.Build != nil || before != nil {
		build := map[string]any{}
		current := m.Build
		if current == nil {
			current = &pipelineWebhookTriggerBuildModel{}
		}
		fields := map[string]types.String{"branch": current.Branch, "commit": current.Commit, "message": current.Message}
		previousFields := map[string]types.String{}
		if before != nil {
			previousFields = map[string]types.String{"branch": before.Branch, "commit": before.Commit, "message": before.Message}
		}
		for key, value := range fields {
			if !value.IsNull() {
				build[key] = value.ValueString()
			} else if old, ok := previousFields[key]; ok && !old.IsNull() {
				build[key] = nil
			}
		}
		if !current.Environment.IsNull() {
			environment := map[string]string{}
			diags.Append(current.Environment.ElementsAs(ctx, &environment, false)...)
			build["environment"] = environment
		} else if before != nil && !before.Environment.IsNull() {
			build["environment"] = map[string]string{}
		}
		if len(build) > 0 {
			payload["build"] = build
		}
	}
	return payload, diags
}

// Refresh readable values, including resolved defaults. Credentials remain in state.
func (m *pipelineWebhookTriggerModel) refresh(result pipelineWebhookTriggerResponse) diag.Diagnostics {
	var diags diag.Diagnostics
	if result.Type != "webhook" {
		diags.AddError("Unsupported pipeline trigger type", "This resource only manages generic webhook triggers.")
		return diags
	}
	if result.ID == "" {
		diags.AddError("Invalid pipeline trigger response", "The Buildkite API returned a trigger without an ID.")
		return diags
	}
	m.ID = types.StringValue(result.ID)
	m.Label = types.StringValue(result.Label)
	m.Enabled = types.BoolValue(result.Enabled)
	m.Filter = types.StringNull()
	if result.Filter != nil {
		m.Filter = types.StringValue(result.Filter.Expression)
	}
	if result.Build == nil {
		diags.AddError("Invalid pipeline trigger response", "The webhook trigger response did not include build settings.")
		return diags
	}
	m.EffectiveBuild = &pipelineWebhookTriggerEffectiveBuildModel{
		Branch:  types.StringPointerValue(result.Build.Branch),
		Commit:  types.StringPointerValue(result.Build.Commit),
		Message: types.StringPointerValue(result.Build.Message),
	}
	if m.Build != nil {
		if !m.Build.Branch.IsNull() {
			m.Build.Branch = m.EffectiveBuild.Branch
		}
		if !m.Build.Commit.IsNull() {
			m.Build.Commit = m.EffectiveBuild.Commit
		}
		if !m.Build.Message.IsNull() {
			m.Build.Message = m.EffectiveBuild.Message
		}
	}

	return diags
}

func (r *pipelineWebhookTriggerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan pipelineWebhookTriggerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("enabled"), &plan.Enabled)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload, diags := plan.payload(ctx, nil)
	resp.Diagnostics.Append(diags...)
	timeout, diags := r.client.createTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result pipelineWebhookTriggerResponse
	pipeline, lookupErr := r.pipelineSlug(ctx, plan.PipelineID.ValueString())
	if lookupErr != nil {
		resp.Diagnostics.AddError("Unable to resolve receiving pipeline", lookupErr.Error())
		return
	}
	if pipeline == "" {
		resp.Diagnostics.AddError("Receiving pipeline not found", "The receiving pipeline is missing or inaccessible.")
		return
	}
	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()
	err := r.client.makeRequest(requestCtx, http.MethodPost, r.apiPath(pipeline, ""), payload, &result)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create pipeline webhook trigger", err.Error())
		return
	}
	plan.EndpointURL = types.StringNull()
	if result.EndpointURL != "" {
		plan.EndpointURL = types.StringValue(result.EndpointURL)
	}
	resp.Diagnostics.Append(plan.refresh(result)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	// Save the ID before reporting a missing creation credential so the resource can be destroyed.
	if result.EndpointURL == "" {
		resp.Diagnostics.AddError("Missing pipeline trigger endpoint URL", "The trigger was created, but the API did not return its endpoint URL. Replace the trigger to obtain a new URL.")
	}
}

func (r *pipelineWebhookTriggerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state pipelineWebhookTriggerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	timeout, diags := r.client.readTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var result pipelineWebhookTriggerResponse
	pipeline, lookupErr := r.pipelineSlug(requestCtx, state.PipelineID.ValueString())
	if lookupErr != nil {
		resp.Diagnostics.AddError("Unable to resolve receiving pipeline", lookupErr.Error())
		return
	}
	if pipeline == "" {
		resp.State.RemoveResource(ctx)
		return
	}
	err := r.client.makeRequest(requestCtx, http.MethodGet, r.apiPath(pipeline, state.ID.ValueString()), nil, &result)
	if isAPIStatus(err, http.StatusNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read pipeline webhook trigger", err.Error())
		return
	}
	resp.Diagnostics.Append(state.refresh(result)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func (r *pipelineWebhookTriggerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state pipelineWebhookTriggerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("enabled"), &plan.Enabled)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	payload, diags := plan.payload(ctx, &state)
	resp.Diagnostics.Append(diags...)
	timeout, diags := r.client.updateTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var result pipelineWebhookTriggerResponse
	pipeline, lookupErr := r.pipelineSlug(ctx, state.PipelineID.ValueString())
	if lookupErr != nil {
		resp.Diagnostics.AddError("Unable to resolve receiving pipeline", lookupErr.Error())
		return
	}
	if pipeline == "" {
		resp.Diagnostics.AddError("Receiving pipeline not found", "The receiving pipeline is missing or inaccessible.")
		return
	}
	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()
	err := r.client.makeRequest(requestCtx, http.MethodPatch, r.apiPath(pipeline, state.ID.ValueString()), payload, &result)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update pipeline webhook trigger", err.Error())
		return
	}
	plan.EndpointURL = state.EndpointURL
	resp.Diagnostics.Append(plan.refresh(result)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	}
}

func (r *pipelineWebhookTriggerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state pipelineWebhookTriggerModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	timeout, diags := r.client.deleteTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	pipeline, lookupErr := r.pipelineSlug(ctx, state.PipelineID.ValueString())
	if lookupErr != nil {
		resp.Diagnostics.AddError("Unable to resolve receiving pipeline", lookupErr.Error())
		return
	}
	if pipeline == "" {
		return
	}
	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()
	err := r.client.makeRequest(requestCtx, http.MethodDelete, r.apiPath(pipeline, state.ID.ValueString()), nil, nil)
	if err != nil && !isAPIStatus(err, http.StatusNotFound) {
		resp.Diagnostics.AddError("Unable to delete pipeline webhook trigger", err.Error())
	}
}

func (r *pipelineWebhookTriggerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	if len(parts) != 2 || parts[0] == "" {
		resp.Diagnostics.AddError("Invalid pipeline webhook trigger import ID", "Import using pipeline-slug/trigger-uuid.")
		return
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		resp.Diagnostics.AddError("Invalid pipeline webhook trigger import ID", "Import using pipeline-slug/trigger-uuid.")
		return
	}
	timeout, diags := r.client.readTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	parent, err := getPipelineId(requestCtx, r.client.genqlient, r.client.organization+"/"+parts[0])
	if err != nil {
		resp.Diagnostics.AddError("Unable to import receiving pipeline", err.Error())
		return
	}
	if parent.Pipeline.Id == "" {
		resp.Diagnostics.AddError("Receiving pipeline not found", "The receiving pipeline is missing or inaccessible.")
		return
	}
	var result pipelineWebhookTriggerResponse
	if err := r.client.makeRequest(requestCtx, http.MethodGet, r.apiPath(parts[0], id.String()), nil, &result); err != nil {
		resp.Diagnostics.AddError("Unable to import pipeline webhook trigger", err.Error())
		return
	}
	state := pipelineWebhookTriggerModel{PipelineID: types.StringValue(parent.Pipeline.Id)}
	resp.Diagnostics.Append(state.refresh(result)...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Build = &pipelineWebhookTriggerBuildModel{
		Branch: state.EffectiveBuild.Branch, Commit: state.EffectiveBuild.Commit, Message: state.EffectiveBuild.Message,
		Environment: types.MapNull(types.StringType),
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	resp.Diagnostics.AddWarning("Endpoint URL cannot be recovered", "Buildkite only returns the full endpoint URL when creating a trigger. Import leaves endpoint_url unset. Environment values cannot be imported. Readable build settings are imported. Configure them to retain the existing overrides; omitting them restores their defaults on the next apply.")
}

// The REST route uses a mutable slug; resolve it from the stable parent ID each time.
func (r *pipelineWebhookTriggerResource) pipelineSlug(ctx context.Context, id string) (string, error) {
	timeout, diags := r.client.readTimeout(ctx)
	if diags.HasError() {
		return "", fmt.Errorf("invalid read timeout: %s", diags.Errors()[0].Detail())
	}
	lookupCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := getPipelineTriggerParent(lookupCtx, r.client.genqlient, id)
	if err != nil {
		return "", err
	}
	parent, ok := result.Node.(*getPipelineTriggerParentNodePipeline)
	if result.Node == nil {
		return "", nil
	}
	if !ok || parent == nil || parent.Id != id || parent.Slug == "" {
		return "", fmt.Errorf("receiving pipeline ID did not resolve to a valid pipeline")
	}
	if parent.Organization.Slug != r.client.organization {
		return "", fmt.Errorf("receiving pipeline belongs to a different organization")
	}
	return parent.Slug, nil
}
