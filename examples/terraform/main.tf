provider "aws" {
  region = "ap-northeast-1"
}

data "aws_iam_policy_document" "assume_role" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "ecs_task" {
  name               = "ecs-task-execute-role"
  assume_role_policy = data.aws_iam_policy_document.assume_role.json
}

resource "aws_iam_role_policy_attachment" "ecs_task" {
  role       = aws_iam_role.ecs_task.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

resource "aws_ecs_cluster" "myshoes" {
  name = "myshoes"
}

resource "aws_ecs_cluster_capacity_providers" "farget_spot" {
  cluster_name       = aws_ecs_cluster.myshoes.name
  capacity_providers = ["FARGATE_SPOT"]

  default_capacity_provider_strategy {
    capacity_provider = "FARGATE_SPOT"
    base              = 0
  }

  depends_on = [aws_ecs_cluster.myshoes]
}

resource "aws_cloudwatch_log_group" "runner" {
  name              = "/ecs/myshoes-ecs/runner"
  retention_in_days = 30
}

resource "aws_ecs_task_definition" "myshoes" {
  family = "myshoes"

  requires_compatibilities = ["FARGATE"]
  execution_role_arn       = aws_iam_role.ecs_task.arn

  cpu    = "256"
  memory = "512"

  network_mode = "awsvpc"

  container_definitions = <<EOL
[
  {
    "name": "runner",
    "image": "myoung34/github-runner-base",
    "logConfiguration": {
      "logDriver": "awslogs",
      "options": {
        "awslogs-region": "ap-northeast-1",
        "awslogs-stream-prefix": "runner",
        "awslogs-group": "/ecs/myshoes-ecs/runner"
      }
    }
  }
]
EOL
}