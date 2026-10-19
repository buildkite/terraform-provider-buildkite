package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var workflowMonitorFields = map[string][]string{
	"transition_count":        {"window_size", "alarm_threshold", "recover_threshold"},
	"probabilistic_flakiness": {"alarm_threshold", "recover_threshold"},
	"duration_threshold":      {"aggregation", "evaluation_window", "alarm_threshold_ms", "recover_threshold_ms"},
	"passed_on_retry":         {},
	"new_test":                {},
}

var workflowActionFields = map[string][]string{
	"add_label":                 {"label"},
	"remove_label":              {"label"},
	"change_state":              {"state"},
	"send_webhook_notification": {"endpoint"},
	"send_slack_notification":   {"slack_workspace_id", "channel", "message_template"},
	"create_linear_issue":       {"linear_team_id", "title_template", "description_template"},
}

type testSuiteWorkflowResource struct {
	client *Client
}

type testSuiteWorkflowModel struct {
	ID             types.String `tfsdk:"id"`
	TestSuiteUUID  types.String `tfsdk:"test_suite_uuid"`
	Monitor        types.Object `tfsdk:"monitor"`
	Filters        types.Set    `tfsdk:"filters"`
	AlarmActions   types.Set    `tfsdk:"alarm_actions"`
	RecoverActions types.Set    `tfsdk:"recover_actions"`
	URL            types.String `tfsdk:"url"`
	WebURL         types.String `tfsdk:"web_url"`
}

type testSuiteWorkflowResponse struct {
	ID             string                       `json:"id"`
	Monitor        map[string]json.RawMessage   `json:"monitor"`
	Filters        []map[string]json.RawMessage `json:"filters"`
	AlarmActions   []map[string]json.RawMessage `json:"alarm_actions"`
	RecoverActions []map[string]json.RawMessage `json:"recover_actions"`
	URL            string                       `json:"url"`
	WebURL         string                       `json:"web_url"`
}

var (
	_ resource.ResourceWithConfigure      = (*testSuiteWorkflowResource)(nil)
	_ resource.ResourceWithValidateConfig = (*testSuiteWorkflowResource)(nil)
)

func newTestSuiteWorkflowResource() resource.Resource {
	return &testSuiteWorkflowResource{}
}

func (*testSuiteWorkflowResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_test_suite_workflow"
}

func (r *testSuiteWorkflowResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*Client)
	}
}

func workflowMonitorSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Required:            true,
		MarkdownDescription: "Monitor settings. Supply all fields for the selected type. Recovery thresholds must be below alarm thresholds.",
		PlanModifiers:       []planmodifier.Object{objectplanmodifier.RequiresReplace()},
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "One of `transition_count`, `probabilistic_flakiness`, `duration_threshold`, `passed_on_retry`, or `new_test`. " +
					"`new_test` requires an existing run and cannot have recovery actions.",
				Validators: []validator.String{stringvalidator.OneOf("transition_count", "probabilistic_flakiness", "duration_threshold", "passed_on_retry", "new_test")},
			},
			"window_size": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "Transition-count window size, from 2 to 1000.",
				Validators: []validator.Int64{int64validator.Between(2, 1000)},
			},
			"alarm_threshold": schema.Float64Attribute{
				Optional: true, MarkdownDescription: "Score alarm threshold, from 0 to 1. Minimum 0.025 for probabilistic flakiness. Transition-count scores allow at most three decimal places.",
				Validators: []validator.Float64{float64validator.Between(0, 1)},
			},
			"recover_threshold": schema.Float64Attribute{
				Optional: true, MarkdownDescription: "Score recovery threshold, from 0 to 1. Transition-count scores allow at most three decimal places.",
				Validators: []validator.Float64{float64validator.Between(0, 1)},
			},
			"aggregation": schema.StringAttribute{
				Optional: true, MarkdownDescription: "Duration aggregation. Only `p50` is supported.",
				Validators: []validator.String{stringvalidator.OneOf("p50")},
			},
			"evaluation_window": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "Duration evaluation window, from 1 to 100.",
				Validators: []validator.Int64{int64validator.Between(1, 100)},
			},
			"alarm_threshold_ms": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "Duration alarm threshold in integer milliseconds, from 0 to 3600000.",
				Validators: []validator.Int64{int64validator.Between(0, 3600000)},
			},
			"recover_threshold_ms": schema.Int64Attribute{
				Optional: true, MarkdownDescription: "Duration recovery threshold in integer milliseconds, from 0 to 3600000.",
				Validators: []validator.Int64{int64validator.Between(0, 3600000)},
			},
		},
	}
}

