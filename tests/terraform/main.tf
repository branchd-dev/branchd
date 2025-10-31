terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.region
}

# Data source: Find latest Ubuntu 24.04 LTS ARM64 AMI
data "aws_ami" "ubuntu" {
  most_recent = true
  owners      = ["099720109477"] # Canonical

  filter {
    name   = "name"
    values = ["ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-arm64-server-*"]
  }

  filter {
    name   = "state"
    values = ["available"]
  }

  filter {
    name   = "architecture"
    values = ["arm64"]
  }

  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# Security group for test VM
resource "aws_security_group" "branchd_test" {
  name_prefix = "branchd-e2e-test-"
  description = "Security group for Branchd E2E test instance"

  # SSH access
  ingress {
    from_port   = 22
    to_port     = 22
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "SSH access"
  }

  # Caddy HTTP (redirects to HTTPS)
  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Caddy HTTP (redirects to HTTPS)"
  }

  # Caddy HTTPS (API + UI)
  ingress {
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Caddy HTTPS (API + UI)"
  }

  # PostgreSQL branch ports (15432-16432)
  # AWS Security Group allows full range, UFW dynamically opens specific ports
  ingress {
    from_port   = 15432
    to_port     = 16432
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
    description = "PostgreSQL branch instances"
  }

  # Outbound internet access
  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name      = "branchd-e2e-test"
    Purpose   = "E2E-Testing"
    ManagedBy = "Terraform"
  }

  lifecycle {
    create_before_destroy = true
  }
}

# Test EC2 instance
resource "aws_instance" "branchd_test" {
  ami           = data.aws_ami.ubuntu.id
  instance_type = "t4g.small"
  key_name      = "default"

  vpc_security_group_ids = [aws_security_group.branchd_test.id]

  # Root volume
  root_block_device {
    volume_size           = 10
    volume_type           = "gp3"
    iops                  = 3000
    throughput            = 125
    encrypted             = true
    delete_on_termination = true
  }

  # Data volume for ZFS pool (tank)
  ebs_block_device {
    device_name           = "/dev/sdb"
    volume_size           = 2
    volume_type           = "gp3"
    iops                  = 3000
    throughput            = 125
    encrypted             = true
    delete_on_termination = true
  }

  tags = {
    Name      = "branchd-e2e-test-${var.postgres_version}"
    Purpose   = "E2E-Testing"
    ManagedBy = "Terraform"
    Version   = var.postgres_version
  }

  # Wait for instance to be ready
  provisioner "remote-exec" {
    inline = [
      "cloud-init status --wait",
      "echo 'Instance ready'",
    ]

    connection {
      type        = "ssh"
      user        = "ubuntu"
      private_key = file(var.ssh_private_key_path)
      host        = self.public_ip
      timeout     = "5m"
    }
  }

  # Upload server_setup.sh script
  provisioner "file" {
    source      = "../../scripts/server_setup.sh"
    destination = "/tmp/server_setup.sh"

    connection {
      type        = "ssh"
      user        = "ubuntu"
      private_key = file(var.ssh_private_key_path)
      host        = self.public_ip
      timeout     = "5m"
    }
  }

  # Run server_setup.sh
  provisioner "remote-exec" {
    inline = [
      "chmod +x /tmp/server_setup.sh",
      "/tmp/server_setup.sh --pg-version=${var.postgres_version} 2>&1 | tee /tmp/setup.log",
      "echo 'Setup complete'",
    ]

    connection {
      type        = "ssh"
      user        = "ubuntu"
      private_key = file(var.ssh_private_key_path)
      host        = self.public_ip
      timeout     = "30m"
    }
  }
}
