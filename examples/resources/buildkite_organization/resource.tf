# allow api access only from 1.1.1.1, enforce 2fa for all members, keep API token creation to
# administrators and revoke tokens that go unused for 90 days
resource "buildkite_organization" "settings" {
  allowed_api_ip_addresses          = ["1.1.1.1/32"]
  enforce_2fa                       = true
  restrict_user_api_token_creation  = true
  revoke_inactive_tokens_after_days = 90
}
