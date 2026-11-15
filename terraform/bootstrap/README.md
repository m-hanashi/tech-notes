# 準備用の Terraform（bootstrap）

常設層と起動層の state を置く S3 バケットと、予算の通知を作ります。最初に一度だけ、手元で実行します（仕様書 6.1、6.5）。

## 作るもの

| リソース | 名前 | 内容 |
|---|---|---|
| S3 バケット | `tech-notes-tfstate` | state の置き場所。バージョン管理・暗号化・公開の遮断・HTTPS 以外の通信の拒否を有効にする |
| AWS Budgets の予算 | `tech-notes-monthly` | 月 $30。実績が50%と100%を超えたとき、月末の予測が100%を超えたときに、メールで知らせる |

料金は、どちらもほぼかかりません。S3 は state のファイルが小さいので月1円未満で、予算の通知は無料です。

## 実行の手順

すべて、このフォルダ（`terraform/bootstrap/`）で行います。

1. AWS にログインする：`aws login --profile <管理用のプロファイル>`
2. `terraform.tfvars.example` を `terraform.tfvars` という名前でコピーし、通知を受け取るメールアドレスを書く。`terraform.tfvars` は `.gitignore` で除外しているので、公開されない
3. `terraform init`：AWS のプロバイダー（Terraform から AWS を操作するプラグイン）を取り寄せる
4. `AWS_PROFILE=<管理用のプロファイル> terraform plan`：何が作られるかを確かめる
5. `AWS_PROFILE=<管理用のプロファイル> terraform apply`：表示された計画を読んでから、`yes` と入力する。確認を省くオプション（`-auto-approve`）は使わない（仕様書 6.3）

## 注意

- plan や apply で `No valid credential sources found` のエラーが出たら、先に `aws sts get-caller-identity --profile <管理用のプロファイル>` を実行してから、もう一度実行する。AWS CLI が `aws login` の一時的な認証情報を新しくするので、Terraform もそれを使えるようになる（2026-11-15 に、この方法で解消した）
- 通知のメールアドレスの確認は要らない。SNS のメールの通知と違い、AWS Budgets は確認なしでメールを送る
- state は、このフォルダの `terraform.tfstate` に保存される（`.gitignore` 済み）。消すと、Terraform がこのバケットと予算を管理できなくなる。そのときは `terraform import` で取り込み直す
- バケットには `prevent_destroy` を付けているので、`terraform destroy` では消えない。バケットの名前は世界で1つだけで、消すと他人に取られるおそれがあるため、消さない前提で使う
