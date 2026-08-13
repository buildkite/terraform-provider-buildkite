package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	notificationServiceProviderWebhook                   = "webhook"
	notificationServiceProviderAWSEventBridge            = "aws_event_bridge"
	notificationServiceProviderDatadogPipelineVisibility = "datadog_pipeline_visibility"
	notificationServiceProviderOpenTelemetryTracing      = "open_telemetry_tracing"
	notificationServiceProviderLinear                    = "linear"
	notificationServiceProviderSlackWorkspace            = "slack_workspace"
)

var notificationServiceUUIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type notificationServiceResource struct {
	client *Client
}

type notificationServiceResourceModel struct {
	ID                        types.String                                       `tfsdk:"id"`
	GraphQLID                 types.String                                       `tfsdk:"graphql_id"`
	ProviderType              types.String                                       `tfsdk:"provider_type"`
	Description               types.String                                       `tfsdk:"description"`
	BranchConfiguration       types.String                                       `tfsdk:"branch_configuration"`
	Enabled                   types.Bool                                         `tfsdk:"enabled"`
	Scope                     types.String                                       `tfsdk:"scope"`
	ScopeUUIDs                types.Set                                          `tfsdk:"scope_uuids"`
	BuildStates               *notificationServiceBuildStatesModel               `tfsdk:"build_states"`
	Webhook                   *notificationServiceWebhookModel                   `tfsdk:"webhook"`
	AWSEventBridge            *notificationServiceAWSEventBridgeModel            `tfsdk:"aws_event_bridge"`
	DatadogPipelineVisibility *notificationServiceDatadogPipelineVisibilityModel `tfsdk:"datadog_pipeline_visibility"`
	OpenTelemetryTracing      *notificationServiceOpenTelemetryTracingModel      `tfsdk:"open_telemetry_tracing"`
	CreatedAt                 types.String                                       `tfsdk:"created_at"`
}

type notificationServiceBuildStatesModel struct {
	BuildPassed   types.Bool `tfsdk:"build_passed"`
	BuildFixed    types.Bool `tfsdk:"build_fixed"`
	BuildFailed   types.Bool `tfsdk:"build_failed"`
	BuildBlocked  types.Bool `tfsdk:"build_blocked"`
	BuildCanceled types.Bool `tfsdk:"build_canceled"`
	BuildFailing  types.Bool `tfsdk:"build_failing"`
	JobActivated  types.Bool `tfsdk:"job_activated"`
}

type notificationServiceWebhookModel struct {
	URL       types.String `tfsdk:"url"`
	Token     types.String `tfsdk:"token"`
	TokenMode types.String `tfsdk:"token_mode"`
	Version   types.Int64  `tfsdk:"version"`
	Events    types.Set    `tfsdk:"events"`
	TLSVerify types.Bool   `tfsdk:"tls_verify"`
}

type notificationServiceAWSEventBridgeModel struct {
	AWSRegion            types.String `tfsdk:"aws_region"`
	AWSAccountID         types.String `tfsdk:"aws_account_id"`
	EventSourceName      types.String `tfsdk:"event_source_name"`
	IncludeBuildMetadata types.String `tfsdk:"include_build_meta_data"`
}

type notificationServiceDatadogPipelineVisibilityModel struct {
	APIKey      types.String `tfsdk:"api_key"`
	DatadogSite types.String `tfsdk:"datadog_site"`
	DatadogTags types.String `tfsdk:"datadog_tags"`
}

type notificationServiceOpenTelemetryTracingModel struct {
	Endpoint           types.String `tfsdk:"endpoint"`
	ServiceName        types.String `tfsdk:"service_name"`
	Headers            types.Map    `tfsdk:"headers"`
	ResourceAttributes types.Map    `tfsdk:"resource_attributes"`
}

type notificationServiceAPIResponse struct {
	ID        string  `json:"id"`
	GraphQLID *string `json:"graphql_id"`
	Provider  struct {
		ID string `json:"id"`
	} `json:"provider"`
	Description         *string  `json:"description"`
	Enabled             bool     `json:"enabled"`
	Scope               string   `json:"scope"`
	ScopeUUIDs          []string `json:"scope_uuids"`
	BranchConfiguration string   `json:"branch_configuration"`
	BuildStates         struct {
		BuildPassed   bool `json:"build_passed"`
		BuildFixed    bool `json:"build_fixed"`
		BuildFailed   bool `json:"build_failed"`
		BuildBlocked  bool `json:"build_blocked"`
		BuildCanceled bool `json:"build_canceled"`
		BuildFailing  bool `json:"build_failing"`
		JobActivated  bool `json:"job_activated"`
	} `json:"build_states"`
	Settings  json.RawMessage `json:"settings"`
	CreatedAt string          `json:"created_at"`
}

type notificationServiceWebhookAPISettings struct {
	URL       *string  `json:"url"`
	Token     *string  `json:"token"`
	TokenMode *string  `json:"token_mode"`
	Version   *int64   `json:"version"`
	Events    []string `json:"events"`
	TLSVerify *bool    `json:"tls_verify"`
}

