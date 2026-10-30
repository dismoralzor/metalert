package pool_test

import (
	"sync"
	"sync/atomic"
	"testing"

	models "github.com/dismoralzor/metalert/internal/model"
	"github.com/dismoralzor/metalert/internal/pool"
)

// counter - тестовая заглушка с Reset(), считающая число вызовов Reset.
type counter struct {
	resets int
}

func (c *counter) Reset() { c.resets++ }

func TestPool_GetOnEmptyUsesFactory(t *testing.T) {
	var created int32
	p := pool.New(func() *models.Demo {
		atomic.AddInt32(&created, 1)
		return &models.Demo{}
	})

	obj := p.Get()
	if obj == nil {
		t.Fatal("Get() вернул nil на пустом пуле")
	}
	if atomic.LoadInt32(&created) != 1 {
		t.Errorf("фабрика вызвана %d раз, ожидался 1", created)
	}
}

func TestPool_PutResetsObject(t *testing.T) {
	p := pool.New(func() *models.Demo { return &models.Demo{} })

	note := "hello"
	d := &models.Demo{
		Count:   7,
		Name:    "server",
		Enabled: true,
		Note:    &note,
		Numbers: []int{1, 2, 3},
		Labels:  map[string]string{"a": "b"},
	}

	p.Put(d)

	// Проверяем сброс по ТОМУ ЖЕ указателю: Put должен был вызвать d.Reset().
	if d.Count != 0 || d.Name != "" || d.Enabled {
		t.Errorf("Put не сбросил примитивы: %+v", d)
	}
	if d.Note == nil || *d.Note != "" {
		t.Errorf("Put не сбросил *Note: %v", d.Note)
	}
	if len(d.Numbers) != 0 {
		t.Errorf("Put не сбросил Numbers: len=%d", len(d.Numbers))
	}
	if len(d.Labels) != 0 {
		t.Errorf("Put не очистил Labels: len=%d", len(d.Labels))
	}
}

func TestPool_GetAfterPutReturnsResetObject(t *testing.T) {
	p := pool.New(func() *models.Demo { return &models.Demo{} })

	p.Put(&models.Demo{Count: 42, Name: "dirty"})

	// Не проверяем тождество указателя: sync.Pool не гарантирует возврат того же
	// объекта (а под -race специально выбрасывает элементы).
	obj := p.Get()
	if obj == nil {
		t.Fatal("Get() после Put вернул nil")
	}
	if obj.Count != 0 || obj.Name != "" {
		t.Errorf("Get() вернул не сброшенный объект: %+v", obj)
	}
}

func TestPool_PutCallsResetOnce(t *testing.T) {
	p := pool.New(func() *counter { return &counter{} })

	c := &counter{}
	p.Put(c)

	if c.resets != 1 {
		t.Errorf("Reset вызван %d раз, ожидался ровно 1", c.resets)
	}
}

func TestPool_ConcurrentGetPut(t *testing.T) {
	p := pool.New(func() *models.Demo { return &models.Demo{} })

	const goroutines = 50
	const iterations = 1000

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				d := p.Get()
				d.Count = i
				d.Name = "x"
				d.Numbers = append(d.Numbers, i)
				p.Put(d)
			}
		}()
	}
	wg.Wait()
}
