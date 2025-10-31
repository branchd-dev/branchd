variable "region" {
  description = "AWS region for test infrastructure"
  type        = string
  default     = "us-east-2"
}

variable "instance_type" {
  description = "EC2 instance type for test VM"
  type        = string
  default     = "t4g.medium"
}

variable "postgres_version" {
  description = "PostgreSQL version to test (14, 15, 16, 17)"
  type        = string
  default     = "16"

  validation {
    condition     = contains(["14", "15", "16", "17"], var.postgres_version)
    error_message = "PostgreSQL version must be one of: 14, 15, 16, 17"
  }
}

variable "ssh_public_key" {
  description = "SSH public key for instance access"
  type        = string
  default     = "~/.ssh/id_rsa.pub"
}

variable "ssh_private_key_path" {
  description = "Path to SSH private key for provisioning"
  type        = string
  default     = "~/.ssh/id_rsa"
}
