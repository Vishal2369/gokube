package worker

import (
	"cube/queue"
	"cube/store"
	"cube/task"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type Worker struct {
	Name      string
	Queue     *queue.Queue[task.Task]
	Db        store.Store[*task.Task]
	TaskCount int
	Stats     *Stats
}

func New(name string, taskDbType string) *Worker {
	w := Worker{
		Name:  name,
		Queue: queue.New[task.Task](),
	}
	var s store.Store[*task.Task]
	switch taskDbType {
	case "memory":
		s = store.NewInMemoryStore[*task.Task]()
	}
	w.Db = s
	return &w
}

func (w *Worker) CollectStats() {
	for {
		log.Println("Collecting stats")
		w.Stats = GetStats()
		w.Stats.TaskCount = w.TaskCount
		time.Sleep(15 * time.Second)
	}
}

func (w *Worker) runTask() task.DockerResult {
	t := w.Queue.Dequeue()
	if t.ID == uuid.Nil {
		log.Printf("No task in the queu\n")
		return task.DockerResult{Error: nil}
	}

	taskPersisted, err := w.Db.Get(t.ID.String())
	if err != nil {
		taskPersisted = &t
		w.Db.Put(t.ID.String(), taskPersisted)
	}

	var result task.DockerResult
	if task.IsValidStateTransition(taskPersisted.State, t.State) {
		switch t.State {
		case task.Scheduled:
			result = w.StartTask(t)
		case task.Completed:
			result = w.StopTask(t)
		default:
			result.Error = errors.New("not reachable")
		}
	} else {
		err := fmt.Errorf("invalid transition from %v to %v",
			taskPersisted.State, t.State)
		result.Error = err
	}
	return result
}

func (w *Worker) RunTasks() {
	for {
		if w.Queue.Length() != 0 {
			result := w.runTask()
			if result.Error != nil {
				log.Printf("Error running task %v\n", result.ContainerId)
			}
		} else {
			log.Printf("No task to run currently\n")
		}

		log.Printf("Sleeping for 10 sec\n")
		time.Sleep(10 * time.Second)
	}
}

func (w *Worker) AddTask(t task.Task) {
	w.Queue.Enqueue(t)
}

func (w *Worker) StartTask(t task.Task) task.DockerResult {
	t.StartTime = time.Now().UTC()

	config := task.NewConfig(&t)
	d := task.NewDocker(config)

	result := d.Run()
	if result.Error != nil {
		log.Printf("error starting the container %v: %v\n", t.ID,
			result.Error)
		t.State = task.Failed
		w.Db.Put(t.ID.String(), &t)
		return result
	}
	t.ContainerId = result.ContainerId
	t.State = task.Running
	w.Db.Put(t.ID.String(), &t)

	return result
}

func (w *Worker) StopTask(t task.Task) task.DockerResult {
	config := task.NewConfig(&t)
	d := task.NewDocker(config)

	result := d.Stop(t.ContainerId)

	if result.Error != nil {
		log.Printf("error stopping container %s: %v\n", t.ContainerId,
			result.Error)
		t.State = task.Failed
		w.Db.Put(t.ID.String(), &t)
		return result
	}

	t.FinishTime = time.Now().UTC()
	t.State = task.Completed
	w.Db.Put(t.ID.String(), &t)

	log.Printf("stopped and removed container %s for task %s\n",
		t.ContainerId, t.ID)

	return result
}

func (w *Worker) GetTasks() []*task.Task {
	tasks, _ := w.Db.List()
	return tasks
}

func (w *Worker) InspectTask(t task.Task) task.DockerInspectResponse {
	config := task.NewConfig(&t)
	d := task.NewDocker(config)
	return d.Inspect(t.ContainerId)
}

func (w *Worker) UpdateTasks() {
	for {
		log.Println("Checking status of tasks")
		w.updateTasks()
		log.Println("Task updates completed")
		log.Println("Sleeping for 15 seconds")
		time.Sleep(15 * time.Second)
	}
}

func (w *Worker) updateTasks() {
	tasks, _ := w.Db.List()
	for id, t := range tasks {
		if t.State == task.Running {
			resp := w.InspectTask(*t)

			if resp.Error != nil {
				log.Printf("Error inspecting container : %v", resp.Error)
				continue
			}

			if resp.Container == nil {
				log.Printf("No container for running task %d\n", id)
				t.State = task.Failed
			} else if resp.Container.State.Status == "exited" {
				log.Printf("Container for task %d in non-running state %s", id, resp.Container.State.Status)
				t.State = task.Failed
			}

			t.HostPorts = resp.Container.NetworkSettings.NetworkSettingsBase.Ports

			w.Db.Put(t.ID.String(), t)
		}
	}
}
