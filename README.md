# Simple Kubernetes-like Orchestrator

This project is a lightweight orchestrator inspired by Kubernetes, implemented in **Go**. It enables managing containerized tasks across multiple workers. The orchestrator includes APIs to schedule, monitor, and stop tasks, with built-in health checks and container restart capabilities.

## Features

- **Manager and Worker Architecture**: 
  - Manager communicates with multiple workers using API calls.
  - Workers execute tasks and report their status to the manager.
  
- **Task Management**:
  - API endpoints to schedule tasks, monitor their status, and stop them.
  
- **Scheduling Algorithm**:
  - Tasks are currently scheduled using a round-robin algorithm.
  - Future plans include adding support for additional scheduling algorithms.

- **Health Checks and Fault Recovery**:
  - Workers periodically check the health of running tasks.
  - Failed tasks are automatically restarted.

- **Future Enhancements**:
  - CLI tool for easier interaction.
  - Enhanced scheduling strategies.

## Architecture Overview

The orchestrator consists of four main entities:
1. **Manager**: Handles task scheduling and worker management.
2. **Worker**: Executes tasks and performs health checks.
3. **Task**: Represents a containerized task to be executed.
4. **TaskEvent**: Tracks state changes of tasks.

### Workflow
1. Clients interact with the **manager** via REST API calls.
2. The **manager** schedules tasks on available **workers**.
3. **Workers** execute tasks and periodically report their status to the **manager**.
4. Failed tasks are restarted by the **worker**.

## API Endpoints

| Endpoint             | Method | Description                                      |
|----------------------|--------|--------------------------------------------------|
| `/task`              | POST   | Schedule a new task on a worker.                |
| `/task`              | GET    | Retrieve all running tasks from all workers.    |
| `/task/{taskId}`     | GET    | Get details of a specific task by its `taskId`. |
| `/task/{taskId}`     | DELETE | Stop a running task by its `taskId`.            |

### Example Usage
To interact with the manager:
1. Clone the repository:
    ```bash
    git clone https://github.com/Vishal2369/gokube
    ```
2. Start the project:
   ```bash
   CUBE_MANAGER_HOST=localhost CUBE_MANAGER_HOST=5555 CUBE_WORKER_HOST=localhost CUBE_WORKER_HOST=5556 go run main.go
   ```
3. Use API calls to interact with the manager. Example with curl:
    - Schedule a task:
        ```bash
        curl -v -X POST localhost:5555/task -d @task1.json
        ```
    - Get all running tasks:
        ```bash
        curl localhost:5555/task
        ```
    - Get task details by taskId:
        ```bash
        curl http://localhost:8080/task/{taskId}
        ```
    - Stop a task:
        ```bash
        curl -X DELETE http://localhost:8080/task/{taskId}
        ```

### Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.

### License

This project is licensed under the MIT License. See the LICENSE file for details.