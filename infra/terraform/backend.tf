terraform {
  backend "s3" {
    bucket = "pincex-terraform-state"
    key    = "infra/terraform.tfstate"
    region = "us-east-1"
  }
}
