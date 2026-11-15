provider "aws" {
  region = "ap-northeast-1"

  default_tags {
    tags = {
      Project = "tech-notes"
      Layer   = "bootstrap"
    }
  }
}
