# shoes-ecs-task

[myshoes](https://github.com/whywaita/myshoes) provider for [Amazon ECS](https://aws.amazon.com/ecs/).

## Environment values

- "ECS_TASK_CLUSTER"
  - The name of ECS Cluster
- "ECS_TASK_DEFINITION_ARN"
  - The ARN of Task Definition
- "ECS_TASK_SUBNET_ID"
  - The ID of Subnet
- "ECS_TASK_REGION"
  - The name of Region
- "ECS_TASK_NO_WAIT"
  - Optional (default: false)
  - If this value is set, myshoes does not wait for the task to be created.

## Acknowledgement

This provider was inspired by [the tech blog](https://techlife.cookpad.com/entry/2022/11/07/124025) by [Cookpad](https://github.com/cookpad). Thank you for your great work! 