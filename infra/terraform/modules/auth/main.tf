# Cognito auth (docs/design.md §7/§8, Phase 6): the Hosted UI + PKCE app
# client the Expo client signs in through, and the JWT authorizer
# modules/api-gateway attaches to /v1/*. Single-operator personal tool
# (docs/design.md §1, and the Bedrock use-case form's own "internal users
# only" answer) — deliberately simplified from a multi-tenant design:
# self-signup is disabled (admin_create_user_only), and the one app
# client requests both scopes for every signed-in user rather than
# building out per-user role assignment nothing here needs yet.
data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

resource "aws_cognito_user_pool" "main" {
  name = "${var.project}-users"

  # No public self-registration — this is an internal tool with one real
  # user (the operator), created via `aws cognito-idp admin-create-user`
  # or the console, not a sign-up flow.
  admin_create_user_config {
    allow_admin_create_user_only = true
  }

  username_attributes      = ["email"]
  auto_verified_attributes = ["email"]

  password_policy {
    minimum_length    = 12
    require_lowercase = true
    require_uppercase = true
    require_numbers   = true
    require_symbols   = false
  }

  tags = { Name = "${var.project}-users" }
}

# Domain prefix must be globally unique across every AWS account using
# Cognito — account-ID-suffixed to avoid a collision with someone else's
# "job-syllabus" pick.
resource "aws_cognito_user_pool_domain" "main" {
  domain       = "${var.project}-${data.aws_caller_identity.current.account_id}"
  user_pool_id = aws_cognito_user_pool.main.id
}

# Custom scopes (docs/design.md §7: "admin scope for writes, read scope
# for queries"). The identifier becomes the scope prefix Cognito returns
# in the access token, e.g. "job-syllabus-api/read".
resource "aws_cognito_resource_server" "api" {
  identifier   = "${var.project}-api"
  name         = "${var.project} API"
  user_pool_id = aws_cognito_user_pool.main.id

  scope {
    scope_name        = "read"
    scope_description = "Read ranked skills, postings, companies, stats, and the review queue."
  }
  scope {
    scope_name        = "admin"
    scope_description = "Triage the review queue and other write operations."
  }
}

# Public client (no secret) using Authorization Code + PKCE — the only
# OAuth flow expo-auth-session's Hosted UI integration needs, and the
# correct choice for a client that ships in an app bundle / runs in a
# browser, where a confidential client secret can't actually stay secret.
resource "aws_cognito_user_pool_client" "client" {
  name         = "${var.project}-client"
  user_pool_id = aws_cognito_user_pool.main.id

  generate_secret = false

  allowed_oauth_flows_user_pool_client = true
  allowed_oauth_flows                  = ["code"]
  allowed_oauth_scopes = [
    "openid", "email", "profile",
    "${aws_cognito_resource_server.api.identifier}/read",
    "${aws_cognito_resource_server.api.identifier}/admin",
  ]
  supported_identity_providers = ["COGNITO"]

  callback_urls = var.callback_urls
  logout_urls   = var.logout_urls

  # expo-auth-session's PKCE flow needs the refresh token to keep a
  # mobile session alive across app restarts without re-prompting Hosted
  # UI every time.
  explicit_auth_flows = ["ALLOW_REFRESH_TOKEN_AUTH", "ALLOW_USER_SRP_AUTH"]

  access_token_validity  = 1
  id_token_validity      = 1
  refresh_token_validity = 30
  token_validity_units {
    access_token  = "hours"
    id_token      = "hours"
    refresh_token = "days"
  }
}
