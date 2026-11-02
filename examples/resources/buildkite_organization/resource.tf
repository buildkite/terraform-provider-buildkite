# allow api access only from 1.1.1.1 and enforce 2fa for all members
resource "buildkite_organization" "settings" {
  allowed_api_ip_addresses = ["1.1.1.1/32"]
  enforce_2fa              = true

  # Alternatively, replace allowed_api_ip_addresses with entries to attach descriptions:
  # allowed_api_ip_entries = [
  #   { ip_address = "1.1.1.1/32", description = "Office" },
  #   { ip_address = "2001:db8::/48" }, # omitted description defaults to ""
  # ]

  # only let administrators create API access tokens and revoke tokens unused for 90 days
  restrict_user_api_token_creation = true
  revoke_inactive_tokens_after     = "DAYS_90"
}
