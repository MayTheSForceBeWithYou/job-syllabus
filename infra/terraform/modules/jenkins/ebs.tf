# The snapshot data source errors if it matches zero snapshots, so
# "restore from the latest snapshot" can't be auto-detected on a truly
# first-ever apply — restore_from_snapshot is a manual toggle instead.
# Flip it to true once daily DLM snapshots exist (i.e. any apply after the
# first), including specifically after a full dev-compute
# destroy+recreate — that's the actual DoD scenario (docs/design.md §10:
# "terminate the EC2 instance, terraform apply, get a fully-configured
# Jenkins... no clicking").
data "aws_ebs_snapshot" "jenkins_home_latest" {
  count       = var.restore_from_snapshot ? 1 : 0
  most_recent = true
  owners      = ["self"]

  filter {
    name   = "tag:Name"
    values = ["${var.project}-jenkins-home"]
  }
}

resource "aws_ebs_volume" "jenkins_home" {
  availability_zone = data.aws_subnet.jenkins.availability_zone
  type              = "gp3"
  size              = var.restore_from_snapshot ? null : var.ebs_size_gb
  snapshot_id       = var.restore_from_snapshot ? data.aws_ebs_snapshot.jenkins_home_latest[0].id : null

  # Deliberately NOT prevent_destroy: this volume is meant to be
  # destroyed along with the rest of dev-compute and restored from the
  # latest DLM snapshot next apply, not protected from destruction like
  # the DynamoDB table in dev-data.
  tags = { Name = "${var.project}-jenkins-home" }
}

resource "aws_volume_attachment" "jenkins_home" {
  device_name = "/dev/xvdf"
  volume_id   = aws_ebs_volume.jenkins_home.id
  instance_id = aws_instance.jenkins.id
}

resource "aws_dlm_lifecycle_policy" "jenkins_home" {
  description        = "Daily JENKINS_HOME snapshots - 7 day retention"
  execution_role_arn = aws_iam_role.dlm.arn
  state              = "ENABLED"

  policy_details {
    resource_types = ["VOLUME"]
    target_tags = {
      Name = "${var.project}-jenkins-home"
    }

    schedule {
      name = "daily"
      create_rule {
        interval      = 24
        interval_unit = "HOURS"
        times         = ["06:00"]
      }
      retain_rule {
        count = 7
      }
      copy_tags = true
    }
  }
}
