package task

import (
	"context"
	"io"
	"log"
	"math"
	"os"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
)

type State int

const (
	Pending State = iota
	Scheduled
	Running
	Completed
	Failed
)

var stateTransitionMap = map[State][]State{
	Pending:   {Scheduled},
	Scheduled: {Scheduled, Running, Failed},
	Running:   {Running, Completed, Failed},
	Completed: {},
	Failed:    {},
}

type Task struct {
	ID            uuid.UUID
	ContainerId   string
	Name          string
	State         State
	Image         string
	Memory        int
	Disk          int
	ExposedPort   nat.PortSet
	HostPorts     nat.PortMap
	PortBindings  map[string]string
	RestartPolicy container.RestartPolicyMode
	StartTime     time.Time
	FinishTime    time.Time
	HealthCheck   string
	RestartCount  int
}

type TaskEvent struct {
	ID        uuid.UUID
	State     State
	Timestamp time.Time
	Task      Task
}

type Config struct {
	Name          string
	AttachStdin   bool
	AttachStdout  bool
	AttachStderr  bool
	ExposedPorts  nat.PortSet
	PortBindings  map[string]string
	Cmd           []string
	Image         string
	Cpu           float64
	Memory        int64
	Disk          int64
	Env           []string
	RestartPolicy container.RestartPolicyMode
}

type Docker struct {
	Client *client.Client
	Config Config
}

type DockerInspectResponse struct {
	Error     error
	Container *types.ContainerJSON
}

type DockerResult struct {
	Error       error
	Action      string
	ContainerId string
	Result      string
}

func Contains(states []State, state State) bool {
	for _, s := range states {
		if s == state {
			return true
		}
	}
	return false
}

func IsValidStateTransition(src State, dst State) bool {
	return Contains(stateTransitionMap[src], dst)
}

func NewConfig(t *Task) *Config {
	return &Config{
		Name:          t.Name,
		Image:         t.Image,
		RestartPolicy: t.RestartPolicy,
		ExposedPorts:  t.ExposedPort,
		PortBindings:  t.PortBindings,
	}
}

func NewDocker(config *Config) *Docker {
	dc, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		log.Printf("Failed to instantiate docket client :%v\n", err)
		panic(err)
	}
	return &Docker{
		Client: dc,
		Config: *config,
	}
}

func (d *Docker) Run() DockerResult {
	ctx := context.Background()
	reader, err := d.Client.ImagePull(ctx, d.Config.Image, image.PullOptions{})

	if err != nil {
		log.Printf("Error pulling images %s: %v\n", d.Config.Image, err)
		return DockerResult{Error: err}
	}

	defer reader.Close()

	io.Copy(os.Stdout, reader)

	rp := container.RestartPolicy{
		Name: d.Config.RestartPolicy,
	}

	r := container.Resources{
		Memory:   d.Config.Memory,
		NanoCPUs: int64(d.Config.Cpu * math.Pow10(9)),
	}

	cc := container.Config{
		Image:        d.Config.Image,
		Tty:          false,
		Env:          d.Config.Env,
		ExposedPorts: d.Config.ExposedPorts,
	}

	hc := container.HostConfig{
		RestartPolicy:   rp,
		Resources:       r,
		PublishAllPorts: true,
	}

	resp, err := d.Client.ContainerCreate(ctx, &cc, &hc, nil, nil, d.Config.Name)

	if err != nil {
		log.Printf("Error creating container using image %s: %v", d.Config.Name,
			err)
		return DockerResult{Error: err}
	}

	err = d.Client.ContainerStart(ctx, resp.ID, container.StartOptions{})

	if err != nil {
		log.Printf("Error starting container using image %s: %v", d.Config.Name,
			err)
		return DockerResult{Error: err}
	}

	reader, err = d.Client.ContainerLogs(ctx, resp.ID,
		container.LogsOptions{ShowStdout: true, ShowStderr: true})

	if err != nil {
		log.Printf("Error fetching container logs for container %s: %v",
			resp.ID, err)
		return DockerResult{Error: err}
	}

	io.Copy(os.Stdout, reader)

	return DockerResult{
		ContainerId: resp.ID, Action: "start", Result: "success",
	}
}

func (d *Docker) Stop(id string) DockerResult {
	log.Printf("Attempting to stop container %s", id)

	ctx := context.Background()

	err := d.Client.ContainerStop(ctx, id, container.StopOptions{})

	if err != nil {
		log.Printf("Error stopping the container %s: %v", id, err)
		return DockerResult{Error: err}
	}

	err = d.Client.ContainerRemove(ctx, id, container.RemoveOptions{
		RemoveVolumes: true, Force: false})

	if err != nil {
		log.Printf("Error removing the contaienr %s: %v", id, err)
		return DockerResult{Error: err}
	}
	return DockerResult{Action: "stop", Result: "success"}
}

func (d *Docker) Inspect(containerId string) DockerInspectResponse {
	resp, err := d.Client.ContainerInspect(context.Background(), containerId)
	if err != nil {
		log.Printf("Error inspecting container %s: %v\n", containerId, err)
		return DockerInspectResponse{Error: err}
	}
	return DockerInspectResponse{Container: &resp}
}
