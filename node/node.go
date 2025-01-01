package node

import (
	"cube/utils"
	"cube/worker"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
)

type Node struct {
	Name            string
	IP              string
	Api             string
	Cores           int
	Memory          int
	MemoryAllocated int
	Disk            int
	DiskAllocated   int
	Stats           worker.Stats
	Role            string
	TaskCount       int
}

func NewNode(name, api, role string) *Node {
	return &Node{
		Name: name,
		Api:  api,
		Role: role,
	}
}

func (n *Node) GetStats() (*worker.Stats, error) {
	url := fmt.Sprintf("%s/stats", n.Api)
	resp, err := utils.HTTPWithRetry(http.Get, url)
	if err != nil {
		msg := fmt.Sprintf("Unable to connect to %v. Permanent failure.\n", n.Api)
		log.Println(msg)
		return nil, errors.New(msg)
	}
	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("Error retrieving stats from %v: %v", n.Api, err)
		log.Println(msg)
		return nil, errors.New(msg)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var stats worker.Stats
	err = json.Unmarshal(body, &stats)
	if err != nil {
		msg := fmt.Sprintf("error decoding message while getting stats for node %s", n.Name)
		log.Println(msg)
		return nil, errors.New(msg)
	}
	n.Memory = int(stats.MemTotalKb())
	n.Disk = int(stats.DiskTotal())
	n.Stats = stats
	return &n.Stats, nil
}
