package queue

type Queue[T any] struct {
	start *node[T]
	end   *node[T]
	len   int
}

type node[T any] struct {
	val  T
	next *node[T]
}

func New[T any]() *Queue[T] {
	return &Queue[T]{nil, nil, 0}
}

func (q *Queue[T]) Dequeue() T {
	if q.len == 0 {
		var zeroValue T
		return zeroValue
	}

	start := q.start

	if q.len == 1 {
		q.start = nil
		q.end = nil
		q.len = 0
	} else {
		q.start = q.start.next
		q.len--
	}
	return start.val
}

func (q *Queue[T]) Enqueue(val T) {
	n := &node[T]{val, nil}
	if q.len == 0 {
		q.start = n
		q.end = n
	} else {
		q.end.next = n
		q.end = n
	}
	q.len++
}

func (q *Queue[T]) Peek() T {
	if q.len == 0 {
		var zeroValue T
		return zeroValue
	}
	return q.start.val
}

func (q *Queue[T]) Length() int {
	return q.len
}
