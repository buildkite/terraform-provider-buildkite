package buildkite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MakeNowJust/heredoc"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type workloadIdentityResource struct {
	client *Client
}

type workloadIdentityResourceModel struct {
	UUID                            types.String                 `tfsdk:"uuid"`
	ClientID                        types.String                 `tfsdk:"client_id"`
	Name                            types.String                 `tfsdk:"name"`
	Description                     types.String                 `tfsdk:"description"`
	Enabled                         types.Bool                   `tfsdk:"enabled"`
	OIDCIssuer                      types.String                 `tfsdk:"oidc_issuer"`
	OIDCAudience                    types.String                 `tfsdk:"oidc_audience"`
	OIDCSubject                     types.String                 `tfsdk:"oidc_subject"`
	OIDCClaims                      jsontypes.Normalized         `tfsdk:"oidc_claims"`
	OIDCClaimConditions             types.String                 `tfsdk:"oidc_claim_conditions"`
	GrantableTokenScopes            []string                     `tfsdk:"grantable_token_scopes"`
	DefaultTokenScopes              []string                     `tfsdk:"default_token_scopes"`
	MaxTokenTTLSeconds              types.Int64                  `tfsdk:"max_token_ttl_seconds"`
	AllowedIPAddresses              types.String                 `tfsdk:"allowed_ip_addresses"`
	AllowedTokenExchangeIPAddresses types.String                 `tfsdk:"allowed_token_exchange_ip_addresses"`
	ResourceGrants                  []workloadIdentityGrantModel `tfsdk:"resource_grants"`
	JWKS                            types.String                 `tfsdk:"jwks"`
}

type workloadIdentityGrantModel struct {
	Type   types.String `tfsdk:"type"`
	Access types.String `tfsdk:"access"`
	IDs    []string     `tfsdk:"ids"`
}

// workloadIdentityAttributes is the request body for creating and updating a workload identity. The
// API rejects unknown fields, so it carries only the writable ones. Every field is sent, with null
// clearing an optional string, so an update leaves nothing behind that the configuration removed.
type workloadIdentityAttributes struct {
	Name                            string                 `json:"name"`
	Description                     string                 `json:"description"`
	Enabled                         bool                   `json:"enabled"`
	OIDCIssuer                      string                 `json:"oidc_issuer"`
	OIDCAudience                    *string                `json:"oidc_audience"`
	OIDCSubject                     string                 `json:"oidc_subject"`
	OIDCClaims                      json.RawMessage        `json:"oidc_claims"`
	OIDCClaimConditions             *string                `json:"oidc_claim_conditions"`
	GrantableTokenScopes            []string               `json:"grantable_token_scopes"`
	DefaultTokenScopes              []string               `json:"default_token_scopes"`
	MaxTokenTTLSeconds              int64                  `json:"max_token_ttl_seconds"`
	AllowedIPAddresses              *string                `json:"allowed_ip_addresses"`
	AllowedTokenExchangeIPAddresses *string                `json:"allowed_token_exchange_ip_addresses"`
	ResourceGrants                  workloadIdentityGrants `json:"resource_grants"`
	JWKS                            *string                `json:"jwks"`
}

// workloadIdentity is a workload identity as the REST API returns it.
type workloadIdentity struct {
	UUID     string `json:"uuid"`
	ClientID string `json:"client_id"`
	workloadIdentityAttributes
}

type workloadIdentityGrant struct {
	Type   string   `json:"type"`
	Access string   `json:"access"`
	IDs    []string `json:"ids,omitempty"`
}

type workloadIdentityGrants []workloadIdentityGrant

// UnmarshalJSON reads identities created before resource grants were added, which hold an empty
// JSON object rather than an array. Both grant nothing.
func (g *workloadIdentityGrants) UnmarshalJSON(data []byte) error {
	var legacy map[string]json.RawMessage
	if json.Unmarshal(data, &legacy) == nil && len(legacy) == 0 {
		*g = nil
		return nil
	}

	return json.Unmarshal(data, (*[]workloadIdentityGrant)(g))
}

func newWorkloadIdentityResource() resource.Resource {
	return &workloadIdentityResource{}
}

func (r *workloadIdentityResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_workload_identity"
}

func (r *workloadIdentityResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = req.ProviderData.(*Client)
}

