# Single-table design per docs/design.md §4 — mirrors internal/store's
# DynamoDB Local schema (EnsureTable in internal/store/store.go) exactly, so
# the same application code works against either.
#
# Name is "jobsyllabus" (no hyphen) deliberately, NOT var.project
# ("job-syllabus") — internal/store.TableName has hardcoded "jobsyllabus"
# since Phase 0/1, and that's the established, already-tested convention
# to match, not the newer Terraform naming.
resource "aws_dynamodb_table" "main" {
  name         = "jobsyllabus"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "PK"
  range_key    = "SK"

  attribute {
    name = "PK"
    type = "S"
  }
  attribute {
    name = "SK"
    type = "S"
  }
  attribute {
    name = "GSI1PK"
    type = "S"
  }
  attribute {
    name = "GSI1SK"
    type = "S"
  }
  attribute {
    name = "GSI2PK"
    type = "S"
  }
  attribute {
    name = "GSI2SK"
    type = "S"
  }

  global_secondary_index {
    name            = "GSI1"
    hash_key        = "GSI1PK"
    range_key       = "GSI1SK"
    projection_type = "ALL"
  }

  global_secondary_index {
    name            = "GSI2"
    hash_key        = "GSI2PK"
    range_key       = "GSI2SK"
    projection_type = "KEYS_ONLY"
  }

  point_in_time_recovery {
    enabled = true
  }

  # Backs the DEDUP#<contentHash> marker's 30-day expiry (docs/design.md
  # §5) — DynamoDB deletes items whose `ttl` attribute (epoch seconds) has
  # passed, no scheduled job required. Only items that set `ttl` are
  # affected; Posting/Skill/PostingSkill items never set it and live
  # forever.
  ttl {
    attribute_name = "ttl"
    enabled        = true
  }

  # This is the one resource in the project we're most protective of.
  # Destroying it is only ever a deliberate action (with a cmd/rollup export
  # taken first) — never a side effect of cycling the compute stack, and
  # not even a side effect of a careless `terraform destroy` in this stack.
  lifecycle {
    prevent_destroy = true
  }
}
