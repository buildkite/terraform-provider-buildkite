package buildkite

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// These four registrations share the same UUID-addressed archive lifecycle. Only
// their create payloads and immutable relationships differ.
type releasesResource struct {
	client *Client
	kind   string
	route  string
}

func newDeploymentArtifactResource() resource.Resource {
	return &releasesResource{kind: "deployment_artifact", route: "artifacts"}
}

func newTargetResource() resource.Resource {
	return &releasesResource{kind: "target", route: "targets"}
}

func newComponentResource() resource.Resource {
	return &releasesResource{kind: "component", route: "components"}
}

func newComponentInstanceResource() resource.Resource {
	return &releasesResource{kind: "component_instance", route: "component-instances"}
}

type releasesRegistration struct {
	UUID       string            `json:"id"`
	ID         string            `json:"graphql_id"`
	Name       string            `json:"name"`
	ArchivedAt *string           `json:"archived_at"`
	Labels     map[string]string `json:"labels"`
	Artifact   struct {
		UUID string `json:"id"`
	} `json:"artifact"`
	TargetUUID    string `json:"target_id"`
	ComponentUUID string `json:"component_id"`
}

type releasesResourceData struct {
	UUID          types.String
	Name          types.String
	AllowRestore  types.Bool
	Labels        types.Map
	ArtifactUUID  types.String
	TargetUUID    types.String
	ComponentUUID types.String
}

func (r *releasesResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.kind
}

func (r *releasesResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData != nil {
		r.client = req.ProviderData.(*Client)
	}
}

func (r *releasesResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The GraphQL ID of the registration.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"uuid": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The UUID of the registration, used by the REST API and for import.",
			PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
		},
		"name": schema.StringAttribute{
			Required:            true,
			MarkdownDescription: "The registration name. Unique within the organization, including archived registrations. Renamed in place.",
			Validators: []validator.String{
				stringvalidator.LengthBetween(1, 253),
				stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`), "must contain lowercase letters, digits, dots or hyphens and start and end with a letter or digit"),
				stringvalidator.NoneOf("by-id", "by-uuid"),
			},
		},
		"allow_unarchive_on_create": schema.BoolAttribute{
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(false),
			MarkdownDescription: "Allow create to restore an archived registration with the same name and matching immutable bindings, retaining its UUID and history. Defaults to `false`. Never adopts an active registration. This flag has no effect on updates or refresh.",
		},
	}
	immutableUUID := func(description string) schema.StringAttribute {
		return schema.StringAttribute{
			Required:            true,
			MarkdownDescription: description,
			Validators: []validator.String{
				stringvalidator.RegexMatches(regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`), "must be a UUID, not a GraphQL ID"),
			},
			PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
		}
	}
	description := ""
	switch r.kind {
	case "deployment_artifact":
		description = "Manages a Deployment Artifact registration, not build job artifacts or artifact publications."
	case "target":
		description = "Manages a Releases Target registration. Archiving a Target also archives its active Instances; restoring the Target does not restore those Instances. Manage each Instance separately."
		attrs["labels"] = schema.MapAttribute{
			Optional:            true,
			Computed:            true,
			ElementType:         types.StringType,
			Default:             mapdefault.StaticValue(types.MapValueMust(types.StringType, map[string]attr.Value{})),
			MarkdownDescription: "The Target's complete label map. Omitted labels default to `{}`. Updates replace the entire map; removing a key removes it remotely. At most 100 entries, with keys up to 64 UTF-8 bytes and values up to 256 UTF-8 bytes.",
		}
	case "component":
		description = "Manages an organization-wide Releases catalog Component with an immutable Deployment Artifact binding. Components are not children of Targets."
		attrs["artifact_uuid"] = immutableUUID("The UUID of the Deployment Artifact. Changing this binding requires replacement.")
	case "component_instance":
		description = "Registers a catalog Component on a Target. The Target and Component bindings are immutable. No deployment or observed version state is managed."
		attrs["target_uuid"] = immutableUUID("The UUID of the Target. Changing this binding requires replacement.")
		attrs["component_uuid"] = immutableUUID("The UUID of the catalog Component. Changing this binding requires replacement.")
		name := attrs["name"].(schema.StringAttribute)
		name.Required = false
		name.Optional = true
		name.Computed = true
		name.MarkdownDescription = "The Instance name, unique within its Target including archived Instances. Defaults to the Component's current name on create. Renamed in place; omitting it later retains the assigned name."
		name.PlanModifiers = []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
		attrs["name"] = name
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: description + "\n\n" +
			"Destroy archives the registration, preserving history and reserving its name. Create rejects existing active or archived names by default. " +
			"For manual recovery, unarchive through the API, then import the UUID. Alternatively, explicitly set `allow_unarchive_on_create = true` to restore a matching archived registration. " +
			"Refresh treats an archived registration as absent; it never unarchives during read. " +
			"Immutable binding changes require replacement, which can conflict with the reserved name; choose a different name or explicitly manage recovery. " +
			"The provider never renames an old registration to force replacement.\n\n" +
			"Requires Releases enabled, `read_deployment_artifacts` and `write_deployment_artifacts` token scopes, and permission to manage organization rules for non-OIDC callers. " +
			"In-place renaming requires the Releases by-ID PATCH API.",
		Attributes: attrs,
	}
}

