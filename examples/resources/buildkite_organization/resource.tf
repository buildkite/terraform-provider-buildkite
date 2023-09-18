# allow api access only from 1.1.1.1
resource "buildkite_organization" "settings" {
  allowed_api_ip_addresses = ["1.1.1.1/32"]
}
