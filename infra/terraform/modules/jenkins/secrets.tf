# This is a PUBLIC repo — jenkins.yaml (JCasC) cannot contain a real
# password. Terraform generates one, stores it as a SecureString SSM
# parameter, and user_data fetches it at boot (via the instance's IAM role,
# zero static credentials) into an environment file that systemd feeds to
# Jenkins. jenkins.yaml references it as ${JENKINS_ADMIN_PASSWORD}.
resource "random_password" "jenkins_admin" {
  length  = 24
  special = true
}

resource "aws_ssm_parameter" "jenkins_admin_password" {
  name  = "/${var.project}/jenkins/admin-password"
  type  = "SecureString"
  value = random_password.jenkins_admin.result
}