// Attribute access keeps one lifecycle for four schemas without exposing irrelevant
// fields (such as labels on an Artifact) in the Terraform configuration.
type releasesAttributeReader interface {
	GetAttribute(context.Context, path.Path, interface{}) diag.Diagnostics
}

func (r *releasesResource) data(ctx context.Context, reader releasesAttributeReader) (releasesResourceData, diag.Diagnostics) {
	var data releasesResourceData
	var diags diag.Diagnostics
	fields := map[string]interface{}{
		"uuid":                      &data.UUID,
		"name":                      &data.Name,
		"allow_unarchive_on_create": &data.AllowRestore,
	}
	switch r.kind {
	case "target":
		fields["labels"] = &data.Labels
	case "component":
		fields["artifact_uuid"] = &data.ArtifactUUID
	case "component_instance":
		fields["target_uuid"] = &data.TargetUUID
		fields["component_uuid"] = &data.ComponentUUID
	}
	for name, value := range fields {
		diags.Append(reader.GetAttribute(ctx, path.Root(name), value)...)
	}
	return data, diags
}

func (r *releasesResource) setState(ctx context.Context, state *tfsdk.State, data releasesResourceData, remote releasesRegistration) diag.Diagnostics {
	var diags diag.Diagnostics
	fields := map[string]interface{}{
		"id":                        remote.ID,
		"uuid":                      remote.UUID,
		"name":                      remote.Name,
		"allow_unarchive_on_create": data.AllowRestore.ValueBool(),
	}
	switch r.kind {
	case "target":
		if remote.Labels == nil {
			remote.Labels = map[string]string{}
		}
		fields["labels"] = remote.Labels
	case "component":
		fields["artifact_uuid"] = releasesUUIDValue(data.ArtifactUUID, remote.Artifact.UUID)
	case "component_instance":
		fields["target_uuid"] = releasesUUIDValue(data.TargetUUID, remote.TargetUUID)
		fields["component_uuid"] = releasesUUIDValue(data.ComponentUUID, remote.ComponentUUID)
	}
	for name, value := range fields {
		diags.Append(state.SetAttribute(ctx, path.Root(name), value)...)
	}
	return diags
}

// Preserve configured UUID casing when the API returns the same binding in its
// canonical lowercase form. Import and genuinely different bindings use the API value.
func releasesUUIDValue(current types.String, remote string) types.String {
	if strings.EqualFold(current.ValueString(), remote) {
		return current
	}
	return types.StringValue(remote)
}

func (r *releasesResource) basePath() string {
	return fmt.Sprintf("/v2/organizations/%s/%s", url.PathEscape(r.client.organization), r.route)
}

func (r *releasesResource) idPath(uuid string) string {
	return r.basePath() + "/by-id/" + url.PathEscape(uuid)
}