func workflowFiltersSchema() schema.SetNestedAttribute {
	attributes := map[string]schema.Attribute{
		"key": schema.StringAttribute{
			Required: true, MarkdownDescription: "Tag key: at most 64 bytes, starting with a letter. Cannot be `mut` or start with `mut.`.",
			Validators: []validator.String{stringvalidator.RegexMatches(regexp.MustCompile(`^[A-Za-z][\w.\-]*$`), "must start with a letter and contain only letters, digits, underscores, dots, or hyphens")},
		},
		"operator": schema.StringAttribute{
			Required: true, MarkdownDescription: "One of `is`, `is_not`, or `starts_with`.",
			Validators: []validator.String{stringvalidator.OneOf("is", "is_not", "starts_with")},
		},
		"value": schema.StringAttribute{
			Required: true, MarkdownDescription: "Nonempty tag value, at most 128 bytes, with no surrounding whitespace.",
			Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
		},
	}
	return workflowSetSchema(attributes, 4, "Tag filters. Defaults to empty. `transition_count` requires a nonblank `scm.branch` filter; no default-branch filter is added.")
}

func workflowActionsSchema() schema.SetNestedAttribute {
	attributes := map[string]schema.Attribute{
		"type": schema.StringAttribute{
			Required: true, MarkdownDescription: "One of `add_label`, `remove_label`, `change_state`, `send_webhook_notification`, `send_slack_notification`, or `create_linear_issue`.",
			Validators: []validator.String{stringvalidator.OneOf("add_label", "remove_label", "change_state", "send_webhook_notification", "send_slack_notification", "create_linear_issue")},
		},
		"label": schema.StringAttribute{
			Optional: true, MarkdownDescription: "Normalized label name for label actions. Missing labels are created by the API.",
			Validators: []validator.String{stringvalidator.LengthBetween(1, 25)},
		},
		"state": schema.StringAttribute{
			Optional: true, MarkdownDescription: "Test state for `change_state`. Requires state management on the suite.",
			Validators: []validator.String{stringvalidator.OneOf("enabled", "muted", "skipped")},
		},
		"endpoint": schema.StringAttribute{
			Optional: true, Sensitive: true, MarkdownDescription: "Webhook HTTP(S) URL, at most 2048 characters.",
			Validators: []validator.String{stringvalidator.LengthBetween(1, 2048)},
		},
		"slack_workspace_id": schema.StringAttribute{
			Optional: true, MarkdownDescription: "Slack notification service UUID from the notification services API (`read_notification_services` scope), not a native Slack ID.",
			Validators: []validator.String{stringvalidator.RegexMatches(notificationServiceUUIDRegex, "must be a UUID")},
		},
		"channel": schema.StringAttribute{
			Optional: true, Sensitive: true, MarkdownDescription: "Slack channel.",
			Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
		},
		"message_template": schema.StringAttribute{
			Optional: true, Sensitive: true, MarkdownDescription: "Slack Liquid message template, maximum 1000 characters.",
			Validators: []validator.String{stringvalidator.LengthBetween(1, 1000)},
		},
		"linear_team_id": schema.StringAttribute{
			Optional: true, MarkdownDescription: "Team UUID from Linear, accessible to the organization's integration.",
			Validators: []validator.String{stringvalidator.RegexMatches(notificationServiceUUIDRegex, "must be a UUID")},
		},
		"title_template": schema.StringAttribute{
			Optional: true, Sensitive: true, MarkdownDescription: "Linear issue title Liquid template.",
			Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
		},
		"description_template": schema.StringAttribute{
			Optional: true, Sensitive: true, MarkdownDescription: "Linear issue description Liquid template.",
			Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
		},
	}
	return workflowSetSchema(attributes, 7, "Actions. Defaults to empty. Supply only the fields required by each action type.")
}

func workflowSetSchema(attributes map[string]schema.Attribute, limit int, description string) schema.SetNestedAttribute {
	nested := schema.NestedAttributeObject{Attributes: attributes}
	return schema.SetNestedAttribute{
		Optional: true, Computed: true,
		MarkdownDescription: fmt.Sprintf("%s Maximum %d entries.", description, limit),
		NestedObject:        nested,
		Default:             setdefault.StaticValue(types.SetValueMust(nested.Type(), []attr.Value{})),
		Validators:          []validator.Set{setvalidator.SizeAtMost(limit)},
		PlanModifiers:       []planmodifier.Set{setplanmodifier.RequiresReplace()},
	}
}

