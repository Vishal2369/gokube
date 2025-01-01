package manager

import (
	"bytes"
	"cube/node"
	"cube/queue"
	"cube/scheduler"
	"cube/store"
	"cube/task"
	"cube/worker"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
)

type Manager struct {
	Pending       *queue.Queue[*task.TaskEvent]
	TaskDb        store.Store[*task.Task]
	EventDb       store.Store[*task.TaskEvent]
	Workers       []string
	WorkerTaskMap map[string][]uuid.UUID
	TaskWorkerMap map[uuid.UUID]string
	LastWorkerIdx int
	WorkerNodes   []*node.Node
	Scheduler     scheduler.Scheduler
}

func New(workers []string, schedulerType, dbType string) *Manager {
	var nodes []*node.Node
	workerTaskMap := make(map[string][]uuid.UUID)
	for worker := range workers {
		workerTaskMap[workers[worker]] = []uuid.UUID{}

		nAPI := fmt.Sprintf("http://%v", workers[worker])
		n := node.NewNode(workers[worker], nAPI, "worker")
		nodes = append(nodes, n)
	}

	var s scheduler.Scheduler
	switch schedulerType {
	case "roundrobin":
		s = &scheduler.RoundRobin{Name: "roundrobin"}
	case "epvm":
		s = &scheduler.Epvm{Name: "epvm"}
	default:
		s = &scheduler.RoundRobin{Name: "roundrobin"}
	}

	var ts store.Store[*task.Task]
	var es store.Store[*task.TaskEvent]

	switch dbType {
	case "memory":
		ts = store.NewInMemoryStore[*task.Task]()
		es = store.NewInMemoryStore[*task.TaskEvent]()
	}

	return &Manager{
		Pending:       queue.New[*task.TaskEvent](),
		TaskDb:        ts,
		EventDb:       es,
		Workers:       workers,
		WorkerTaskMap: workerTaskMap,
		TaskWorkerMap: make(map[uuid.UUID]string),
		WorkerNodes:   nodes,
		Scheduler:     s,
	}
}

func (m *Manager) SelectWorker(t task.Task) (*node.Node, error) {
	candidates := m.Scheduler.SelectCandidateNodes(t, m.WorkerNodes)

	if candidates == nil {
		msg := fmt.Sprintf("No available candidates match resource request for task %v", t.ID)
		err := errors.New(msg)
		return nil, err
	}

	scores := m.Scheduler.Score(t, candidates)
	selectedNode := m.Scheduler.Pick(scores, candidates)
	return selectedNode, nil
}

func (m *Manager) AddTask(te task.TaskEvent) {
	m.Pending.Enqueue(&te)
}

func (m *Manager) updateTasks() {
	for _, worker := range m.Workers {
		log.Printf("Checking worker %v for task update", worker)
		url := fmt.Sprintf("http://%s/task", worker)

		resp, err := http.Get(url)
		if err != nil {
			log.Printf("Error connecting to %v:%v\n", worker, err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			log.Printf("Error sending request to worker :%v\n", worker)
			continue
		}

		d := json.NewDecoder(resp.Body)
		var tasks []*task.Task

		err = d.Decode(&tasks)

		if err != nil {
			log.Printf("Error decoding tasks :%v\n", err)
			continue
		}

		for _, t := range tasks {
			log.Printf("Attempting to update task: %v", t.ID)

			task, err := m.TaskDb.Get(t.ID.String())

			if err != nil {
				log.Printf("Task with ID %s not found\n", t.ID)
				continue
			}
			if task.State != t.State {
				task.State = t.State
			}
			task.StartTime = t.StartTime
			task.FinishTime = t.FinishTime
			task.ContainerId = t.ContainerId
			task.HostPorts = t.HostPorts

			m.TaskDb.Put(task.ID.String(), task)
		}
	}
}

func (m *Manager) UpdateTasks() {
	for {
		log.Println("Checking for task updates from workers")
		m.updateTasks()
		log.Println("Task updates completed")
		log.Println("Sleeping for 15 seconds")
		time.Sleep(15 * time.Second)
	}
}

func (m *Manager) ProcessTasks() {
	for {
		log.Println("Processing any tasks in the queue")
		m.SendWork()
		log.Println("Sleeping for 10 seconds")
		time.Sleep(10 * time.Second)
	}
}

func (m *Manager) DoHealthChecks() {
	for {
		log.Println("Performing task health check")
		m.doHealthChecks()
		log.Println("Task health checks completed")
		log.Println("Sleeping for 60 seconds")
		time.Sleep(60 * time.Second)
	}
}

func (m *Manager) SendWork() {
	if m.Pending.Length() == 0 {
		log.Println("Not work in the queue")
		return
	}

	te := m.Pending.Dequeue()
	t := te.Task
	m.EventDb.Put(te.ID.String(), te)

	log.Printf("Pulled %v from the pending queue\n", t)

	taskWorker, ok := m.TaskWorkerMap[t.ID]
	if ok {
		persistedTask, err := m.TaskDb.Get(t.ID.String())

		if err != nil {
			log.Printf("Failed to get task event with id: %v\n", t.ID)
			return
		}

		if te.State == task.Completed && task.IsValidStateTransition(persistedTask.State, te.State) {
			log.Printf("Stopping the task %v from worker %v", t, taskWorker)
			m.stopTask(taskWorker, t.ID.String())
			return
		}

		log.Printf("invalid request: existing task %s is in state %v and cannot transition to the completed state\n",
			persistedTask.ID.String(), persistedTask.State)
		return
	}

	w, err := m.SelectWorker(t)

	if err != nil {
		log.Printf("Failed to select a worker for task %v\n", t)
		return
	}

	m.WorkerTaskMap[w.Name] = append(m.WorkerTaskMap[w.Name], t.ID)
	m.TaskWorkerMap[t.ID] = w.Name

	t.State = task.Scheduled
	m.TaskDb.Put(t.ID.String(), &t)

	data, err := json.Marshal(te)
	if err != nil {
		log.Printf("Unable to marshal task object: %v\n", err)
		return
	}

	url := fmt.Sprintf("http://%s/task", w.Name)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))

	if err != nil {
		log.Printf("Error connecting to %v: %v\n", w, err)
		m.Pending.Enqueue(te)
		return
	}

	d := json.NewDecoder(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		e := worker.ErrorResponse{}
		err := d.Decode(&e)
		if err != nil {
			fmt.Printf("Error decoding response: %s\n", err.Error())
			return
		}
		log.Printf("Response error (%d): %s", e.HttpStatusCode, e.Message)
		return
	}
	t = task.Task{}
	err = d.Decode(&t)
	if err != nil {
		fmt.Printf("Error decoding response: %s\n", err.Error())
		return
	}
	log.Printf("%#v\n", t)
}