type notificationServiceAWSEventBridgeAPISettings struct {
	AWSRegion            *string `json:"aws_region"`
	EventSourceName      *string `json:"event_source_name"`
	IncludeBuildMetadata *string `json:"include_build_meta_data"`
}

type notificationServiceDatadogPipelineVisibilityAPISettings struct {
	DatadogSite *string `json:"datadog_site"`
	DatadogTags *string `json:"datadog_tags"`
}

type notificationServiceOpenTelemetryTracingAPISettings struct {
	Endpoint           *string           `json:"endpoint"`
	ServiceName        *string           `json:"service_name"`
	ResourceAttributes map[string]string `json:"resource_attributes"`
}

var (
	_ resource.Resource                = (*notificationServiceResource)(nil)
	_ resource.ResourceWithConfigure   = (*notificationServiceResource)(nil)
	_ resource.ResourceWithImportState = (*notificationServiceResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*notificationServiceResource)(nil)
)

func newNotificationServiceResource() resource.Resource {
	return &notificationServiceResource{}
}

func (r *notificationServiceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notification_service"
}

func (r *notificationServiceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	r.client = req.ProviderData.(*Client)
}

func (r *notificationServiceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			Manages an organization-level Buildkite notification service.

			The API token needs the read_notification_services and write_notification_services
			REST API scopes. Use provider_type rather than provider, which is a reserved
			Terraform meta-argument.

			Secret settings that the Buildkite API masks or omits are preserved from Terraform
			state during refresh. Changes made to those values outside Terraform cannot be detected.
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the notification service.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"graphql_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The GraphQL ID of the notification service, when its provider has a GraphQL node type.",
			},
			"provider_type": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The notification provider type. OAuth-managed `linear` and `slack_workspace` services can be imported but cannot be created with this resource. Legacy `slack` services are not supported.",
				Validators: []validator.String{
					stringvalidator.OneOf(
						notificationServiceProviderWebhook,
						notificationServiceProviderAWSEventBridge,
						notificationServiceProviderDatadogPipelineVisibility,
						notificationServiceProviderOpenTelemetryTracing,
						notificationServiceProviderLinear,
						notificationServiceProviderSlackWorkspace,
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A description of the notification service.",
			},
			"branch_configuration": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "A branch pattern restricting which builds produce notifications.",
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether the notification service is enabled. Defaults to true.",
				Default:             booldefault.StaticBool(true),
			},
			"scope": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Which resources the service applies to. Defaults to `all`.",
				Default:             stringdefault.StaticString("all"),
				Validators: []validator.String{
					stringvalidator.OneOf("all", "some_projects", "some_teams", "some_clusters"),
				},
			},
			"scope_uuids": schema.SetAttribute{
				Optional:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "The project, team, or cluster UUIDs selected by a `some_*` scope.",
				Validators: []validator.Set{
					setvalidator.ValueStringsAre(
						stringvalidator.RegexMatches(notificationServiceUUIDPattern, "must be a UUID"),
					),
				},
			},
			"build_states":                notificationServiceBuildStatesSchema(),
			"webhook":                     notificationServiceWebhookSchema(),
			"aws_event_bridge":            notificationServiceAWSEventBridgeSchema(),
			"datadog_pipeline_visibility": notificationServiceDatadogPipelineVisibilitySchema(),
			"open_telemetry_tracing":      notificationServiceOpenTelemetryTracingSchema(),
			"created_at": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The time when the notification service was created.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func notificationServiceBuildStatesSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Build and job states that produce notifications. Omitted values use the provider's API defaults.",
		Attributes: map[string]schema.Attribute{
			"build_passed":   notificationServiceOptionalComputedBool("Notify when a build passes."),
			"build_fixed":    notificationServiceOptionalComputedBool("Notify when a previously failing build passes."),
			"build_failed":   notificationServiceOptionalComputedBool("Notify when a build fails."),
			"build_blocked":  notificationServiceOptionalComputedBool("Notify when a build becomes blocked."),
			"build_canceled": notificationServiceOptionalComputedBool("Notify when a build is canceled."),
			"build_failing":  notificationServiceOptionalComputedBool("Notify when a build starts failing."),
			"job_activated":  notificationServiceOptionalComputedBool("Notify when a job is activated."),
		},
	}
}

func notificationServiceWebhookSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Settings for the `webhook` provider.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The URL that receives webhook deliveries. Required when creating a webhook service.",
			},
			"token": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The token used to authenticate webhook deliveries. The API generates one when omitted.",
			},
			"token_mode": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "How the webhook token authenticates deliveries.",
				Validators: []validator.String{
					stringvalidator.OneOf("token", "signature"),
				},
			},
			"version": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The webhook payload version. New services must use the API's latest version.",
			},
			"events": notificationServiceEventsAttribute("The webhook event names to deliver."),
			"tls_verify": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Whether to verify the webhook endpoint's TLS certificate.",
			},
		},
	}
}

func notificationServiceAWSEventBridgeSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Settings for the `aws_event_bridge` provider.",
		Attributes: map[string]schema.Attribute{
			"aws_region": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The AWS region for the partner event source. Required on create and immutable.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"aws_account_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The AWS account ID for the partner event source. Required on create, immutable, and masked by the API.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"event_source_name": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The AWS partner event source created for this service.",
			},
			"include_build_meta_data": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "A build meta-data pattern to include in EventBridge payloads.",
			},
		},
	}
}

