output "instance_id" {
  description = "EC2 instance ID"
  value       = aws_instance.branchd_test.id
}

output "public_ip" {
  description = "Public IP address of test instance"
  value       = aws_instance.branchd_test.public_ip
}

output "ami_id" {
  description = "AMI ID used for test instance"
  value       = data.aws_ami.ubuntu.id
}

output "ami_name" {
  description = "AMI name used for test instance"
  value       = data.aws_ami.ubuntu.name
}

output "postgres_version" {
  description = "PostgreSQL version being tested"
  value       = var.postgres_version
}

output "ssh_command" {
  description = "SSH command to connect to instance"
  value       = "ssh ubuntu@${aws_instance.branchd_test.public_ip}"
}

output "api_url" {
  description = "Branchd API URL (HTTPS with self-signed certificate)"
  value       = "https://${aws_instance.branchd_test.public_ip}"
}