func (m *Manager) GetTasks() []*task.Task {
	tasks, _ := m.TaskDb.List()
	return tasks
}

func (m *Manager) checkTaskHealth(t task.Task) error {
	log.Printf("Calling health check for task %s: %s\n", t.ID, t.HealthCheck)

	w := m.TaskWorkerMap[t.ID]

	hostPort := getHostPort(t.HostPorts)

	worker := strings.Split(w, ":")

	if hostPort == nil {
		msg := fmt.Sprintf("Host port is nil for task %s", t.ID)
		log.Println(msg)
		return nil
	}

	url := fmt.Sprintf("http://%s:%s%s", worker[0], *hostPort, t.HealthCheck)

	log.Printf("Calling health check for task %s: %s\n", t.ID, url)

	resp, err := http.Get(url)

	if err != nil {
		msg := fmt.Sprintf("Error connecting to health check %s", url)
		log.Println(msg)
		return errors.New(msg)
	}

	if resp.StatusCode != http.StatusOK {
		msg := fmt.Sprintf("Error health check for task %s did not return 200\n", t.ID)
		log.Println(msg)
		return errors.New(msg)
	}

	log.Printf("Task %s health check response: %v\n", t.ID, resp.StatusCode)
	return nil
}

func getHostPort(ports nat.PortMap) *string {
	for _, portBinding := range ports {
		return &portBinding[0].HostPort
	}
	return nil
}

func (m *Manager) doHealthChecks() {
	for _, t := range m.GetTasks() {
		if t.State == task.Running && t.RestartCount < 3 {
			err := m.checkTaskHealth(*t)
			if err != nil {
				if t.RestartCount < 3 {
					m.restartTask(t)
				}
			}
		} else if t.State == task.Failed && t.RestartCount < 3 {
			m.restartTask(t)
		}
	}
}

func (m *Manager) restartTask(t *task.Task) {
	w := m.TaskWorkerMap[t.ID]
	t.State = task.Scheduled
	t.RestartCount++
	m.TaskDb.Put(t.ID.String(), t)

	te := task.TaskEvent{
		ID:        uuid.New(),
		State:     task.Running,
		Timestamp: time.Now(),
		Task:      *t,
	}
	data, err := json.Marshal(te)
	if err != nil {
		log.Printf("Unable to marshal task object: %v.", t)
		return
	}
	url := fmt.Sprintf("http://%s/task", w)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Printf("Error connecting to %v: %v", w, err)
		m.Pending.Enqueue(&te)
		return
	}
	d := json.NewDecoder(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		e := worker.ErrorResponse{}
		err := d.Decode(&e)
		if err != nil {
			fmt.Printf("Error decoding response: %s\n", err.Error())
			return
		}
		log.Printf("Response error (%d): %s", e.HttpStatusCode, e.Message)
		return
	}
	newTask := task.Task{}
	err = d.Decode(&newTask)
	if err != nil {
		fmt.Printf("Error decoding response: %s\n", err.Error())
		return
	}
	log.Printf("%#v\n", t)
}

func (m *Manager) stopTask(worker string, taskId string) {
	url := fmt.Sprintf("http://%s/task/%s", worker, taskId)

	client := &http.Client{}

	req, err := http.NewRequest(http.MethodDelete, url, nil)

	if err != nil {
		log.Printf("Error creating request to delete task %s: %v\n", taskId, err)
		return
	}

	resp, err := client.Do(req)

	if err != nil {
		log.Printf("error connecting to worker at %s: %v\n", url, err)
		return
	}
	if resp.StatusCode != 204 {
		log.Printf("Error sending request: %v\n", err)
		return
	}
	log.Printf("task %s has been scheduled to be stopped", taskId)
}
