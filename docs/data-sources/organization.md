---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "buildkite_organization Data Source - terraform-provider-buildkite"
subcategory: ""
description: |-
  Use this data source to look up the organization settings.
---

# buildkite_organization (Data Source)

Use this data source to look up the organization settings.



<!-- schema generated by tfplugindocs -->
## Schema

### Read-Only

- `allowed_api_ip_addresses` (List of String) List of IP addresses in CIDR format that are allowed to access the Buildkite API for this organization.
- `id` (String) The GraphQL ID of the organization.
- `uuid` (String) The UUID of the organization.