func (r *workloadIdentityResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	optionalString := func(description string) schema.StringAttribute {
		return schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: description,
			// An empty string is read back as unset, so it would be recorded as null.
			Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
		}
	}

	resp.Schema = schema.Schema{
		MarkdownDescription: heredoc.Doc(`
			A workload identity lets a workload, such as a job running on a Buildkite agent, call the Buildkite REST API
			without a stored API access token. The workload exchanges an OIDC token from its identity provider for a
			short-lived API access token, using the workload identity's ` + "`client_id`" + `.

			Buildkite only mints the access token when the OIDC token matches the workload identity's issuer, audience,
			subject, claims, and claim conditions. The access token's scopes are limited to ` + "`grantable_token_scopes`" + `,
			and the pipelines it can reach to ` + "`resource_grants`" + `. Changes to these only apply to access tokens minted afterwards.
			Learn more in [Workload identity federation](https://buildkite.com/docs/apis/workload-identity-federation).

			The API token's user must be an organization administrator, and the token needs the
			` + "`read_workload_identities`, `write_workload_identities`, and `delete_workload_identities`" + ` scopes.
		`),
		Attributes: map[string]schema.Attribute{
			"uuid": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The UUID of the workload identity.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"client_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The Client ID of the workload identity.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The name of the workload identity.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1), stringvalidator.UTF8LengthAtMost(255)},
			},
			"description": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "A description of the workload identity.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"enabled": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				MarkdownDescription: "Whether workloads can exchange OIDC tokens for access tokens. Disabling the identity revokes its access tokens. Defaults to `true`.",
			},
			"oidc_issuer": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The issuer (`iss`) that OIDC tokens must have. For example, `https://agent.buildkite.com` for Buildkite jobs.",
			},
			"oidc_audience": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "The audience (`aud`) that OIDC tokens must include.",
				Validators:          []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"oidc_subject": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "The pattern that the OIDC token's subject (`sub`) must match, where `*` matches any characters. " +
					"For example, `organization:acme-inc:pipeline:deploy:ref:refs/heads/main:commit:*:step:*` matches every job in builds of the `deploy` pipeline's `main` branch. " +
					"For better security, restrict the subject as much as possible: a broad subject lets workloads that you didn't intend mint API access tokens for your organization.",
			},
			"oidc_claims": schema.StringAttribute{
				Optional:   true,
				CustomType: jsontypes.NormalizedType{},
				MarkdownDescription: "Top-level claims that OIDC tokens must have, as a JSON object of each claim and its exact value. " +
					"For example, `jsonencode({ organization_id = \"<organization UUID>\" })`. Values must match their JSON type too, so `\"42\"` doesn't match `42`. " +
					"For tokens from Buildkite jobs, include an `organization_id` or `organization_slug` claim. Up to 10 claims are allowed.",
			},
			"oidc_claim_conditions": optionalString("A [CEL](https://cel.dev) expression that OIDC tokens must also satisfy. Use it for more complex checks, such as a list of allowed values or a branch prefix. " +
				"For example, `claims.build_branch == \"main\" || claims.build_branch.startsWith(\"release/\")`. " +
				"Learn more in [Write claim conditions](https://buildkite.com/docs/apis/workload-identity-federation#write-claim-conditions)."),
			"grantable_token_scopes": schema.SetAttribute{
				Required:            true,
				ElementType:         types.StringType,
				MarkdownDescription: "The API scopes that access tokens from this identity can have, such as `read_builds` and `write_builds`. A workload can request a subset of them. Only some scopes are available to workload identities; `graphql` is not.",
				Validators:          []validator.Set{setvalidator.SizeAtLeast(1)},
			},
			"default_token_scopes": schema.SetAttribute{
				Optional:            true,
				Computed:            true,
				ElementType:         types.StringType,
				Default:             setdefault.StaticValue(types.SetValueMust(types.StringType, []attr.Value{})),
				MarkdownDescription: "The scopes that an access token has when its workload requests none. Must be a subset of `grantable_token_scopes`. Defaults to none, so workloads must request scopes.",
			},
			"max_token_ttl_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(3600),
				MarkdownDescription: "The longest lifetime, in seconds, of access tokens from this identity. An access token never outlives the OIDC token it was exchanged for. Defaults to `3600`.",
			},
			"allowed_ip_addresses":                optionalString("Space-separated IP addresses in CIDR notation that access tokens from this identity can be used from. If not set, they can be used from any address."),
			"allowed_token_exchange_ip_addresses": optionalString("Space-separated IP addresses in CIDR notation that workloads can exchange OIDC tokens from. If not set, workloads can exchange tokens from any address."),
			"resource_grants": schema.ListNestedAttribute{
				Optional: true,
				Computed: true,
				Default: listdefault.StaticValue(types.ListValueMust(
					types.ObjectType{AttrTypes: map[string]attr.Type{
						"type":   types.StringType,
						"access": types.StringType,
						"ids":    types.SetType{ElemType: types.StringType},
					}},
					[]attr.Value{},
				)),
				MarkdownDescription: "The resources that access tokens from this identity can access, with at most one grant for each type. Defaults to none, so access tokens can't access any pipeline.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "The type of resource to grant. Only `pipeline` is supported.",
						},
						"access": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Which resources to grant: `all`, including those created later; `selected`, the resources in `ids`; or `none`.",
							Validators:          []validator.String{stringvalidator.OneOf("all", "selected", "none")},
						},
						"ids": schema.SetAttribute{
							Optional:            true,
							ElementType:         types.StringType,
							MarkdownDescription: "The UUIDs of the resources to grant. Required when `access` is `selected`, and not allowed otherwise. Up to 20 pipelines can be selected.",
							Validators:          []validator.Set{setvalidator.SizeAtLeast(1)},
						},
					},
				},
			},
			"jwks": optionalString("If the issuer doesn't support OIDC discovery, the JSON Web Key Set (JWKS) to verify OIDC tokens with, as an HTTPS URL or inline JWKS JSON. " +
				"If not set, Buildkite discovers the keys from the issuer."),
		},
	}
}

