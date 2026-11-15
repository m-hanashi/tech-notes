output "tfstate_bucket" {
  description = "常設層と起動層の state を置くバケットの名前"
  value       = aws_s3_bucket.tfstate.bucket
}
