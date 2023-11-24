package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/ecs/types"

	"github.com/hashicorp/go-plugin"
	pb "github.com/whywaita/myshoes/api/proto.go"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AppConfig struct {
	Cluster        string
	TaskDefinition string
	SubnetID       string
	Region         string

	NoWait bool
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	handshake := plugin.HandshakeConfig{
		ProtocolVersion:  1,
		MagicCookieKey:   "SHOES_PLUGIN_MAGIC_COOKIE",
		MagicCookieValue: "are_you_a_shoes?",
	}
	pluginMap := map[string]plugin.Plugin{
		"shoes_grpc": &ECSTaskPlugin{},
	}

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: handshake,
		Plugins:         pluginMap,
		GRPCServer:      plugin.DefaultGRPCServer,
	})

	return nil
}

// ECSTaskPlugin is plugin for lxd multi node
type ECSTaskPlugin struct {
	plugin.Plugin
}

func loadAppConfig() (*AppConfig, error) {
	const (
		EnvCluster        = "ECS_TASK_CLUSTER"
		EnvTaskDefinition = "ECS_TASK_DEFINITION_ARN"
		EnvSubnetID       = "ECS_TASK_SUBNET_ID"
		EnvRegion         = "ECS_TASK_REGION"
		EnvNoWait         = "ECS_TASK_NO_WAIT"
	)

	if strings.EqualFold(os.Getenv(EnvCluster), "") {
		return nil, fmt.Errorf("must set %s", EnvCluster)
	}
	if strings.EqualFold(os.Getenv(EnvTaskDefinition), "") {
		return nil, fmt.Errorf("must set %s", EnvTaskDefinition)
	}
	if strings.EqualFold(os.Getenv(EnvSubnetID), "") {
		return nil, fmt.Errorf("must set %s", EnvSubnetID)
	}
	if strings.EqualFold(os.Getenv(EnvRegion), "") {
		return nil, fmt.Errorf("must set %s", EnvRegion)
	}

	// Optional
	noWait := false
	if strings.EqualFold(os.Getenv(EnvNoWait), "true") {
		noWait = true
	}

	return &AppConfig{
		Cluster:        os.Getenv(EnvCluster),
		TaskDefinition: os.Getenv(EnvTaskDefinition),
		SubnetID:       os.Getenv(EnvSubnetID),
		Region:         os.Getenv(EnvRegion),
		NoWait:         noWait,
	}, nil
}

// GRPCServer is implement gRPC Server.
func (p *ECSTaskPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	c, err := loadAppConfig()
	if err != nil {
		return fmt.Errorf("loadAppConfig(): %w", err)
	}

	client := Client{appConfig: *c}
	pb.RegisterShoesServer(s, client)
	return nil
}

// GRPCClient is implement gRPC client.
// This function is not have client, so return nil
func (p *ECSTaskPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return nil, nil
}

// Client is client of ECS Task
type Client struct {
	pb.UnimplementedShoesServer

	appConfig AppConfig
}

// AddInstance create an ECS Task.
func (c Client) AddInstance(ctx context.Context, req *pb.AddInstanceRequest) (*pb.AddInstanceResponse, error) {
	oneLine := ToOneLine(req.SetupScript)
	taskArn, err := runTask(ctx, c.appConfig, oneLine)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to runTask(): %+v", err)
	}

	return &pb.AddInstanceResponse{
		CloudId:   taskArn,
		ShoesType: "ecs-task-fargate",
		IpAddress: "",
	}, nil
}

// DeleteInstance delete an ECS Task.
func (c Client) DeleteInstance(ctx context.Context, req *pb.DeleteInstanceRequest) (*pb.DeleteInstanceResponse, error) {
	// Deleting ECS task is automatically.
	return &pb.DeleteInstanceResponse{}, nil
}

func runTask(ctx context.Context, appConfig AppConfig, oneLineSetupScript string) (string, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(appConfig.Region))
	if err != nil {
		return "", fmt.Errorf("config.LoadDefaultConfig(ctx, config.WithRegion(%s): %w", appConfig.Region, err)
	}

	runTaskIn := &ecs.RunTaskInput{
		TaskDefinition: aws.String(appConfig.TaskDefinition),
		Cluster:        aws.String(appConfig.Cluster),
		LaunchType:     types.LaunchTypeFargate,

		NetworkConfiguration: &types.NetworkConfiguration{
			AwsvpcConfiguration: &types.AwsVpcConfiguration{
				AssignPublicIp: types.AssignPublicIpEnabled,
				Subnets: []string{
					appConfig.SubnetID,
				},
			},
		},
		Overrides: &types.TaskOverride{
			ContainerOverrides: []types.ContainerOverride{
				{
					Command: []string{
						"bash",
						"-c",
						oneLineSetupScript,
					},
					Name: aws.String("runner"),
				},
			},
		},
	}

	client := ecs.NewFromConfig(cfg)
	runOut, err := client.RunTask(ctx, runTaskIn)
	if err != nil {
		return "", fmt.Errorf("client.RunTask(ctx, runTaskIn): %w", err)
	}

	taskArn := aws.ToString(runOut.Tasks[0].TaskArn)

	if appConfig.NoWait {
		return taskArn, nil
	}

	waiter := ecs.NewTasksRunningWaiter(client)
	descTaskIn := &ecs.DescribeTasksInput{
		Tasks:   []string{taskArn},
		Cluster: aws.String(appConfig.Cluster),
	}
	maxWaitDur := 5 * time.Minute
	if err := waiter.Wait(ctx, descTaskIn, maxWaitDur); err != nil {
		return "", fmt.Errorf("waiter.Wait(ctx, %+v, %s): %w", descTaskIn, maxWaitDur, err)
	}

	return taskArn, nil
}

// ToOneLine convert bash script to one line.
func ToOneLine(in string) string {
	ll := strings.Split(in, "\n")

	var commands []string
	for _, line := range ll {
		if strings.HasPrefix(line, "#") {
			// Remove shebang and commend
			continue
		}
		if strings.EqualFold(line, "") {
			// Remove blank comment
			continue
		}

		commands = append(commands, line)
	}

	return strings.Join(commands, ";")
}