func notificationServiceDatadogPipelineVisibilitySchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Settings for the `datadog_pipeline_visibility` provider.",
		Attributes: map[string]schema.Attribute{
			"api_key": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				MarkdownDescription: "The Datadog API key. Required on create and masked by the API.",
			},
			"datadog_site": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The Datadog site, such as `datadoghq.com` or `datadoghq.eu`.",
			},
			"datadog_tags": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Newline-delimited `key:value` tags attached to Datadog events.",
			},
		},
	}
}

func notificationServiceOpenTelemetryTracingSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Optional:            true,
		MarkdownDescription: "Settings for the `open_telemetry_tracing` provider.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "The OTLP HTTP endpoint. Required on create.",
			},
			"service_name": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "The OpenTelemetry service name. Defaults to `buildkite`.",
			},
			"headers": schema.MapAttribute{
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				ElementType:         types.StringType,
				MarkdownDescription: "Headers sent to the OTLP endpoint. Buildkite never returns these values, so they are preserved only in Terraform state.",
			},
			"resource_attributes": schema.MapAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "Additional OpenTelemetry resource attributes.",
			},
		},
	}
}

func notificationServiceOptionalComputedBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		Optional:            true,
		Computed:            true,
		MarkdownDescription: description,
	}
}

func notificationServiceEventsAttribute(description string) schema.SetAttribute {
	return schema.SetAttribute{
		Optional:            true,
		Computed:            true,
		ElementType:         types.StringType,
		MarkdownDescription: description,
	}
}

func (r *notificationServiceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var config notificationServiceResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() || config.ProviderType.IsNull() || config.ProviderType.IsUnknown() {
		return
	}

	providerType := config.ProviderType.ValueString()
	configuredSettings := config.configuredSettings()
	if len(configuredSettings) > 1 {
		resp.Diagnostics.AddError(
			"Multiple notification service settings configured",
			"Configure only the settings object matching provider_type.",
		)
		return
	}
	if len(configuredSettings) == 1 && configuredSettings[0] != providerType {
		resp.Diagnostics.AddError(
			"Notification service settings do not match provider_type",
			fmt.Sprintf("The %q settings object cannot be used with provider_type %q.", configuredSettings[0], providerType),
		)
		return
	}

	if !req.State.Raw.IsNull() {
		return
	}

	if providerType == notificationServiceProviderLinear || providerType == notificationServiceProviderSlackWorkspace {
		resp.Diagnostics.AddError(
			"OAuth-managed notification service cannot be created",
			fmt.Sprintf("%s notification services must be connected in the Buildkite web UI and then imported into Terraform.", providerType),
		)
		return
	}

	if len(configuredSettings) == 0 {
		resp.Diagnostics.AddError(
			"Notification service settings are required",
			fmt.Sprintf("Configure the %q settings object when creating a %q notification service.", providerType, providerType),
		)
		return
	}

	switch providerType {
	case notificationServiceProviderWebhook:
		addMissingNotificationServiceSetting(&resp.Diagnostics, config.Webhook.URL, "webhook.url")
	case notificationServiceProviderAWSEventBridge:
		addMissingNotificationServiceSetting(&resp.Diagnostics, config.AWSEventBridge.AWSRegion, "aws_event_bridge.aws_region")
		addMissingNotificationServiceSetting(&resp.Diagnostics, config.AWSEventBridge.AWSAccountID, "aws_event_bridge.aws_account_id")
	case notificationServiceProviderDatadogPipelineVisibility:
		addMissingNotificationServiceSetting(&resp.Diagnostics, config.DatadogPipelineVisibility.APIKey, "datadog_pipeline_visibility.api_key")
	case notificationServiceProviderOpenTelemetryTracing:
		addMissingNotificationServiceSetting(&resp.Diagnostics, config.OpenTelemetryTracing.Endpoint, "open_telemetry_tracing.endpoint")
	}
}

func (m *notificationServiceResourceModel) configuredSettings() []string {
	var configured []string
	if m.Webhook != nil {
		configured = append(configured, notificationServiceProviderWebhook)
	}
	if m.AWSEventBridge != nil {
		configured = append(configured, notificationServiceProviderAWSEventBridge)
	}
	if m.DatadogPipelineVisibility != nil {
		configured = append(configured, notificationServiceProviderDatadogPipelineVisibility)
	}
	if m.OpenTelemetryTracing != nil {
		configured = append(configured, notificationServiceProviderOpenTelemetryTracing)
	}
	return configured
}

func addMissingNotificationServiceSetting(diags *diag.Diagnostics, value types.String, name string) {
	if value.IsNull() {
		diags.AddError(
			"Missing notification service setting",
			fmt.Sprintf("%s is required when creating this notification service.", name),
		)
	}
}

