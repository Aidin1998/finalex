variable "environment" {
  description = "Deployment environment"
  type        = string
}

variable "vpc_id" {
  description = "VPC ID for CockroachDB"
  type        = string
}

variable "private_subnets" {
  description = "Private subnet IDs for CockroachDB ASG"
  type        = list(string)
}

variable "cockroach_instance_type" {
  description = "CockroachDB node EC2 instance type"
  type        = string
}

variable "cockroach_desired_capacity" {
  description = "CockroachDB desired number of nodes"
  type        = number
}

variable "cockroach_min_capacity" {
  description = "CockroachDB minimum nodes"
  type        = number
}

variable "cockroach_max_capacity" {
  description = "CockroachDB maximum nodes"
  type        = number
}