func (r *workloadIdentityResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan workloadIdentityResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
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

	var identity workloadIdentity
	err := r.client.makeRequest(requestCtx, http.MethodPost, r.client.workloadIdentityPath(""), workloadIdentityRequest(&plan), &identity)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create workload identity", fmt.Sprintf("Unable to create workload identity: %s", err))
		return
	}

	setWorkloadIdentityModel(&plan, &identity)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *workloadIdentityResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state workloadIdentityResourceModel
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

	var identity workloadIdentity
	err := r.client.makeRequest(requestCtx, http.MethodGet, r.client.workloadIdentityPath(state.UUID.ValueString()), nil, &identity)
	if err != nil {
		if isAPIStatus(err, http.StatusNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read workload identity", fmt.Sprintf("Unable to read workload identity: %s", err))
		return
	}

	setWorkloadIdentityModel(&state, &identity)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *workloadIdentityResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan workloadIdentityResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	timeout, diags := r.client.updateTimeout(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	requestCtx, cancel := mutationContext(ctx, timeout)
	defer cancel()

	var identity workloadIdentity
	err := r.client.makeRequest(requestCtx, http.MethodPatch, r.client.workloadIdentityPath(plan.UUID.ValueString()), workloadIdentityRequest(&plan), &identity)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update workload identity", fmt.Sprintf("Unable to update workload identity: %s", err))
		return
	}

	setWorkloadIdentityModel(&plan, &identity)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *workloadIdentityResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state workloadIdentityResourceModel
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

	err := r.client.makeRequest(requestCtx, http.MethodDelete, r.client.workloadIdentityPath(state.UUID.ValueString()), nil, nil)
	if err != nil && !isAPIStatus(err, http.StatusNotFound) {
		resp.Diagnostics.AddError("Unable to delete workload identity", fmt.Sprintf("Unable to delete workload identity: %s", err))
	}
}

func (r *workloadIdentityResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("uuid"), req, resp)
}

