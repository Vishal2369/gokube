package store

import (
	"cube/task"
	"fmt"
	"log"
	"os"

	"github.com/boltdb/bolt"
)

type Store[T any] interface {
	Put(key string, value T) error
	Get(key string) (T, error)
	List() ([]T, error)
	Count() (int, error)
}

type InMemoryTaskStore[T any] struct {
	Db map[string]T
}

// Ensure InMemoryTaskStore implements the Store interface
var _ Store[*task.Task] = (*InMemoryTaskStore[*task.Task])(nil)
var _ Store[*task.TaskEvent] = (*InMemoryTaskStore[*task.TaskEvent])(nil)

func NewInMemoryStore[T any]() *InMemoryTaskStore[T] {
	return &InMemoryTaskStore[T]{
		Db: make(map[string]T),
	}
}

func (i *InMemoryTaskStore[T]) Put(key string, value T) error {
	i.Db[key] = value
	return nil
}

func (i *InMemoryTaskStore[T]) Get(key string) (T, error) {
	var zeroVal T

	t, ok := i.Db[key]
	if !ok {
		return zeroVal, fmt.Errorf("task with key %s does not exist", key)
	}
	return t, nil
}

func (i *InMemoryTaskStore[T]) List() ([]T, error) {
	data := make([]T, 0, len(i.Db))

	for _, v := range i.Db {
		data = append(data, v)
	}

	return data, nil
}
func (i *InMemoryTaskStore[T]) Count() (int, error) {
	return len(i.Db), nil
}

type TaskStore struct {
	Db       *bolt.DB
	DbFile   string
	FileMode os.FileMode
	Bucket   string
}

func NewTaskStore(file string, mode os.FileMode, bucket string) (*TaskStore, error) {
	db, err := bolt.Open(file, mode, nil)
	if err != nil {
		return nil, fmt.Errorf("unable to open %v", file)
	}

	t := TaskStore{
		DbFile:   file,
		FileMode: mode,
		Db:       db,
		Bucket:   bucket,
	}

	err = t.CreateBucket()

	if err != nil {
		log.Printf("bucket already exists, will use it instead of creating new one")
	}

	return &t, nil
}

func (t *TaskStore) Close() {
	t.Db.Close()
}
