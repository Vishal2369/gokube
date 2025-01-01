package worker

import (
	"cube/task"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type ErrorResponse struct {
	HttpStatusCode int
	Message        string
}

func (a *Api) StartTaskHandler(w http.ResponseWriter, r *http.Request) {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()

	te := task.TaskEvent{}

	err := d.Decode(&te)

	if err != nil {
		msg := fmt.Sprintf("Error marshalling body: %v", err)
		log.Println(msg)
		w.WriteHeader(http.StatusBadRequest)

		e := ErrorResponse{HttpStatusCode: http.StatusBadRequest, Message: msg}
		json.NewEncoder(w).Encode(e)
		return
	}

	a.Worker.AddTask(te.Task)
	log.Printf("Task added: %v\n", te.Task.ID)

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(te.Task)
}

func (a *Api) GetTaskHandler(w http.ResponseWriter, h *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(a.Worker.GetTasks())
}

func (a *Api) StopTaskHandler(w http.ResponseWriter, r *http.Request) {
	taskID := chi.URLParam(r, "taskID")
	if taskID == "" {
		log.Printf("No task id is present in the request\n")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	tID, err := uuid.Parse(taskID)

	if err != nil {
		log.Printf("Failed to parse the task id\n")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	taskToStop, err := a.Worker.Db.Get(tID.String())
	if err != nil {
		msg := fmt.Sprintf("No task found with id %v\n", tID)
		log.Print(msg)
		e := ErrorResponse{
			HttpStatusCode: http.StatusNotFound,
			Message:        msg,
		}
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(e)
		return
	}

	taskCopy := *taskToStop
	taskCopy.State = task.Completed

	a.Worker.AddTask(taskCopy)

	log.Printf("Added task %v to stop containter %v\n", taskCopy.ID,
		taskCopy.ContainerId)

	w.WriteHeader(http.StatusNoContent)
}

func (a *Api) GetStatsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(a.Worker.Stats)
}
