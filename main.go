package main

import (
	"cube/manager"
	"cube/worker"
	"fmt"
	"os"
	"strconv"
)

func main() {
	workers := initWorkers()

	initManager(workers)

}

func initWorkers() []string {
	whost := os.Getenv("CUBE_WORKER_HOST")
	wport, _ := strconv.Atoi(os.Getenv("CUBE_WORKER_PORT"))

	fmt.Println("Starting cube worker")

	workers := make([]string, 0)

	for i := range 3 {
		w := worker.New(fmt.Sprintf("worker-%d", i), "memory")

		wapi := worker.Api{Address: whost, Port: wport + i, Worker: w}

		go w.RunTasks()
		go w.CollectStats()
		go w.UpdateTasks()
		go wapi.Start()

		workers = append(workers, fmt.Sprintf("%s:%d", wapi.Address, wapi.Port))
	}

	return workers
}

func initManager(workers []string) {
	mhost := os.Getenv("CUBE_MANAGER_HOST")
	mport, _ := strconv.Atoi(os.Getenv("CUBE_MANAGER_PORT"))

	fmt.Println("Starting Cube manager")
	m := manager.New(workers, "roundrobin", "memory")

	mapi := manager.Api{Address: mhost, Port: mport, Manager: m}

	go m.ProcessTasks()
	go m.UpdateTasks()
	go m.DoHealthChecks()
	mapi.Start()
}
