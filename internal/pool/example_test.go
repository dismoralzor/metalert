package pool_test

import (
	"fmt"

	models "github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/pool"
)

// ExamplePool показывает цикл Get -> использование -> Put -> Get. После Put
// объект сброшен, поэтому следующий Get отдаёт чистый объект (Count == 0) -
// независимо от того, вернул ли sync.Pool тот же объект или создал новый
// фабрикой.
func ExamplePool() {
	p := pool.New(func() *models.Demo { return &models.Demo{} })

	d := p.Get()
	d.Count = 42
	d.Name = "dirty"
	p.Put(d)

	got := p.Get()
	fmt.Println(got.Count)

	// Output:
	// 0
}
