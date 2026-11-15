# 料金がしきい値を超えた、または超えそうなときに、メールで知らせる（仕様書 6.2）
resource "aws_budgets_budget" "monthly" {
  name         = "tech-notes-monthly"
  budget_type  = "COST"
  limit_amount = tostring(var.monthly_budget_usd)
  limit_unit   = "USD"
  time_unit    = "MONTHLY"

  # 実績が予算の50%を超えた
  notification {
    notification_type          = "ACTUAL"
    comparison_operator        = "GREATER_THAN"
    threshold                  = 50
    threshold_type             = "PERCENTAGE"
    subscriber_email_addresses = [var.notification_email]
  }

  # 実績が予算の100%を超えた
  notification {
    notification_type          = "ACTUAL"
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    subscriber_email_addresses = [var.notification_email]
  }

  # 月末の予測が予算の100%を超えた
  notification {
    notification_type          = "FORECASTED"
    comparison_operator        = "GREATER_THAN"
    threshold                  = 100
    threshold_type             = "PERCENTAGE"
    subscriber_email_addresses = [var.notification_email]
  }
}