func (*testSuiteWorkflowResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a Test Engine workflow. Requires the workflows API, `read_suites` and `write_suites` scopes, and suite-edit permission. " +
			"Available monitors and actions depend on the organization's plan and integrations.\n\n" +
			"Configuration changes replace the workflow. Import is not supported.\n\n" +
			"Sensitive values remain in Terraform state. Refresh fails if webhook endpoints are masked.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true, MarkdownDescription: "The workflow UUID.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"test_suite_uuid": schema.StringAttribute{
				Required: true, MarkdownDescription: "Suite UUID, such as `buildkite_test_suite.example.uuid`, not its GraphQL ID or slug.",
				Validators:    []validator.String{stringvalidator.RegexMatches(notificationServiceUUIDRegex, "must be a UUID")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"monitor":         workflowMonitorSchema(),
			"filters":         workflowFiltersSchema(),
			"alarm_actions":   workflowActionsSchema(),
			"recover_actions": workflowActionsSchema(),
			"url":             schema.StringAttribute{Computed: true, MarkdownDescription: "The workflow's REST API URL."},
			"web_url":         schema.StringAttribute{Computed: true, MarkdownDescription: "The workflow's browser URL."},
		},
	}
}

// Validate only relationships known at plan time. Entitlements and integration availability are
// checked by the API, whose validation errors include pointers to the offending fields.
func (*testSuiteWorkflowResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config testSuiteWorkflowModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateWorkflowFields(config.Monitor, workflowMonitorFields, path.Root("monitor"), &resp.Diagnostics)
	for name, actions := range map[string]types.Set{"alarm_actions": config.AlarmActions, "recover_actions": config.RecoverActions} {
		for _, action := range actions.Elements() {
			if action.IsNull() {
				resp.Diagnostics.AddAttributeError(path.Root(name), "Invalid workflow action", "Actions cannot contain null elements.")
				continue
			}
			validateWorkflowFields(action.(types.Object), workflowActionFields, path.Root(name), &resp.Diagnostics)
		}
	}
	branchFound, branchUnknown := false, config.Filters.IsUnknown()
	for _, filter := range config.Filters.Elements() {
		if filter.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("filters"), "Invalid workflow filter", "Filters cannot contain null elements.")
			continue
		}
		if filter.IsUnknown() {
			branchUnknown = true
			continue
		}
		fields := filter.(types.Object).Attributes()
		key, value := fields["key"].(types.String), fields["value"].(types.String)
		if key.IsUnknown() || value.IsUnknown() {
			branchUnknown = true
		}
		if key.ValueString() == "scm.branch" && strings.TrimSpace(value.ValueString()) != "" {
			branchFound = true
		}
		if len(key.ValueString()) > 64 || len(value.ValueString()) > 128 ||
			key.ValueString() == "mut" || strings.HasPrefix(key.ValueString(), "mut.") ||
			strings.TrimSpace(value.ValueString()) != value.ValueString() {
			resp.Diagnostics.AddAttributeError(path.Root("filters"), "Invalid workflow filter", "Keys must be at most 64 bytes and cannot be mutable tags. Values must be at most 128 bytes and cannot have leading or trailing whitespace.")
		}
	}
	if config.Monitor.IsNull() || config.Monitor.IsUnknown() {
		return
	}
	monitor := config.Monitor.Attributes()
	typ := monitor["type"].(types.String).ValueString()
	if typ == "transition_count" && !branchFound && !branchUnknown {
		resp.Diagnostics.AddAttributeError(path.Root("filters"), "Missing branch filter", "transition_count requires a nonblank scm.branch filter.")
	}
	if typ == "new_test" && len(config.RecoverActions.Elements()) > 0 {
		resp.Diagnostics.AddAttributeError(path.Root("recover_actions"), "Unsupported recovery actions", "new_test does not support recovery actions.")
	}
	if typ == "duration_threshold" {
		alarm, recover := monitor["alarm_threshold_ms"].(types.Int64), monitor["recover_threshold_ms"].(types.Int64)
		if !alarm.IsNull() && !alarm.IsUnknown() && !recover.IsNull() && !recover.IsUnknown() && recover.ValueInt64() >= alarm.ValueInt64() {
			resp.Diagnostics.AddAttributeError(path.Root("monitor"), "Invalid recovery threshold", "recover_threshold_ms must be less than alarm_threshold_ms.")
		}
	}
	if typ == "transition_count" || typ == "probabilistic_flakiness" {
		alarm, recover := monitor["alarm_threshold"].(types.Float64), monitor["recover_threshold"].(types.Float64)
		if !alarm.IsNull() && !alarm.IsUnknown() && !recover.IsNull() && !recover.IsUnknown() && recover.ValueFloat64() >= alarm.ValueFloat64() {
			resp.Diagnostics.AddAttributeError(path.Root("monitor"), "Invalid recovery threshold", "recover_threshold must be less than alarm_threshold.")
		}
		if typ == "probabilistic_flakiness" && !alarm.IsNull() && !alarm.IsUnknown() && alarm.ValueFloat64() < 0.025 {
			resp.Diagnostics.AddAttributeError(path.Root("monitor"), "Invalid alarm threshold", "probabilistic_flakiness requires alarm_threshold of at least 0.025.")
		}
		if typ == "transition_count" {
			for _, threshold := range []types.Float64{alarm, recover} {
				if !threshold.IsNull() && !threshold.IsUnknown() && math.Abs(threshold.ValueFloat64()*1000-math.Round(threshold.ValueFloat64()*1000)) > 1e-9 {
					resp.Diagnostics.AddAttributeError(path.Root("monitor"), "Invalid threshold precision", "transition_count thresholds allow at most three decimal places.")
				}
			}
		}
	}
}

