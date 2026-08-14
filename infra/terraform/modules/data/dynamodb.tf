# Single-table design per docs/design.md §4 — mirrors internal/store's
# DynamoDB Local schema (EnsureTable in internal/store/store.go) exactly, so
# the same application code works against either.
resource "aws_dynamodb_table" "main" {
  name         = var.project
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

  # This is the one resource in the project we're most protective of.
  # Destroying it is only ever a deliberate action (with a cmd/rollup export
  # taken first) — never a side effect of cycling the compute stack, and
  # not even a side effect of a careless `terraform destroy` in this stack.
  lifecycle {
    prevent_destroy = true
  }
}
