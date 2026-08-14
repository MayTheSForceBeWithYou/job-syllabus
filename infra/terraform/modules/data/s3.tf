data "aws_caller_identity" "current" {}

# One bucket, three prefixes — matches the architecture diagram in
# docs/design.md §3 (S3: raw/, exports/) plus backups/ for the
# cmd/rollup export/import safety net (docs/design.md §9 amendment).
resource "aws_s3_bucket" "data" {
  bucket = "${var.project}-data-${data.aws_caller_identity.current.account_id}"

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_versioning" "data" {
  bucket = aws_s3_bucket.data.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "data" {
  bucket = aws_s3_bucket.data.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_public_access_block" "data" {
  bucket = aws_s3_bucket.data.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_lifecycle_configuration" "data" {
  bucket = aws_s3_bucket.data.id

  # raw/ — original posting snapshots (Posting.RawS3Key). Not written yet
  # (Phase 1 doesn't persist to S3), but the lifecycle rule is defined now
  # per §3's architecture diagram: "private, lifecycle -> Glacier @ 90d".
  rule {
    id     = "raw-to-glacier"
    status = "Enabled"
    filter {
      prefix = "raw/"
    }
    transition {
      days          = 90
      storage_class = "GLACIER"
    }
    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }

  # exports/ — generated CSV/XLSX, served via 15-minute presigned URLs
  # (docs/design.md §7). Transient by design; no reason to keep them.
  rule {
    id     = "exports-expire"
    status = "Enabled"
    filter {
      prefix = "exports/"
    }
    expiration {
      days = 30
    }
    noncurrent_version_expiration {
      noncurrent_days = 30
    }
  }

  # backups/ — cmd/rollup export dumps. Each export is a full snapshot, so
  # older ones become redundant once a newer one exists; bounded like
  # everything else per the "unbounded retention is a classic surprise
  # bill" principle in §9's observability section. Copy one out manually
  # if you want to keep it past 90 days.
  rule {
    id     = "backups-expire"
    status = "Enabled"
    filter {
      prefix = "backups/"
    }
    expiration {
      days = 90
    }
    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }
}

output "data_bucket" {
  value = aws_s3_bucket.data.id
}

output "data_bucket_arn" {
  value = aws_s3_bucket.data.arn
}