func validateWorkflowFields(object types.Object, fieldsByType map[string][]string, p path.Path, diags *diag.Diagnostics) {
	if object.IsNull() || object.IsUnknown() {
		return
	}
	attributes := object.Attributes()
	typ := attributes["type"].(types.String)
	if typ.IsNull() || typ.IsUnknown() {
		return
	}
	fields, ok := fieldsByType[typ.ValueString()]
	if !ok {
		return // The enum validator reports unsupported types.
	}
	for name, value := range attributes {
		if name == "type" {
			continue
		}
		required := slices.Contains(fields, name)
		if required && value.IsNull() {
			diags.AddAttributeError(p, "Missing workflow setting", fmt.Sprintf("%s requires %s.", typ.ValueString(), name))
		} else if !required && !value.IsNull() {
			diags.AddAttributeError(p, "Unsupported workflow setting", fmt.Sprintf("%s does not support %s.", typ.ValueString(), name))
		}
	}
}

func (r *testSuiteWorkflowResource) workflowPath(state testSuiteWorkflowModel) string {
	p := fmt.Sprintf("/v2/analytics/organizations/%s/suites/%s/workflows", r.client.organization, state.TestSuiteUUID.ValueString())
	if !state.ID.IsNull() && !state.ID.IsUnknown() {
		p += "/" + state.ID.ValueString()
	}
	return p
}

func (r *testSuiteWorkflowResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state testSuiteWorkflowModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := r.client.createTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()
	payload := map[string]any{
		"monitor":         workflowObjectPayload(state.Monitor),
		"filters":         workflowSetPayload(state.Filters),
		"alarm_actions":   workflowSetPayload(state.AlarmActions),
		"recover_actions": workflowSetPayload(state.RecoverActions),
	}
	var result testSuiteWorkflowResponse
	if err := r.client.makeRequest(requestCtx, http.MethodPost, r.workflowPath(state), payload, &result); err != nil {
		resp.Diagnostics.AddError("Failed to create test suite workflow", err.Error())
		return
	}
	state.ID = types.StringValue(result.ID)
	// Keep the successfully created resource tracked even if response conversion fails.
	state.URL, state.WebURL = types.StringValue(result.URL), types.StringValue(result.WebURL)
	defer func() { resp.Diagnostics.Append(resp.State.Set(ctx, &state)...) }()
	resp.Diagnostics.Append(state.applyWorkflowResponse(result)...)
}

func (r *testSuiteWorkflowResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state testSuiteWorkflowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := r.client.readTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var result testSuiteWorkflowResponse
	err := r.client.makeRequest(requestCtx, http.MethodGet, r.workflowPath(state), nil, &result)
	if isAPIStatus(err, http.StatusNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to read test suite workflow", err.Error())
		return
	}
	resp.Diagnostics.Append(state.applyWorkflowResponse(result)...)
	if !resp.Diagnostics.HasError() {
		resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
	}
}

func (r *testSuiteWorkflowResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state testSuiteWorkflowModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := r.client.deleteTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()
	err := r.client.makeRequest(requestCtx, http.MethodDelete, r.workflowPath(state), nil, nil)
	if err != nil && !isAPIStatus(err, http.StatusNotFound) {
		resp.Diagnostics.AddError("Failed to delete test suite workflow", err.Error())
	}
}

func (*testSuiteWorkflowResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Workflow update is not supported", "Changes to workflow configuration must replace the resource.")
}

