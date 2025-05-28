// infrastructure variables
tflint = {}
variable "aws_region" {
  description = "AWS region to deploy resources"
  type        = string
  default     = "us-east-1"
}

variable "environment" {
  description = "Deployment environment (e.g., dev, staging, prod)"
  type        = string
  default     = "dev"
}

variable "db_name" {
  description = "PostgreSQL database name"
  type        = string
  default     = "pincexdb"
}

variable "db_username" {
  description = "Database master username"
  type        = string
  default     = "pincexadmin"
}

variable "db_password" {
  description = "Database master password"
  type        = string
}

variable "rds_instance_class" {
  description = "RDS instance class"
  type        = string
  default     = "db.t3.medium"
}

variable "rds_allocated_storage" {
  description = "RDS allocated storage in GB"
  type        = number
  default     = 20
}

variable "redis_node_type" {
  description = "ElastiCache Redis node type"
  type        = string
  default     = "cache.t3.micro"
}

variable "eks_desired_capacity" {
  description = "EKS worker node desired capacity"
  type        = number
  default     = 2
}

variable "eks_min_capacity" {
  description = "EKS worker node minimum capacity"
  type        = number
  default     = 1
}

variable "eks_max_capacity" {
  description = "EKS worker node maximum capacity"
  type        = number
  default     = 4
}

variable "eks_instance_type" {
  description = "EKS worker node EC2 instance type"
  type        = string
  default     = "t3.medium"
}

// CockroachDB variables
variable "cockroach_instance_type" {
  description = "CockroachDB node EC2 instance type"
  type        = string
  default     = "r5.large"
}

variable "cockroach_desired_capacity" {
  description = "CockroachDB desired number of nodes"
  type        = number
  default     = 3
}

variable "cockroach_min_capacity" {
  description = "CockroachDB minimum nodes"
  type        = number
  default     = 3
}

variable "cockroach_max_capacity" {
  description = "CockroachDB maximum nodes"
  type        = number
  default     = 5
}
