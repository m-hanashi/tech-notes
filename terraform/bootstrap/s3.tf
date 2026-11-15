# 常設層と起動層の state を置くバケット。
# 名前は世界で1つだけなので、消すと他人に取られるおそれがある。消さない前提で使う。
resource "aws_s3_bucket" "tfstate" {
  bucket = "tech-notes-tfstate"

  lifecycle {
    prevent_destroy = true
  }
}

# 上書きや削除をしても、前の版の state から戻せるようにする
resource "aws_s3_bucket_versioning" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  versioning_configuration {
    status = "Enabled"
  }
}

# 保存するデータを暗号化する（S3 の標準の暗号化）
resource "aws_s3_bucket_server_side_encryption_configuration" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# 外部に公開する設定を、4つすべて遮断する
resource "aws_s3_bucket_public_access_block" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# ACL を使わず、バケットの持ち主がすべてのオブジェクトを持つ
resource "aws_s3_bucket_ownership_controls" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id

  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

# HTTPS 以外の通信を拒否する
data "aws_iam_policy_document" "tfstate" {
  statement {
    sid     = "DenyInsecureTransport"
    effect  = "Deny"
    actions = ["s3:*"]
    resources = [
      aws_s3_bucket.tfstate.arn,
      "${aws_s3_bucket.tfstate.arn}/*",
    ]

    principals {
      type        = "*"
      identifiers = ["*"]
    }

    condition {
      test     = "Bool"
      variable = "aws:SecureTransport"
      values   = ["false"]
    }
  }
}

resource "aws_s3_bucket_policy" "tfstate" {
  bucket = aws_s3_bucket.tfstate.id
  policy = data.aws_iam_policy_document.tfstate.json

  # 公開の遮断を先に効かせてから、ポリシーを付ける
  depends_on = [aws_s3_bucket_public_access_block.tfstate]
}
