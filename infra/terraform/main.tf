terraform {
  required_version = ">= 1.1.0"
  backend "s3" {}
}

provider "aws" {
  region = var.aws_region
}

// Fetch AZs
data "aws_availability_zones" "available" {}

//// VPC
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "3.14.2"

  name = "${var.environment}-vpc"
  cidr = "10.0.0.0/16"

  azs             = data.aws_availability_zones.available.names
  public_subnets  = ["10.0.1.0/24", "10.0.2.0/24", "10.0.3.0/24"]
  private_subnets = ["10.0.101.0/24", "10.0.102.0/24", "10.0.103.0/24"]

  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true
  enable_dns_support   = true

  tags = {
    "Environment" = var.environment
    "Terraform"   = "true"
  }
}

//// RDS PostgreSQL (Long-term data)
module "postgresql" {
  source  = "terraform-aws-modules/rds/aws"
  version = "3.4.0"

  identifier = "${var.environment}-postgresql"

  engine            = "postgres"
  engine_version    = "13.7"
  instance_class    = var.rds_instance_class
  allocated_storage = var.rds_allocated_storage

  name     = var.db_name
  username = var.db_username
  password = var.db_password

  vpc_security_group_ids = [module.vpc.default_security_group_id]
  subnet_ids             = module.vpc.private_subnets

  multi_az          = true
  publicly_accessible = false

  backup_retention_period = 7
  skip_final_snapshot     = false

  tags = {
    "Environment" = var.environment
    "Terraform"   = "true"
  }
}

//// ElastiCache Redis (Short-term cache)
module "redis" {
  source  = "terraform-aws-modules/elasticache/aws"
  version = "2.1.0"

  cluster_id           = "${var.environment}-redis"
  engine               = "redis"
  engine_version       = "6.x"
  node_type            = var.redis_node_type
  number_cache_clusters = 2

  subnet_group_name = module.vpc.vpc_id
  subnet_ids        = module.vpc.private_subnets

  security_group_ids = [module.vpc.default_security_group_id]

  apply_immediately = true
  tags = {
    "Environment" = var.environment
    "Terraform"   = "true"
  }
}

//// CockroachDB (Short-term transactional data)
module "cockroach" {
  source           = "./modules/cockroach"
  environment      = var.environment
  vpc_id           = module.vpc.vpc_id
  private_subnets  = module.vpc.private_subnets
  instance_type    = var.cockroach_instance_type
  desired_capacity = var.cockroach_desired_capacity
  min_capacity     = var.cockroach_min_capacity
  max_capacity     = var.cockroach_max_capacity
}

//// EKS Cluster
module "eks" {
  source          = "terraform-aws-modules/eks/aws"
  version         = "18.32.3"

  cluster_name    = "${var.environment}-eks"
  cluster_version = "1.25"

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  node_groups = {
    default = {
      desired_capacity = var.eks_desired_capacity
      max_capacity     = var.eks_max_capacity
      min_capacity     = var.eks_min_capacity

      instance_types = [var.eks_instance_type]
    }
  }

  tags = {
    "Environment" = var.environment
    "Terraform"   = "true"
  }
}

//// IAM Roles and Policies
module "iam" {
  source = "./modules/iam"
  environment = var.environment
}

//// AWS Secrets Manager
module "secrets" {
  source = "./modules/secrets"
  environment = var.environment
  db_username = var.db_username
  db_password = var.db_password
}

//// CloudWatch Monitoring
module "monitoring" {
  source = "./modules/cloudwatch"
  environment = var.environment
  eks_cluster_name = module.eks.cluster_name
  rds_endpoint = module.postgresql.db_instance_address
}