func (r *notificationServiceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan notificationServiceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, diags := plan.createPayload(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.create(ctx, payload)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create notification service", err.Error())
		return
	}

	state := plan
	resp.Diagnostics.Append(state.applyAPIResponse(ctx, created, plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Enabled.ValueBool() {
		return
	}

	disabled, err := r.setEnabled(ctx, state.ID.ValueString(), false)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to disable notification service",
			fmt.Sprintf("The notification service was created and saved in Terraform state, but disabling it failed: %s", err),
		)
		return
	}

	resp.Diagnostics.Append(state.applyAPIResponse(ctx, disabled, state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *notificationServiceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state notificationServiceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	result, err := r.get(ctx, state.ID.ValueString())
	if err != nil {
		if isAPIStatus(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read notification service", err.Error())
		return
	}

	previous := state
	resp.Diagnostics.Append(state.applyAPIResponse(ctx, result, previous)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *notificationServiceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan notificationServiceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var state notificationServiceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload, diags := plan.updatePayload(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var result *notificationServiceAPIResponse
	var err error
	if len(payload) > 0 {
		result, err = r.update(ctx, state.ID.ValueString(), payload)
		if err != nil {
			resp.Diagnostics.AddError("Unable to update notification service", err.Error())
			return
		}
	}

	if !plan.Enabled.Equal(state.Enabled) {
		result, err = r.setEnabled(ctx, state.ID.ValueString(), plan.Enabled.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError("Unable to change notification service enabled state", err.Error())
			return
		}
	}

	if result == nil {
		result, err = r.get(ctx, state.ID.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to read updated notification service", err.Error())
			return
		}
	}

	resp.Diagnostics.Append(plan.applyAPIResponse(ctx, result, state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *notificationServiceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state notificationServiceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.delete(ctx, state.ID.ValueString())
	if err != nil && !isAPIStatus(err, http.StatusNotFound) {
		resp.Diagnostics.AddError("Unable to delete notification service", err.Error())
	}
}

func (r *notificationServiceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !notificationServiceUUIDPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Invalid notification service import ID", "Import using the notification service UUID.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}

func (m notificationServiceResourceModel) createPayload(ctx context.Context) (map[string]any, diag.Diagnostics) {
	payload := map[string]any{"provider": m.ProviderType.ValueString()}
	if !m.Description.IsNull() && !m.Description.IsUnknown() {
		payload["description"] = m.Description.ValueString()
	}
	if !m.BranchConfiguration.IsNull() && !m.BranchConfiguration.IsUnknown() {
		payload["branch_configuration"] = m.BranchConfiguration.ValueString()
	}
	if !m.Scope.IsNull() && !m.Scope.IsUnknown() {
		payload["scope"] = m.Scope.ValueString()
	}

	var diags diag.Diagnostics
	if !m.ScopeUUIDs.IsNull() && !m.ScopeUUIDs.IsUnknown() {
		values, valueDiags := notificationServiceStringSet(ctx, m.ScopeUUIDs)
		diags.Append(valueDiags...)
		if len(values) > 0 {
			payload["scope_uuids"] = values
		}
	}
	if m.BuildStates != nil {
		payload["build_states"] = m.BuildStates.createPayload()
	}

	settings, settingsDiags := m.settingsCreatePayload(ctx)
	diags.Append(settingsDiags...)
	payload["settings"] = settings
	return payload, diags
}

func (m notificationServiceResourceModel) updatePayload(ctx context.Context, state notificationServiceResourceModel) (map[string]any, diag.Diagnostics) {
	payload := make(map[string]any)
	if !m.Description.Equal(state.Description) {
		payload["description"] = notificationServiceNullableString(m.Description)
	}
	if !m.BranchConfiguration.Equal(state.BranchConfiguration) {
		payload["branch_configuration"] = notificationServiceNullableString(m.BranchConfiguration)
	}
	if !m.Scope.Equal(state.Scope) {
		payload["scope"] = notificationServiceNullableString(m.Scope)
	}

	var diags diag.Diagnostics
	if !m.ScopeUUIDs.Equal(state.ScopeUUIDs) && !m.ScopeUUIDs.IsUnknown() {
		if m.ScopeUUIDs.IsNull() {
			payload["scope_uuids"] = nil
		} else {
			values, valueDiags := notificationServiceStringSet(ctx, m.ScopeUUIDs)
			diags.Append(valueDiags...)
			payload["scope_uuids"] = values
		}
	}

	if states := m.buildStatesUpdatePayload(state); len(states) > 0 {
		payload["build_states"] = states
	}
	settings, settingsDiags := m.settingsUpdatePayload(ctx, state)
	diags.Append(settingsDiags...)
	if len(settings) > 0 {
		payload["settings"] = settings
	}
	return payload, diags
}

func notificationServiceNullableString(value types.String) any {
	if value.IsNull() {
		return nil
	}
	return value.ValueString()
}

func (m *notificationServiceBuildStatesModel) createPayload() map[string]bool {
	payload := make(map[string]bool)
	addNotificationServiceBool(payload, "build_passed", m.BuildPassed)
	addNotificationServiceBool(payload, "build_fixed", m.BuildFixed)
	addNotificationServiceBool(payload, "build_failed", m.BuildFailed)
	addNotificationServiceBool(payload, "build_blocked", m.BuildBlocked)
	addNotificationServiceBool(payload, "build_canceled", m.BuildCanceled)
	addNotificationServiceBool(payload, "build_failing", m.BuildFailing)
	addNotificationServiceBool(payload, "job_activated", m.JobActivated)
	return payload
}

func addNotificationServiceBool(payload map[string]bool, key string, value types.Bool) {
	if !value.IsNull() && !value.IsUnknown() {
		payload[key] = value.ValueBool()
	}
}

func (m notificationServiceResourceModel) buildStatesUpdatePayload(state notificationServiceResourceModel) map[string]bool {
	if m.BuildStates == nil {
		return nil
	}
	if state.BuildStates == nil {
		return m.BuildStates.createPayload()
	}

	payload := make(map[string]bool)
	addChangedNotificationServiceBool(payload, "build_passed", m.BuildStates.BuildPassed, state.BuildStates.BuildPassed)
	addChangedNotificationServiceBool(payload, "build_fixed", m.BuildStates.BuildFixed, state.BuildStates.BuildFixed)
	addChangedNotificationServiceBool(payload, "build_failed", m.BuildStates.BuildFailed, state.BuildStates.BuildFailed)
	addChangedNotificationServiceBool(payload, "build_blocked", m.BuildStates.BuildBlocked, state.BuildStates.BuildBlocked)
	addChangedNotificationServiceBool(payload, "build_canceled", m.BuildStates.BuildCanceled, state.BuildStates.BuildCanceled)
	addChangedNotificationServiceBool(payload, "build_failing", m.BuildStates.BuildFailing, state.BuildStates.BuildFailing)
	addChangedNotificationServiceBool(payload, "job_activated", m.BuildStates.JobActivated, state.BuildStates.JobActivated)
	return payload
}

func addChangedNotificationServiceBool(payload map[string]bool, key string, plan, state types.Bool) {
	if !plan.IsUnknown() && !plan.Equal(state) {
		payload[key] = plan.ValueBool()
	}
}

func (m notificationServiceResourceModel) settingsCreatePayload(ctx context.Context) (map[string]any, diag.Diagnostics) {
	settings := make(map[string]any)
	var diags diag.Diagnostics

	switch m.ProviderType.ValueString() {
	case notificationServiceProviderWebhook:
		addNotificationServiceString(settings, "url", m.Webhook.URL)
		addNotificationServiceString(settings, "token", m.Webhook.Token)
		addNotificationServiceString(settings, "token_mode", m.Webhook.TokenMode)
		addNotificationServiceInt64(settings, "version", m.Webhook.Version)
		addNotificationServiceBoolAny(settings, "tls_verify", m.Webhook.TLSVerify)
		addNotificationServiceSet(ctx, settings, "events", m.Webhook.Events, &diags)
	case notificationServiceProviderAWSEventBridge:
		addNotificationServiceString(settings, "aws_region", m.AWSEventBridge.AWSRegion)
		addNotificationServiceString(settings, "aws_account_id", m.AWSEventBridge.AWSAccountID)
		addNotificationServiceString(settings, "include_build_meta_data", m.AWSEventBridge.IncludeBuildMetadata)
	case notificationServiceProviderDatadogPipelineVisibility:
		addNotificationServiceString(settings, "api_key", m.DatadogPipelineVisibility.APIKey)
		addNotificationServiceString(settings, "datadog_site", m.DatadogPipelineVisibility.DatadogSite)
		addNotificationServiceString(settings, "datadog_tags", m.DatadogPipelineVisibility.DatadogTags)
	case notificationServiceProviderOpenTelemetryTracing:
		addNotificationServiceString(settings, "endpoint", m.OpenTelemetryTracing.Endpoint)
		addNotificationServiceString(settings, "service_name", m.OpenTelemetryTracing.ServiceName)
		addNotificationServiceMap(ctx, settings, "headers", m.OpenTelemetryTracing.Headers, &diags)
		addNotificationServiceMap(ctx, settings, "resource_attributes", m.OpenTelemetryTracing.ResourceAttributes, &diags)
	}

	return settings, diags
}

func addNotificationServiceString(payload map[string]any, key string, value types.String) {
	if !value.IsNull() && !value.IsUnknown() {
		payload[key] = value.ValueString()
	}
}

func addNotificationServiceInt64(payload map[string]any, key string, value types.Int64) {
	if !value.IsNull() && !value.IsUnknown() {
		payload[key] = value.ValueInt64()
	}
}

func addNotificationServiceBoolAny(payload map[string]any, key string, value types.Bool) {
	if !value.IsNull() && !value.IsUnknown() {
		payload[key] = value.ValueBool()
	}
}

func addNotificationServiceSet(ctx context.Context, payload map[string]any, key string, value types.Set, diags *diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	values, valueDiags := notificationServiceStringSet(ctx, value)
	diags.Append(valueDiags...)
	payload[key] = values
}

func addNotificationServiceMap(ctx context.Context, payload map[string]any, key string, value types.Map, diags *diag.Diagnostics) {
	if value.IsNull() || value.IsUnknown() {
		return
	}
	values, valueDiags := notificationServiceStringMap(ctx, value)
	diags.Append(valueDiags...)
	payload[key] = values
}

func (m notificationServiceResourceModel) settingsUpdatePayload(ctx context.Context, state notificationServiceResourceModel) (map[string]any, diag.Diagnostics) {
	settings := make(map[string]any)
	var diags diag.Diagnostics

	switch m.ProviderType.ValueString() {
	case notificationServiceProviderWebhook:
		if m.Webhook == nil {
			break
		}
		if state.Webhook == nil {
			return m.settingsCreatePayload(ctx)
		}
		addChangedNotificationServiceString(settings, "url", m.Webhook.URL, state.Webhook.URL)
		addChangedNotificationServiceString(settings, "token", m.Webhook.Token, state.Webhook.Token)
		addChangedNotificationServiceString(settings, "token_mode", m.Webhook.TokenMode, state.Webhook.TokenMode)
		addChangedNotificationServiceInt64(settings, "version", m.Webhook.Version, state.Webhook.Version)
		addChangedNotificationServiceBoolAny(settings, "tls_verify", m.Webhook.TLSVerify, state.Webhook.TLSVerify)
		addChangedNotificationServiceSet(ctx, settings, "events", m.Webhook.Events, state.Webhook.Events, &diags)
	case notificationServiceProviderAWSEventBridge:
		if m.AWSEventBridge == nil {
			break
		}
		if state.AWSEventBridge == nil {
			return m.settingsCreatePayload(ctx)
		}
		addChangedNotificationServiceString(settings, "include_build_meta_data", m.AWSEventBridge.IncludeBuildMetadata, state.AWSEventBridge.IncludeBuildMetadata)
	case notificationServiceProviderDatadogPipelineVisibility:
		if m.DatadogPipelineVisibility == nil {
			break
		}
		if state.DatadogPipelineVisibility == nil {
			return m.settingsCreatePayload(ctx)
		}
		addChangedNotificationServiceString(settings, "api_key", m.DatadogPipelineVisibility.APIKey, state.DatadogPipelineVisibility.APIKey)
		addChangedNotificationServiceString(settings, "datadog_site", m.DatadogPipelineVisibility.DatadogSite, state.DatadogPipelineVisibility.DatadogSite)
		addChangedNotificationServiceString(settings, "datadog_tags", m.DatadogPipelineVisibility.DatadogTags, state.DatadogPipelineVisibility.DatadogTags)
	case notificationServiceProviderOpenTelemetryTracing:
		if m.OpenTelemetryTracing == nil {
			break
		}
		if state.OpenTelemetryTracing == nil {
			return m.settingsCreatePayload(ctx)
		}
		addChangedNotificationServiceString(settings, "endpoint", m.OpenTelemetryTracing.Endpoint, state.OpenTelemetryTracing.Endpoint)
		addChangedNotificationServiceString(settings, "service_name", m.OpenTelemetryTracing.ServiceName, state.OpenTelemetryTracing.ServiceName)
		addChangedNotificationServiceMap(ctx, settings, "headers", m.OpenTelemetryTracing.Headers, state.OpenTelemetryTracing.Headers, &diags)
		addChangedNotificationServiceMap(ctx, settings, "resource_attributes", m.OpenTelemetryTracing.ResourceAttributes, state.OpenTelemetryTracing.ResourceAttributes, &diags)
	}

	return settings, diags
}

func addChangedNotificationServiceString(payload map[string]any, key string, plan, state types.String) {
	if plan.IsUnknown() || plan.Equal(state) {
		return
	}
	payload[key] = notificationServiceNullableString(plan)
}

func addChangedNotificationServiceInt64(payload map[string]any, key string, plan, state types.Int64) {
	if plan.IsUnknown() || plan.Equal(state) {
		return
	}
	if plan.IsNull() {
		payload[key] = nil
	} else {
		payload[key] = plan.ValueInt64()
	}
}

func addChangedNotificationServiceBoolAny(payload map[string]any, key string, plan, state types.Bool) {
	if plan.IsUnknown() || plan.Equal(state) {
		return
	}
	if plan.IsNull() {
		payload[key] = nil
	} else {
		payload[key] = plan.ValueBool()
	}
}

func addChangedNotificationServiceSet(ctx context.Context, payload map[string]any, key string, plan, state types.Set, diags *diag.Diagnostics) {
	if plan.IsUnknown() || plan.Equal(state) {
		return
	}
	if plan.IsNull() {
		payload[key] = nil
		return
	}
	values, valueDiags := notificationServiceStringSet(ctx, plan)
	diags.Append(valueDiags...)
	payload[key] = values
}

func addChangedNotificationServiceMap(ctx context.Context, payload map[string]any, key string, plan, state types.Map, diags *diag.Diagnostics) {
	if plan.IsUnknown() || plan.Equal(state) {
		return
	}
	if plan.IsNull() {
		payload[key] = nil
		return
	}
	values, valueDiags := notificationServiceStringMap(ctx, plan)
	diags.Append(valueDiags...)
	payload[key] = values
}

func notificationServiceStringSet(ctx context.Context, value types.Set) ([]string, diag.Diagnostics) {
	var result []string
	diags := value.ElementsAs(ctx, &result, false)
	return result, diags
}

func notificationServiceStringMap(ctx context.Context, value types.Map) (map[string]string, diag.Diagnostics) {
	result := make(map[string]string)
	diags := value.ElementsAs(ctx, &result, false)
	return result, diags
}

func (m *notificationServiceResourceModel) applyAPIResponse(ctx context.Context, result *notificationServiceAPIResponse, previous notificationServiceResourceModel) diag.Diagnostics {
	imported := previous.ProviderType.IsNull()
	if imported {
		m.initializeImportedSettings(result.Provider.ID)
	}

	m.ID = types.StringValue(result.ID)
	m.GraphQLID = types.StringPointerValue(result.GraphQLID)
	m.ProviderType = types.StringValue(result.Provider.ID)
	m.Description = types.StringPointerValue(result.Description)
	if result.BranchConfiguration == "" && m.BranchConfiguration.IsNull() {
		m.BranchConfiguration = types.StringNull()
	} else {
		m.BranchConfiguration = types.StringValue(result.BranchConfiguration)
	}
	m.Enabled = types.BoolValue(result.Enabled)
	m.Scope = types.StringValue(result.Scope)
	m.CreatedAt = types.StringValue(result.CreatedAt)

	var diags diag.Diagnostics
	if len(result.ScopeUUIDs) == 0 && m.ScopeUUIDs.IsNull() {
		m.ScopeUUIDs = types.SetNull(types.StringType)
	} else {
		scopeUUIDs, scopeUUIDDiags := types.SetValueFrom(ctx, types.StringType, result.ScopeUUIDs)
		diags.Append(scopeUUIDDiags...)
		m.ScopeUUIDs = scopeUUIDs
	}

	if m.BuildStates != nil || imported {
		m.BuildStates = &notificationServiceBuildStatesModel{
			BuildPassed:   types.BoolValue(result.BuildStates.BuildPassed),
			BuildFixed:    types.BoolValue(result.BuildStates.BuildFixed),
			BuildFailed:   types.BoolValue(result.BuildStates.BuildFailed),
			BuildBlocked:  types.BoolValue(result.BuildStates.BuildBlocked),
			BuildCanceled: types.BoolValue(result.BuildStates.BuildCanceled),
			BuildFailing:  types.BoolValue(result.BuildStates.BuildFailing),
			JobActivated:  types.BoolValue(result.BuildStates.JobActivated),
		}
	}

	switch result.Provider.ID {
	case notificationServiceProviderWebhook:
		diags.Append(m.applyWebhookAPISettings(ctx, result.Settings)...)
	case notificationServiceProviderAWSEventBridge:
		diags.Append(m.applyAWSEventBridgeAPISettings(result.Settings, previous)...)
	case notificationServiceProviderDatadogPipelineVisibility:
		diags.Append(m.applyDatadogPipelineVisibilityAPISettings(result.Settings, previous)...)
	case notificationServiceProviderOpenTelemetryTracing:
		diags.Append(m.applyOpenTelemetryTracingAPISettings(ctx, result.Settings, previous)...)
	}

	return diags
}

func (m *notificationServiceResourceModel) initializeImportedSettings(providerType string) {
	switch providerType {
	case notificationServiceProviderWebhook:
		m.Webhook = &notificationServiceWebhookModel{}
	case notificationServiceProviderAWSEventBridge:
		m.AWSEventBridge = &notificationServiceAWSEventBridgeModel{AWSAccountID: types.StringNull()}
	case notificationServiceProviderDatadogPipelineVisibility:
		m.DatadogPipelineVisibility = &notificationServiceDatadogPipelineVisibilityModel{APIKey: types.StringNull()}
	case notificationServiceProviderOpenTelemetryTracing:
		m.OpenTelemetryTracing = &notificationServiceOpenTelemetryTracingModel{Headers: types.MapNull(types.StringType)}
	}
}

func (m *notificationServiceResourceModel) applyWebhookAPISettings(ctx context.Context, raw json.RawMessage) diag.Diagnostics {
	if m.Webhook == nil {
		return nil
	}
	var settings notificationServiceWebhookAPISettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return notificationServiceSettingsDiagnostic(notificationServiceProviderWebhook, err)
	}

	events, diags := types.SetValueFrom(ctx, types.StringType, settings.Events)
	m.Webhook = &notificationServiceWebhookModel{
		URL:       types.StringPointerValue(settings.URL),
		Token:     types.StringPointerValue(settings.Token),
		TokenMode: types.StringPointerValue(settings.TokenMode),
		Version:   types.Int64PointerValue(settings.Version),
		Events:    events,
		TLSVerify: types.BoolPointerValue(settings.TLSVerify),
	}
	return diags
}

func (m *notificationServiceResourceModel) applyAWSEventBridgeAPISettings(raw json.RawMessage, previous notificationServiceResourceModel) diag.Diagnostics {
	if m.AWSEventBridge == nil {
		return nil
	}
	var settings notificationServiceAWSEventBridgeAPISettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return notificationServiceSettingsDiagnostic(notificationServiceProviderAWSEventBridge, err)
	}

	previousAccountID := types.StringNull()
	if previous.AWSEventBridge != nil {
		previousAccountID = previous.AWSEventBridge.AWSAccountID
	}
	m.AWSEventBridge = &notificationServiceAWSEventBridgeModel{
		AWSRegion:            types.StringPointerValue(settings.AWSRegion),
		AWSAccountID:         preserveNotificationServiceString(m.AWSEventBridge.AWSAccountID, previousAccountID),
		EventSourceName:      types.StringPointerValue(settings.EventSourceName),
		IncludeBuildMetadata: types.StringPointerValue(settings.IncludeBuildMetadata),
	}
	return nil
}

func (m *notificationServiceResourceModel) applyDatadogPipelineVisibilityAPISettings(raw json.RawMessage, previous notificationServiceResourceModel) diag.Diagnostics {
	if m.DatadogPipelineVisibility == nil {
		return nil
	}
	var settings notificationServiceDatadogPipelineVisibilityAPISettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return notificationServiceSettingsDiagnostic(notificationServiceProviderDatadogPipelineVisibility, err)
	}

	previousAPIKey := types.StringNull()
	if previous.DatadogPipelineVisibility != nil {
		previousAPIKey = previous.DatadogPipelineVisibility.APIKey
	}
	m.DatadogPipelineVisibility = &notificationServiceDatadogPipelineVisibilityModel{
		APIKey:      preserveNotificationServiceString(m.DatadogPipelineVisibility.APIKey, previousAPIKey),
		DatadogSite: types.StringPointerValue(settings.DatadogSite),
		DatadogTags: types.StringPointerValue(settings.DatadogTags),
	}
	return nil
}

func (m *notificationServiceResourceModel) applyOpenTelemetryTracingAPISettings(ctx context.Context, raw json.RawMessage, previous notificationServiceResourceModel) diag.Diagnostics {
	if m.OpenTelemetryTracing == nil {
		return nil
	}
	var settings notificationServiceOpenTelemetryTracingAPISettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return notificationServiceSettingsDiagnostic(notificationServiceProviderOpenTelemetryTracing, err)
	}

	resourceAttributes, diags := types.MapValueFrom(ctx, types.StringType, settings.ResourceAttributes)
	previousHeaders := types.MapNull(types.StringType)
	if previous.OpenTelemetryTracing != nil {
		previousHeaders = previous.OpenTelemetryTracing.Headers
	}
	m.OpenTelemetryTracing = &notificationServiceOpenTelemetryTracingModel{
		Endpoint:           types.StringPointerValue(settings.Endpoint),
		ServiceName:        types.StringPointerValue(settings.ServiceName),
		Headers:            preserveNotificationServiceMap(m.OpenTelemetryTracing.Headers, previousHeaders),
		ResourceAttributes: resourceAttributes,
	}
	return diags
}

func preserveNotificationServiceString(configured, previous types.String) types.String {
	if !configured.IsNull() && !configured.IsUnknown() {
		return configured
	}
	if !previous.IsUnknown() {
		return previous
	}
	return types.StringNull()
}

func preserveNotificationServiceMap(configured, previous types.Map) types.Map {
	if !configured.IsNull() && !configured.IsUnknown() {
		return configured
	}
	if !previous.IsUnknown() {
		return previous
	}
	return types.MapNull(types.StringType)
}

func notificationServiceSettingsDiagnostic(providerType string, err error) diag.Diagnostics {
	var diags diag.Diagnostics
	diags.AddError(
		"Unable to read notification service settings",
		fmt.Sprintf("The API returned invalid %s settings: %s", providerType, err),
	)
	return diags
}

func (r *notificationServiceResource) create(ctx context.Context, payload map[string]any) (*notificationServiceAPIResponse, error) {
	path := fmt.Sprintf("/v2/organizations/%s/services", r.client.organization)
	var result notificationServiceAPIResponse
	if err := r.client.makeRequest(ctx, http.MethodPost, path, payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *notificationServiceResource) get(ctx context.Context, id string) (*notificationServiceAPIResponse, error) {
	path := fmt.Sprintf("/v2/organizations/%s/services/%s", r.client.organization, id)
	var result notificationServiceAPIResponse
	if err := r.client.makeRequest(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *notificationServiceResource) update(ctx context.Context, id string, payload map[string]any) (*notificationServiceAPIResponse, error) {
	path := fmt.Sprintf("/v2/organizations/%s/services/%s", r.client.organization, id)
	var result notificationServiceAPIResponse
	if err := r.client.makeRequest(ctx, http.MethodPatch, path, payload, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *notificationServiceResource) setEnabled(ctx context.Context, id string, enabled bool) (*notificationServiceAPIResponse, error) {
	action := "disable"
	if enabled {
		action = "enable"
	}
	path := fmt.Sprintf("/v2/organizations/%s/services/%s/%s", r.client.organization, id, action)
	var result notificationServiceAPIResponse
	if err := r.client.makeRequest(ctx, http.MethodPut, path, nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *notificationServiceResource) delete(ctx context.Context, id string) error {
	path := fmt.Sprintf("/v2/organizations/%s/services/%s", r.client.organization, id)
	return r.client.makeRequest(ctx, http.MethodDelete, path, nil, nil)
}
