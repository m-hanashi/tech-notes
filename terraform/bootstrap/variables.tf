variable "notification_email" {
  description = "予算の通知を受け取るメールアドレス。terraform.tfvars に書き、公開しない"
  type        = string
  sensitive   = true
}

variable "monthly_budget_usd" {
  description = "月の予算（米ドル）"
  type        = number
  default     = 30
}
