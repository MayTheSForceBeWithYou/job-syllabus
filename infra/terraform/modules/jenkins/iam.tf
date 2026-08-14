# "IAM instance profile with permissions for ECR push, ECS deploy, S3, and
# Terraform. Zero static AWS access keys anywhere in Jenkins." — docs/design.md
# §10. PowerUserAccess covers ECR/ECS/S3/EC2/ELB/SQS/DynamoDB/CloudWatch/
# SNS/Budgets/Route53 (everything Terraform needs to manage this project's
# infra) but deliberately excludes IAM, so a second policy grants IAM
# management scoped to resources this project actually owns
# ("${var.project}-*"), rather than full IAM admin over the account.
data "aws_iam_policy_document" "jenkins_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "jenkins" {
  name               = "${var.project}-jenkins"
  assume_role_policy = data.aws_iam_policy_document.jenkins_assume.json
}

resource "aws_iam_role_policy_attachment" "jenkins_power_user" {
  role       = aws_iam_role.jenkins.name
  policy_arn = "arn:aws:iam::aws:policy/PowerUserAccess"
}

data "aws_iam_policy_document" "jenkins_iam_scoped" {
  statement {
    sid = "ManageProjectIAM"
    actions = [
      "iam:CreateRole", "iam:DeleteRole", "iam:GetRole", "iam:ListRolePolicies",
      "iam:PutRolePolicy", "iam:DeleteRolePolicy", "iam:GetRolePolicy",
      "iam:AttachRolePolicy", "iam:DetachRolePolicy", "iam:ListAttachedRolePolicies",
      "iam:TagRole", "iam:UntagRole", "iam:UpdateAssumeRolePolicy",
      "iam:CreatePolicy", "iam:DeletePolicy", "iam:GetPolicy", "iam:GetPolicyVersion",
      "iam:CreatePolicyVersion", "iam:DeletePolicyVersion", "iam:ListPolicyVersions",
      "iam:CreateInstanceProfile", "iam:DeleteInstanceProfile",
      "iam:AddRoleToInstanceProfile", "iam:RemoveRoleFromInstanceProfile",
      "iam:GetInstanceProfile", "iam:PassRole",
    ]
    resources = [
      "arn:aws:iam::*:role/${var.project}-*",
      "arn:aws:iam::*:instance-profile/${var.project}-*",
      "arn:aws:iam::*:policy/${var.project}-*",
    ]
  }
  statement {
    sid       = "ReadOnlyIAM"
    actions   = ["iam:ListRoles", "iam:ListPolicies", "iam:ListInstanceProfiles"]
    resources = ["*"]
  }
}

# user_data reads this at boot to populate JENKINS_ADMIN_PASSWORD - see
# secrets.tf. Explicit statement rather than relying on PowerUserAccess's
# broad SSM coverage, so the one secret this instance can decrypt is
# self-documenting.
data "aws_iam_policy_document" "jenkins_read_own_secret" {
  statement {
    sid       = "ReadJenkinsAdminPassword"
    actions   = ["ssm:GetParameter"]
    resources = [aws_ssm_parameter.jenkins_admin_password.arn]
  }
  statement {
    sid       = "DecryptJenkinsAdminPassword"
    actions   = ["kms:Decrypt"]
    resources = ["arn:aws:kms:${data.aws_region.current.name}:${data.aws_caller_identity.current.account_id}:alias/aws/ssm"]
  }
}

resource "aws_iam_policy" "jenkins_read_own_secret" {
  name   = "${var.project}-jenkins-read-own-secret"
  policy = data.aws_iam_policy_document.jenkins_read_own_secret.json
}

resource "aws_iam_role_policy_attachment" "jenkins_read_own_secret" {
  role       = aws_iam_role.jenkins.name
  policy_arn = aws_iam_policy.jenkins_read_own_secret.arn
}

resource "aws_iam_policy" "jenkins_iam_scoped" {
  name   = "${var.project}-jenkins-iam-scoped"
  policy = data.aws_iam_policy_document.jenkins_iam_scoped.json
}

resource "aws_iam_role_policy_attachment" "jenkins_iam_scoped" {
  role       = aws_iam_role.jenkins.name
  policy_arn = aws_iam_policy.jenkins_iam_scoped.arn
}

resource "aws_iam_instance_profile" "jenkins" {
  name = "${var.project}-jenkins"
  role = aws_iam_role.jenkins.name
}

# --- Jenkins Fargate BUILD AGENTS (not the controller) - Task Role for the
# ephemeral agents the Amazon ECS plugin launches (§10). numExecutors: 0
# on the controller means ALL pipeline work - including infra-plan/
# infra-apply's own `terraform apply` - runs on these agents, so they need
# real permissions, not just the execution role's ECR-pull+CloudWatch-Logs
# scope. Phase 2 pointed JCasC's taskrole at the execution role as a
# placeholder ("tightened... in a later phase" per ci/jenkins.yaml's
# original comment); Phase 3's api-build Deploy stage (ECR push, ECS
# register-task-definition/update-service) is what makes that gap load-
# bearing, so it's fixed here rather than deferred again. Same permission
# set as the controller's own role, since agents now do the same class of
# work the controller was originally scoped for. ---

resource "aws_iam_role" "jenkins_agent" {
  name = "${var.project}-jenkins-agent"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ecs-tasks.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "jenkins_agent_power_user" {
  role       = aws_iam_role.jenkins_agent.name
  policy_arn = "arn:aws:iam::aws:policy/PowerUserAccess"
}

resource "aws_iam_role_policy_attachment" "jenkins_agent_iam_scoped" {
  role       = aws_iam_role.jenkins_agent.name
  policy_arn = aws_iam_policy.jenkins_iam_scoped.arn
}

# --- DLM (Data Lifecycle Manager): daily JENKINS_HOME snapshots ---

data "aws_iam_policy_document" "dlm_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["dlm.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "dlm" {
  name               = "${var.project}-dlm-lifecycle"
  assume_role_policy = data.aws_iam_policy_document.dlm_assume.json
}

resource "aws_iam_role_policy_attachment" "dlm" {
  role       = aws_iam_role.dlm.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSDataLifecycleManagerServiceRole"
}
