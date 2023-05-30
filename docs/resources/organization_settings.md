# Resource: organization_settings

This resource allows you to manage the settings for an organization.

You must be an organization administrator to manage organization settings.

Note: The "Allowed API IP Addresses" feature must be enabled on your organization in order to manage the `allowed_api_ip_addresses` attribute.

## Example Usage

```hcl
resource "buildkite_organization_settings" "test_settings" {
  allowed_api_ip_addresses = ["1.1.1.1/32"]
}
```

## Argument Reference

- `allowed_api_ip_addresses` - (Optional) A list of IP addresses in CIDR format that are allowed to access the Buildkite API. If not set, all IP addresses are allowed (the same as setting 0.0.0.0/0).

## Import

Organization settings can be imported by passing the organization slug to the import command, along with the identifier of the resource.

```
$ terraform import buildkite_organization_settings.test_settings test_org
```
