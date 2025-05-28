// CockroachDB cluster using AWS EC2 instances
resource "aws_security_group" "cockroach" {
  name        = "${var.environment}-cockroach-sg"
  description = "Security group for CockroachDB nodes"
  vpc_id      = var.vpc_id

  ingress {
    from_port   = 26257
    to_port     = 26257
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/16"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_autoscaling_group" "cockroach" {
  name                      = "${var.environment}-cockroach-asg"
  launch_configuration      = aws_launch_configuration.cockroach.name
  vpc_zone_identifier       = var.private_subnets
  min_size                  = var.cockroach_min_capacity
  max_size                  = var.cockroach_max_capacity
  desired_capacity          = var.cockroach_desired_capacity
  health_check_type         = "EC2"
  health_check_grace_period = 300
}

resource "aws_launch_configuration" "cockroach" {
  name_prefix   = "${var.environment}-cockroach-lc"
  image_id      = data.aws_ami.cockroach.id
  instance_type = var.cockroach_instance_type
  security_groups = [aws_security_group.cockroach.id]
  user_data     = file("cockroach-user-data.sh")
}

data "aws_ami" "cockroach" {
  most_recent = true
  owners      = ["self"]
  filter {
    name   = "name"
    values = ["cockroachdb-*linux*image"]
  }
}
