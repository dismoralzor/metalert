package pool_test

import (
	"fmt"

	"github.com/dismoralzor/metalert/internal/pool"
)

// ExamplePool показывает цикл Get -> использование -> Put -> Get. После Put
// объект сброшен, поэтому следующий Get отдаёт чистый объект (Count == 0) -
// независимо от того, вернул ли sync.Pool тот же объект или создал новый
// фабрикой.
func ExamplePool() {
	p := pool.New(func() *fixture { return &fixture{} })

	f := p.Get()
	f.Count = 42
	f.Name = "dirty"
	p.Put(f)

	got := p.Get()
	fmt.Println(got.Count)

	// Output:
	// 0
}
