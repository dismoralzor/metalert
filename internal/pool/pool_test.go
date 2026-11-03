package pool_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dismoralzor/metalert/internal/pool"
)

// counter - тестовая заглушка с Reset(), считающая число вызовов Reset.
type counter struct {
	resets int
}

func (c *counter) Reset() { c.resets++ }

// fixture - тестовая фикстура пула: структура с полями разных видов и Reset(),
// сбрасывающим её в нулевое состояние (указатель, слайс и мапа сохраняются).
type fixture struct {
	Count   int
	Name    string
	Enabled bool
	Note    *string
	Numbers []int
	Labels  map[string]string
}

func (f *fixture) Reset() {
	note, numbers, labels := f.Note, f.Numbers, f.Labels
	*f = fixture{}
	f.Note, f.Numbers, f.Labels = note, numbers[:0], labels
	if f.Note != nil {
		*f.Note = ""
	}
	clear(f.Labels)
}

func TestPool_GetOnEmptyUsesFactory(t *testing.T) {
	var created int32
	p := pool.New(func() *fixture {
		atomic.AddInt32(&created, 1)
		return &fixture{}
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
	p := pool.New(func() *fixture { return &fixture{} })

	note := "hello"
	f := &fixture{
		Count:   7,
		Name:    "server",
		Enabled: true,
		Note:    &note,
		Numbers: []int{1, 2, 3},
		Labels:  map[string]string{"a": "b"},
	}

	p.Put(f)

	// Проверяем сброс по ТОМУ ЖЕ указателю: Put должен был вызвать f.Reset().
	if f.Count != 0 || f.Name != "" || f.Enabled {
		t.Errorf("Put не сбросил примитивы: %+v", f)
	}
	if f.Note == nil || *f.Note != "" {
		t.Errorf("Put не сбросил *Note: %v", f.Note)
	}
	if len(f.Numbers) != 0 {
		t.Errorf("Put не сбросил Numbers: len=%d", len(f.Numbers))
	}
	if len(f.Labels) != 0 {
		t.Errorf("Put не очистил Labels: len=%d", len(f.Labels))
	}
}

func TestPool_GetAfterPutReturnsResetObject(t *testing.T) {
	p := pool.New(func() *fixture { return &fixture{} })

	p.Put(&fixture{Count: 42, Name: "dirty"})

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
	p := pool.New(func() *fixture { return &fixture{} })

	const goroutines = 50
	const iterations = 1000

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				f := p.Get()
				f.Count = i
				f.Name = "x"
				f.Numbers = append(f.Numbers, i)
				p.Put(f)
			}
		}()
	}
	wg.Wait()
}