func workflowObjectPayload(object types.Object) map[string]any {
	payload := map[string]any{}
	for name, value := range object.Attributes() {
		if value.IsNull() || value.IsUnknown() {
			continue
		}
		switch v := value.(type) {
		case types.String:
			payload[name] = v.ValueString()
		case types.Int64:
			payload[name] = v.ValueInt64()
		case types.Float64:
			payload[name] = v.ValueFloat64()
		}
	}
	return payload
}

func workflowSetPayload(set types.Set) []map[string]any {
	payload := make([]map[string]any, 0, len(set.Elements()))
	for _, value := range set.Elements() {
		payload = append(payload, workflowObjectPayload(value.(types.Object)))
	}
	return payload
}

func (state *testSuiteWorkflowModel) applyWorkflowResponse(result testSuiteWorkflowResponse) diag.Diagnostics {
	var diags diag.Diagnostics
	monitor, err := workflowObjectFromAPI(result.Monitor, workflowMonitorSchema().GetType().(types.ObjectType), workflowMonitorFields)
	if err != nil {
		diags.AddError("Failed to read workflow monitor", err.Error())
		return diags
	}
	filters, err := workflowSetFromAPI(result.Filters, workflowFiltersSchema().NestedObject.Type().(types.ObjectType), nil)
	if err != nil {
		diags.AddError("Failed to read workflow filters", err.Error())
		return diags
	}
	actionsType := workflowActionsSchema().NestedObject.Type().(types.ObjectType)
	alarm, err := workflowSetFromAPI(result.AlarmActions, actionsType, workflowActionFields)
	if err != nil {
		diags.AddError("Failed to read workflow alarm actions", err.Error())
		return diags
	}
	recover, err := workflowSetFromAPI(result.RecoverActions, actionsType, workflowActionFields)
	if err != nil {
		diags.AddError("Failed to read workflow recovery actions", err.Error())
		return diags
	}
	state.Monitor, state.Filters, state.AlarmActions, state.RecoverActions = monitor, filters, alarm, recover
	state.URL, state.WebURL = types.StringValue(result.URL), types.StringValue(result.WebURL)
	return diags
}

func workflowObjectFromAPI(data map[string]json.RawMessage, typ types.ObjectType, fieldsByType map[string][]string) (types.Object, error) {
	if fieldsByType != nil {
		var name string
		if err := json.Unmarshal(data["type"], &name); err != nil {
			return types.Object{}, fmt.Errorf("missing or invalid workflow type")
		}
		if _, ok := fieldsByType[name]; !ok {
			return types.Object{}, fmt.Errorf("unsupported workflow type %q; upgrade the provider to manage it", name)
		}
	}
	values := make(map[string]attr.Value, len(typ.AttrTypes))
	for name, fieldType := range typ.AttrTypes {
		raw, present := data[name]
		var err error
		switch fieldType {
		case types.StringType:
			value := types.StringNull()
			if present && string(raw) != "null" {
				var s string
				err = json.Unmarshal(raw, &s)
				if name == "endpoint" && strings.Contains(s, "[REDACTED]") {
					return types.Object{}, fmt.Errorf("the API returned a masked webhook endpoint; use a token whose user can edit the suite to refresh this workflow")
				}
				value = types.StringValue(s)
			}
			values[name] = value
		case types.Int64Type:
			value := types.Int64Null()
			if present && string(raw) != "null" {
				var n int64
				err = json.Unmarshal(raw, &n)
				value = types.Int64Value(n)
			}
			values[name] = value
		case types.Float64Type:
			value := types.Float64Null()
			if present && string(raw) != "null" {
				var n float64
				err = json.Unmarshal(raw, &n)
				value = types.Float64Value(n)
			}
			values[name] = value
		}
		if err != nil {
			return types.Object{}, fmt.Errorf("invalid API value for workflow field %s", name)
		}
	}
	object, diags := types.ObjectValue(typ.AttrTypes, values)
	if diags.HasError() {
		return types.Object{}, fmt.Errorf("could not decode workflow object")
	}
	return object, nil
}

func workflowSetFromAPI(data []map[string]json.RawMessage, typ types.ObjectType, fieldsByType map[string][]string) (types.Set, error) {
	values := make([]attr.Value, 0, len(data))
	for _, item := range data {
		object, err := workflowObjectFromAPI(item, typ, fieldsByType)
		if err != nil {
			return types.Set{}, err
		}
		if !slices.ContainsFunc(values, func(v attr.Value) bool { return v.Equal(object) }) {
			values = append(values, object)
		}
	}
	set, diags := types.SetValue(typ, values)
	if diags.HasError() {
		return types.Set{}, fmt.Errorf("could not decode workflow set")
	}
	return set, nil
}