func (r *releasesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	data, diags := r.data(ctx, req.Plan)
	resp.Diagnostics.Append(diags...)
	timeout, diags := r.client.createTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Resolve the UUID to the current Component name because the create API takes
	// component_name, while the persisted immutable binding is a Component UUID.
	body := map[string]interface{}{"name": data.Name.ValueString()}
	lookupCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if r.kind == "component_instance" {
		var component releasesRegistration
		componentPath := fmt.Sprintf("/v2/organizations/%s/components/by-id/%s", url.PathEscape(r.client.organization), url.PathEscape(data.ComponentUUID.ValueString()))
		if err := r.client.makeRequest(lookupCtx, http.MethodGet, componentPath, nil, &component); err != nil {
			resp.Diagnostics.AddError("Unable to read Component", err.Error())
			return
		}
		body["component_name"] = component.Name
		body["target_id"] = data.TargetUUID.ValueString()
		if data.Name.IsNull() || data.Name.IsUnknown() {
			body["name"] = component.Name
		}
	}
	if r.kind == "component" {
		body["artifact"] = map[string]string{"id": data.ArtifactUUID.ValueString()}
	}
	if r.kind == "target" {
		var labels map[string]string
		resp.Diagnostics.Append(data.Labels.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body["labels"] = labels
	}

	var remote releasesRegistration
	if data.AllowRestore.ValueBool() {
		lookup := r.basePath() + "/" + url.PathEscape(body["name"].(string))
		if r.kind == "component_instance" {
			lookup = fmt.Sprintf("/v2/organizations/%s/targets/by-id/%s/instances/%s", url.PathEscape(r.client.organization), url.PathEscape(data.TargetUUID.ValueString()), url.PathEscape(body["name"].(string)))
		}
		err := r.client.makeRequest(lookupCtx, http.MethodGet, lookup, nil, &remote)
		if err == nil {
			if remote.ArchivedAt == nil {
				resp.Diagnostics.AddError("Registration already exists", "The registration is active. Import its UUID to manage it; allow_unarchive_on_create never adopts active registrations.")
				return
			}
			if (r.kind == "component" && !strings.EqualFold(remote.Artifact.UUID, data.ArtifactUUID.ValueString())) ||
				(r.kind == "component_instance" && (!strings.EqualFold(remote.TargetUUID, data.TargetUUID.ValueString()) || !strings.EqualFold(remote.ComponentUUID, data.ComponentUUID.ValueString()))) {
				resp.Diagnostics.AddError("Archived registration binding mismatch", "The archived registration's immutable Artifact, Component or Target binding does not match the configuration. It has not been restored.")
				return
			}
			requestCtx, done := mutationContext(ctx, timeout)
			err = r.client.makeRequest(requestCtx, http.MethodPost, r.idPath(remote.UUID)+"/unarchive", map[string]interface{}{}, &remote)
			done()
			if err != nil {
				resp.Diagnostics.AddError("Unable to restore registration", err.Error())
				return
			}
			// Record the restored identity before updating labels, so a failed PATCH
			// does not lose the restored registration from state.
			resp.Diagnostics.Append(r.setState(ctx, &resp.State, data, remote)...)
			if r.kind == "target" {
				var updated releasesRegistration
				requestCtx, done = mutationContext(ctx, timeout)
				err = r.client.makeRequest(requestCtx, http.MethodPatch, r.idPath(remote.UUID), map[string]interface{}{"labels": body["labels"]}, &updated)
				done()
				if err != nil {
					resp.Diagnostics.AddError("Unable to update restored Target labels", err.Error())
					return
				}
				resp.Diagnostics.Append(r.setState(ctx, &resp.State, data, updated)...)
			}
			return
		}
		if !isAPIStatus(err, http.StatusNotFound) {
			resp.Diagnostics.AddError("Unable to look up registration for restore", err.Error())
			return
		}
	}

	requestCtx, done := mutationContext(ctx, timeout)
	defer done()
	err := r.client.makeRequest(requestCtx, http.MethodPost, r.basePath(), body, &remote)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create registration", fmt.Sprintf("%s. Existing active and archived names are not adopted. To recover an existing registration, unarchive it through the API if necessary, then import its UUID; or explicitly enable allow_unarchive_on_create for a matching archived registration.", err))
		return
	}
	resp.Diagnostics.Append(r.setState(ctx, &resp.State, data, remote)...)
}

func (r *releasesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	data, diags := r.data(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	timeout, diags := r.client.readTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var remote releasesRegistration
	err := r.client.makeRequest(requestCtx, http.MethodGet, r.idPath(data.UUID.ValueString()), nil, &remote)
	if isAPIStatus(err, http.StatusNotFound) || (err == nil && remote.ArchivedAt != nil) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read registration", err.Error())
		return
	}
	resp.Diagnostics.Append(r.setState(ctx, &resp.State, data, remote)...)
}

func (r *releasesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	data, diags := r.data(ctx, req.Plan)
	resp.Diagnostics.Append(diags...)
	state, diags := r.data(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	timeout, diags := r.client.updateTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	body := map[string]interface{}{}
	if !data.Name.Equal(state.Name) {
		body["name"] = data.Name.ValueString()
	}
	if r.kind == "target" && !data.Labels.Equal(state.Labels) {
		var labels map[string]string
		resp.Diagnostics.Append(data.Labels.ElementsAs(ctx, &labels, false)...)
		if resp.Diagnostics.HasError() {
			return
		}
		body["labels"] = labels
	}
	if len(body) == 0 {
		resp.State.Raw = req.Plan.Raw // Only the local restore opt-in changed.
		return
	}
	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()
	var remote releasesRegistration
	if err := r.client.makeRequest(requestCtx, http.MethodPatch, r.idPath(data.UUID.ValueString()), body, &remote); err != nil {
		resp.Diagnostics.AddError("Unable to update registration", err.Error())
		return
	}
	resp.Diagnostics.Append(r.setState(ctx, &resp.State, data, remote)...)
}

func (r *releasesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	data, diags := r.data(ctx, req.State)
	resp.Diagnostics.Append(diags...)
	timeout, diags := r.client.deleteTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()
	var remote releasesRegistration
	err := r.client.makeRequest(requestCtx, http.MethodPost, r.idPath(data.UUID.ValueString())+"/archive", map[string]interface{}{}, &remote)
	if err != nil && !isAPIStatus(err, http.StatusNotFound) {
		resp.Diagnostics.AddError("Unable to archive registration", err.Error())
	}
}

func (r *releasesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("uuid"), req, resp)
}
