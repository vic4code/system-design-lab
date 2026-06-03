# Bucket name includes account ID to guarantee global uniqueness.
resource "aws_s3_bucket" "audio" {
  bucket = "beatstream-audio-${var.environment}-${data.aws_caller_identity.current.account_id}"
}

resource "aws_s3_bucket_public_access_block" "audio" {
  bucket                  = aws_s3_bucket.audio.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_versioning" "audio" {
  bucket = aws_s3_bucket.audio.id
  versioning_configuration { status = "Enabled" }
}

resource "aws_s3_bucket_lifecycle_configuration" "audio" {
  bucket = aws_s3_bucket.audio.id

  # Abort uploads that were never completed (stuck multipart uploads burn storage).
  rule {
    id     = "abort-incomplete-multipart"
    status = "Enabled"
    filter {}
    abort_incomplete_multipart_upload { days_after_initiation = 7 }
  }

  # Tracks not streamed in 90 days move to S3-IA (~40% cheaper, 30ms retrieval penalty).
  # Lesson: most audio streaming services serve a long-tail of tracks rarely played.
  rule {
    id     = "transition-to-ia"
    status = "Enabled"
    filter { prefix = "tracks/" }
    transition {
      days          = 90
      storage_class = "STANDARD_IA"
    }
  }
}
