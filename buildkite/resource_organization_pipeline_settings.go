package buildkite

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const (
	// The organization accepts a scheduled job expiry of between an hour and 30 days.
	scheduledJobExpiryMinMinutes = 60
	scheduledJobExpiryMaxMinutes = 30 * 24 * 60

	// Each of these toggles has its own sub-route off the pipeline settings resource, rather than
	// being a writable field on it.
	pipelineSettingsPublicPipelinesPath = "public-pipelines"
	pipelineSettingsHostedAgentsSSHPath = "hosted-agents-ssh"
	pipelineSettingsBuildExportPath     = "build-export"

	readOrganizationSettingsScope  = "read_organization_settings"
	writeOrganizationSettingsScope = "write_organization_settings"
)

// buildExportBucketRegex rejects a scheme on the export bucket. The API accepts a fully qualified
// URI but answers with the bare bucket name, so a configured "s3://bucket" would read back as
// "bucket" and leave the apply disagreeing with its own plan.
var buildExportBucketRegex = regexp.MustCompile(`^[^:]*$`)

type organizationPipelineSettingsResourceModel struct {
	ID                                types.String `tfsdk:"id"`
	DefaultBranch                     types.String `tfsdk:"default_branch"`
	DefaultClusterID                  types.String `tfsdk:"default_cluster_id"`
	DefaultTimeoutInMinutes           types.Int64  `tfsdk:"default_timeout_in_minutes"`
	MaximumTimeoutInMinutes           types.Int64  `tfsdk:"maximum_timeout_in_minutes"`
	ScheduledJobExpiryInMinutes       types.Int64  `tfsdk:"scheduled_job_expiry_in_minutes"`
	PublicPipelineCreationEnabled     types.Bool   `tfsdk:"public_pipeline_creation_enabled"`
	HostedAgentsTerminalAccessEnabled types.Bool   `tfsdk:"hosted_agents_terminal_access_enabled"`
	BuildExportLocation               types.String `tfsdk:"build_export_location"`
	BuildExportStrategyID             types.String `tfsdk:"build_export_strategy_id"`
	BuildExportAvailable              types.Bool   `tfsdk:"build_export_available"`
	BuildExportSupportedStrategies    types.List   `tfsdk:"build_export_supported_strategies"`
}

type organizationPipelineSettings struct {
	DefaultBranch               *string `json:"default_branch"`
	DefaultClusterID            *string `json:"default_cluster_id"`
	DefaultTimeoutInMinutes     *int64  `json:"default_timeout_in_minutes"`
	MaximumTimeoutInMinutes     *int64  `json:"maximum_timeout_in_minutes"`
	ScheduledJobExpiryInMinutes *int64  `json:"scheduled_job_expiry_in_minutes"`
	HostedAgentsTerminalAccess  struct {
		Enabled bool `json:"enabled"`
	} `json:"hosted_agents_terminal_access"`
	PublicPipelineCreation struct {
		Enabled bool `json:"enabled"`
	} `json:"public_pipeline_creation"`
	BuildExports struct {
		Available           bool     `json:"available"`
		Enabled             bool     `json:"enabled"`
		Location            *string  `json:"location"`
		StrategyID          *string  `json:"strategy_id"`
		SupportedStrategies []string `json:"supported_strategies"`
	} `json:"build_exports"`
}

type buildExportRequest struct {
	Location   string `json:"location"`
	StrategyID string `json:"strategy_id"`
}

type organizationPipelineSettingsResource struct {
	client *Client
}

func newOrganizationPipelineSettingsResource() resource.Resource {
	return &organizationPipelineSettingsResource{}
}

func (organizationPipelineSettingsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_organization_pipeline_settings"
}

func (*organizationPipelineSettingsResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		// The two halves of the build export have to be configured together, including when they
		// are cleared. Leaving the strategy out of a configuration that clears the location would
		// leave the plan carrying the previous strategy forward with nothing to hold it.
		resourcevalidator.RequiredTogether(
			path.MatchRoot("build_export_location"),
			path.MatchRoot("build_export_strategy_id"),
		),
	}
}

func (o *organizationPipelineSettingsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	o.client = req.ProviderData.(*Client)
}

