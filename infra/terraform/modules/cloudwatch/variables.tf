variable "environment" {
  description = "Deployment environment"
  type        = string
}

variable "notification_email" {
  description = "Email address for SNS notifications"
  type        = string
}

variable "rds_identifier" {
  description = "RDS instance identifier for alarms"
  type        = string
}