func workloadIdentityRequest(plan *workloadIdentityResourceModel) workloadIdentityAttributes {
	// The API requires an object, and an empty one is how it stores an identity without claims.
	claims := json.RawMessage("{}")
	if !plan.OIDCClaims.IsNull() {
		claims = json.RawMessage(plan.OIDCClaims.ValueString())
	}

	grants := make(workloadIdentityGrants, 0, len(plan.ResourceGrants))
	for _, grant := range plan.ResourceGrants {
		grants = append(grants, workloadIdentityGrant{Type: grant.Type.ValueString(), Access: grant.Access.ValueString(), IDs: grant.IDs})
	}

	return workloadIdentityAttributes{
		Name:                            plan.Name.ValueString(),
		Description:                     plan.Description.ValueString(),
		Enabled:                         plan.Enabled.ValueBool(),
		OIDCIssuer:                      plan.OIDCIssuer.ValueString(),
		OIDCAudience:                    plan.OIDCAudience.ValueStringPointer(),
		OIDCSubject:                     plan.OIDCSubject.ValueString(),
		OIDCClaims:                      claims,
		OIDCClaimConditions:             plan.OIDCClaimConditions.ValueStringPointer(),
		GrantableTokenScopes:            plan.GrantableTokenScopes,
		DefaultTokenScopes:              nonNil(plan.DefaultTokenScopes),
		MaxTokenTTLSeconds:              plan.MaxTokenTTLSeconds.ValueInt64(),
		AllowedIPAddresses:              plan.AllowedIPAddresses.ValueStringPointer(),
		AllowedTokenExchangeIPAddresses: plan.AllowedTokenExchangeIPAddresses.ValueStringPointer(),
		ResourceGrants:                  grants,
		JWKS:                            plan.JWKS.ValueStringPointer(),
	}
}

func setWorkloadIdentityModel(model *workloadIdentityResourceModel, identity *workloadIdentity) {
	model.UUID = types.StringValue(identity.UUID)
	model.ClientID = types.StringValue(identity.ClientID)
	model.Name = types.StringValue(identity.Name)
	model.Description = types.StringValue(identity.Description)
	model.Enabled = types.BoolValue(identity.Enabled)
	model.OIDCIssuer = types.StringValue(identity.OIDCIssuer)
	model.OIDCAudience = nonEmptyStringValue(identity.OIDCAudience)
	model.OIDCSubject = types.StringValue(identity.OIDCSubject)
	model.OIDCClaims = workloadIdentityClaims(model.OIDCClaims, identity.OIDCClaims)
	model.OIDCClaimConditions = nonEmptyStringValue(identity.OIDCClaimConditions)
	model.JWKS = nonEmptyStringValue(identity.JWKS)
	model.GrantableTokenScopes = nonNil(identity.GrantableTokenScopes)
	model.DefaultTokenScopes = nonNil(identity.DefaultTokenScopes)
	model.MaxTokenTTLSeconds = types.Int64Value(identity.MaxTokenTTLSeconds)
	model.AllowedIPAddresses = nonEmptyStringValue(identity.AllowedIPAddresses)
	model.AllowedTokenExchangeIPAddresses = nonEmptyStringValue(identity.AllowedTokenExchangeIPAddresses)

	model.ResourceGrants = make([]workloadIdentityGrantModel, 0, len(identity.ResourceGrants))
	for _, grant := range identity.ResourceGrants {
		model.ResourceGrants = append(model.ResourceGrants, workloadIdentityGrantModel{
			Type:   types.StringValue(grant.Type),
			Access: types.StringValue(grant.Access),
			IDs:    grant.IDs,
		})
	}
}

// workloadIdentityClaims returns the claims to record for an identity. The API returns an identity
// without claims as an empty object, or as null if it has not been saved since claims were added,
// and both mean the same as leaving oidc_claims unset. They are recorded as null unless current is
// set, so that a configured empty object does not plan a change on every refresh.
func workloadIdentityClaims(current jsontypes.Normalized, remote json.RawMessage) jsontypes.Normalized {
	var claims map[string]json.RawMessage
	if len(remote) == 0 || (json.Unmarshal(remote, &claims) == nil && len(claims) == 0) {
		if current.IsNull() {
			return jsontypes.NewNormalizedNull()
		}
		return jsontypes.NewNormalizedValue("{}")
	}

	return jsontypes.NewNormalizedValue(string(remote))
}

func nonEmptyStringValue(value *string) types.String {
	if value == nil || *value == "" {
		return types.StringNull()
	}
	return types.StringValue(*value)
}

// nonNil makes an empty list a JSON array and a known empty set in state, rather than null.
func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func (c *Client) workloadIdentityPath(uuid string) string {
	path := fmt.Sprintf("/v2/organizations/%s/workload-identities", c.organization)
	if uuid != "" {
		path += "/" + uuid
	}
	return path
}