func (*organizationPipelineSettingsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			This resource allows you to manage the organization-wide pipeline settings, which supply the
			defaults new pipelines are created with.

			An organization has exactly one set of these settings, so this resource adopts whatever the
			organization already has rather than creating anything. Any attribute you leave out is read into
			state and left as it is, which means removing an attribute stops managing the setting instead of
			resetting it. Destroying the resource leaves every setting intact.

			The settings that can be emptied are cleared with an empty string rather than by being removed:
			` + "`default_cluster_id`" + `, and ` + "`build_export_location`" + ` together with
			` + "`build_export_strategy_id`" + `. An import has no configuration to read, so a setting cleared
			that way is imported as unset, and the first plan after the import shows it returning to an empty
			string without anything changing in the organization.

			~> The job timeouts have no such empty value, so ` + "`default_timeout_in_minutes`" + ` and
			` + "`maximum_timeout_in_minutes`" + ` can be changed here but only removed in the web UI.

			The user of your API token must be an organization administrator, and the token needs the
			` + "`read_organization_settings`" + ` and ` + "`write_organization_settings`" + ` scopes.

			-> Advanced queue metrics are deliberately not managed here. The API can only turn them on, so a
			destroy or a rollback could not undo it. Enable them in the web UI, and contact Buildkite support
			to turn them off again.
		`),
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The slug of the organization these settings belong to.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_branch": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The default branch that new pipelines in the organization are created with. " +
					"If omitted, the current setting is left unchanged.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_cluster_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The UUID of the cluster that new pipelines in the organization are created in. " +
					"Set this to an empty string to leave new pipelines without a default cluster. " +
					"If omitted, the current setting is left unchanged.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"default_timeout_in_minutes": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The default command timeout, in minutes, applied to jobs that do not set their own. " +
					"If omitted, the current setting is left unchanged.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"maximum_timeout_in_minutes": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The longest command timeout, in minutes, that any pipeline in the organization may set. " +
					"If omitted, the current setting is left unchanged.",
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"scheduled_job_expiry_in_minutes": schema.Int64Attribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: fmt.Sprintf(
					"How long a scheduled job waits, in minutes, before it expires without having started. Must be between %d and %d. "+
						"If omitted, the current setting is left unchanged.",
					scheduledJobExpiryMinMinutes, scheduledJobExpiryMaxMinutes,
				),
				Validators: []validator.Int64{
					int64validator.Between(scheduledJobExpiryMinMinutes, scheduledJobExpiryMaxMinutes),
				},
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"public_pipeline_creation_enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether members of the organization can create public pipelines. " +
					"If omitted, the current setting is left unchanged.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"hosted_agents_terminal_access_enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Whether remote access to hosted agents is enabled, which covers SSH access to all hosted agents " +
					"and VNC access to supported macOS hosted jobs. If omitted, the current setting is left unchanged.",
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"build_export_location": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The bucket to export build data to, given as a bare bucket name without a scheme. " +
					"Set this and `build_export_strategy_id` to empty strings to stop exporting build data. " +
					"If both are omitted, the current setting is left unchanged.\n\n" +
					"-> Exporting build data is a plan-gated feature. Read `build_export_available` to see whether it is available " +
					"on this organization's plan.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						buildExportBucketRegex,
						`must be a bare bucket name, without a scheme such as "s3://". The scheme follows from build_export_strategy_id`,
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"build_export_strategy_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "The strategy used to export build data, which decides the scheme the bucket is addressed with. " +
					"The strategies this organization accepts are listed in `build_export_supported_strategies`, and are usually " +
					"`s3` and `gcs`. Always configured alongside `build_export_location`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"build_export_available": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether exporting build data is available on this organization's billing plan.",
			},
			"build_export_supported_strategies": schema.ListAttribute{
				Computed:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "The build export strategies that `build_export_strategy_id` accepts.",
			},
		},
	}
}

func (o *organizationPipelineSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var config, plan organizationPipelineSettingsResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Creating pipeline settings for organization %s ...", o.client.organization)
	state := o.apply(ctx, &config, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (o *organizationPipelineSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state organizationPipelineSettingsResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Reading pipeline settings for organization %s ...", o.client.organization)
	settings, err := o.client.getOrganizationPipelineSettings(ctx)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read organization pipeline settings",
			pipelineSettingsErrorDetail("read the organization pipeline settings", readOrganizationSettingsScope, err),
		)
		return
	}

	resp.Diagnostics.Append(state.fromAPI(ctx, o.client.organization, settings)...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (o *organizationPipelineSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var config, plan organizationPipelineSettingsResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	log.Printf("Updating pipeline settings for organization %s ...", o.client.organization)
	state := o.apply(ctx, &config, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Delete leaves the organization's settings as they are. They have no existence apart from the
// organization, so there is nothing to remove and no prior value worth guessing at to restore.
func (o *organizationPipelineSettingsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Organization pipeline settings left intact",
		"The organization keeps the pipeline settings it currently has. Use the web UI if you wish to change them.",
	)
}

func (o *organizationPipelineSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// apply writes the configured settings that differ from the organization's current ones and returns
// the resulting state. Each endpoint answers with the whole resource, so the last response read is
// what the organization now holds.
func (o *organizationPipelineSettingsResource) apply(ctx context.Context, config, plan *organizationPipelineSettingsResourceModel, diags *diag.Diagnostics) *organizationPipelineSettingsResourceModel {
	settings, err := o.client.getOrganizationPipelineSettings(ctx)
	if err != nil {
		diags.AddError(
			"Unable to read organization pipeline settings",
			pipelineSettingsErrorDetail("read the organization pipeline settings", readOrganizationSettingsScope, err),
		)
		return nil
	}

	if payload := pipelineSettingsPatch(config, plan, settings); len(payload) > 0 {
		settings, err = o.client.updateOrganizationPipelineSettings(ctx, payload)
		if err != nil {
			diags.AddError(
				"Unable to update organization pipeline settings",
				pipelineSettingsErrorDetail("update the organization pipeline settings", writeOrganizationSettingsScope, err),
			)
			return nil
		}
	}

	settings = o.applyToggle(ctx, pipelineSettingsPublicPipelinesPath, "public pipeline creation",
		config.PublicPipelineCreationEnabled, plan.PublicPipelineCreationEnabled, settings.PublicPipelineCreation.Enabled, settings, diags)
	settings = o.applyToggle(ctx, pipelineSettingsHostedAgentsSSHPath, "hosted agent remote access",
		config.HostedAgentsTerminalAccessEnabled, plan.HostedAgentsTerminalAccessEnabled, settings.HostedAgentsTerminalAccess.Enabled, settings, diags)
	settings = o.applyBuildExport(ctx, config, plan, settings, diags)
	if diags.HasError() {
		return nil
	}

	state := *plan
	diags.Append(state.fromAPI(ctx, o.client.organization, settings)...)
	return &state
}

// applyToggle drives the sub-route of a setting that is switched rather than written, leaving it
// alone when it is unconfigured or already holds the wanted value.
func (o *organizationPipelineSettingsResource) applyToggle(ctx context.Context, subPath, description string, configured, planned types.Bool, current bool, settings *organizationPipelineSettings, diags *diag.Diagnostics) *organizationPipelineSettings {
	if !managed(configured, planned) || planned.ValueBool() == current {
		return settings
	}

	method := http.MethodDelete
	action := "disable"
	if planned.ValueBool() {
		method = http.MethodPut
		action = "enable"
	}

	updated, err := o.client.setOrganizationPipelineSetting(ctx, method, subPath, nil)
	if err != nil {
		diags.AddError(
			fmt.Sprintf("Unable to %s %s", action, description),
			pipelineSettingsErrorDetail(fmt.Sprintf("%s %s", action, description), writeOrganizationSettingsScope, err),
		)
		return settings
	}

	return updated
}

// applyBuildExport points build data export at a bucket, or stops it. Stopping goes through the
// endpoint's delete rather than through an empty location, which the API refuses.
func (o *organizationPipelineSettingsResource) applyBuildExport(ctx context.Context, config, plan *organizationPipelineSettingsResourceModel, settings *organizationPipelineSettings, diags *diag.Diagnostics) *organizationPipelineSettings {
	// ConfigValidators pair these two, so either both are configured or neither is.
	if !managed(config.BuildExportLocation, plan.BuildExportLocation) || !managed(config.BuildExportStrategyID, plan.BuildExportStrategyID) {
		return settings
	}

	location := plan.BuildExportLocation.ValueString()
	strategy := plan.BuildExportStrategyID.ValueString()

	var (
		method  string
		payload any
	)
	switch {
	case location == "" && strategy != "", location != "" && strategy == "":
		diags.AddAttributeError(
			path.Root("build_export_strategy_id"),
			"Mismatched build export location and strategy",
			fmt.Sprintf(
				"build_export_location and build_export_strategy_id describe one destination, so either both name it or both are empty to stop exporting build data. "+
					"The strategies this organization accepts are: %v.",
				settings.BuildExports.SupportedStrategies,
			),
		)
		return settings

	case location == "":
		if !settings.BuildExports.Enabled {
			return settings
		}
		method = http.MethodDelete

	default:
		if settings.BuildExports.Enabled &&
			stringPtrEqual(settings.BuildExports.Location, &location) &&
			stringPtrEqual(settings.BuildExports.StrategyID, &strategy) {
			return settings
		}
		method = http.MethodPut
		payload = buildExportRequest{Location: location, StrategyID: strategy}
	}

	updated, err := o.client.setOrganizationPipelineSetting(ctx, method, pipelineSettingsBuildExportPath, payload)
	if err != nil {
		detail := pipelineSettingsErrorDetail("update the build data export", writeOrganizationSettingsScope, err)
		if isAPIStatus(err, http.StatusForbidden) && !settings.BuildExports.Available {
			detail += " Exporting build data is not available on this organization's plan."
		}
		diags.AddError("Unable to update build data export", detail)
		return settings
	}

	return updated
}

// fromAPI records the settings the API returned. A setting cleared with an empty string keeps it
// rather than reading back as the null the API answers with, which would leave the apply
// disagreeing with the plan that asked for it.
func (m *organizationPipelineSettingsResourceModel) fromAPI(ctx context.Context, organization string, settings *organizationPipelineSettings) diag.Diagnostics {
	strategies, diags := types.ListValueFrom(ctx, types.StringType, settings.BuildExports.SupportedStrategies)
	if diags.HasError() {
		return diags
	}

	m.ID = types.StringValue(organization)
	m.DefaultBranch = optionalStringValue(settings.DefaultBranch)
	m.DefaultClusterID = clearableStringValue(m.DefaultClusterID, settings.DefaultClusterID)
	m.DefaultTimeoutInMinutes = optionalInt64Value(settings.DefaultTimeoutInMinutes)
	m.MaximumTimeoutInMinutes = optionalInt64Value(settings.MaximumTimeoutInMinutes)
	m.ScheduledJobExpiryInMinutes = optionalInt64Value(settings.ScheduledJobExpiryInMinutes)
	m.PublicPipelineCreationEnabled = types.BoolValue(settings.PublicPipelineCreation.Enabled)
	m.HostedAgentsTerminalAccessEnabled = types.BoolValue(settings.HostedAgentsTerminalAccess.Enabled)
	m.BuildExportLocation = clearableStringValue(m.BuildExportLocation, settings.BuildExports.Location)
	m.BuildExportStrategyID = clearableStringValue(m.BuildExportStrategyID, settings.BuildExports.StrategyID)
	m.BuildExportAvailable = types.BoolValue(settings.BuildExports.Available)
	m.BuildExportSupportedStrategies = strategies

	return diags
}

// pipelineSettingsPatch returns the configured defaults that differ from the organization's current
// ones. config says whether an attribute is managed, plan holds the value to apply.
func pipelineSettingsPatch(config, plan *organizationPipelineSettingsResourceModel, current *organizationPipelineSettings) map[string]any {
	payload := map[string]any{}

	if managed(config.DefaultBranch, plan.DefaultBranch) {
		if branch := plan.DefaultBranch.ValueString(); !stringPtrEqual(current.DefaultBranch, &branch) {
			payload["default_branch"] = branch
		}
	}
	if managed(config.DefaultClusterID, plan.DefaultClusterID) {
		// The organization rejects anything that is not a UUID, so an empty string clears the
		// default cluster instead of being sent as it is.
		var cluster *string
		if id := plan.DefaultClusterID.ValueString(); id != "" {
			cluster = &id
		}
		if !stringPtrEqual(current.DefaultClusterID, cluster) {
			payload["default_cluster_id"] = cluster
		}
	}
	if managed(config.DefaultTimeoutInMinutes, plan.DefaultTimeoutInMinutes) {
		if timeout := plan.DefaultTimeoutInMinutes.ValueInt64(); !int64PtrEqual(current.DefaultTimeoutInMinutes, &timeout) {
			payload["default_timeout_in_minutes"] = timeout
		}
	}
	if managed(config.MaximumTimeoutInMinutes, plan.MaximumTimeoutInMinutes) {
		if timeout := plan.MaximumTimeoutInMinutes.ValueInt64(); !int64PtrEqual(current.MaximumTimeoutInMinutes, &timeout) {
			payload["maximum_timeout_in_minutes"] = timeout
		}
	}
	if managed(config.ScheduledJobExpiryInMinutes, plan.ScheduledJobExpiryInMinutes) {
		if expiry := plan.ScheduledJobExpiryInMinutes.ValueInt64(); !int64PtrEqual(current.ScheduledJobExpiryInMinutes, &expiry) {
			payload["scheduled_job_expiry_in_minutes"] = expiry
		}
	}

	return payload
}

// managed reports whether an attribute is one this resource writes, rather than one it only reads
// back into state. An attribute left out of the configuration is not managed.
func managed(config, plan attr.Value) bool {
	return !config.IsNull() && !plan.IsUnknown()
}

// clearableStringValue records a setting the API reports as absent, keeping an explicitly
// configured empty string so that clearing the setting does not read back as null.
func clearableStringValue(configured types.String, remote *string) types.String {
	if remote != nil {
		return types.StringValue(*remote)
	}
	if !configured.IsNull() && !configured.IsUnknown() && configured.ValueString() == "" {
		return configured
	}
	return types.StringNull()
}

// optionalInt64Value maps a nullable API response field to the appropriate Terraform type.
func optionalInt64Value(i *int64) types.Int64 {
	if i == nil {
		return types.Int64Null()
	}
	return types.Int64Value(*i)
}

func stringPtrEqual(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func int64PtrEqual(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// pipelineSettingsErrorDetail explains a failed request, naming the access the endpoint wants where
// it was refused. These settings are only reachable by an organization administrator, which is
// easily missed when the token itself carries the right scope.
func pipelineSettingsErrorDetail(what, scope string, err error) string {
	detail := fmt.Sprintf("Unable to %s: %s", what, err.Error())
	if isAPIStatus(err, http.StatusForbidden) {
		detail += fmt.Sprintf(" The API token needs the %s scope, and its user must be an organization administrator.", scope)
	}
	return detail
}

func (c *Client) organizationPipelineSettingsPath(subPath string) string {
	path := fmt.Sprintf("/v2/organizations/%s/pipeline-settings", c.organization)
	if subPath != "" {
		path += "/" + subPath
	}
	return path
}

func (c *Client) getOrganizationPipelineSettings(ctx context.Context) (*organizationPipelineSettings, error) {
	var settings organizationPipelineSettings
	err := c.makeRequest(ctx, http.MethodGet, c.organizationPipelineSettingsPath(""), nil, &settings)
	return &settings, err
}

func (c *Client) updateOrganizationPipelineSettings(ctx context.Context, payload map[string]any) (*organizationPipelineSettings, error) {
	var settings organizationPipelineSettings
	err := c.makeRequest(ctx, http.MethodPatch, c.organizationPipelineSettingsPath(""), payload, &settings)
	return &settings, err
}

// setOrganizationPipelineSetting drives one of the sub-routes, each of which answers with the whole
// settings resource rather than with the setting it changed.
func (c *Client) setOrganizationPipelineSetting(ctx context.Context, method, subPath string, payload any) (*organizationPipelineSettings, error) {
	var settings organizationPipelineSettings
	err := c.makeRequest(ctx, method, c.organizationPipelineSettingsPath(subPath), payload, &settings)
	return &settings, err
}
