package worker

import (
    "context"
    "fmt"
    "testing"
    "time"
)

func TestFutureGroupTrace(t *testing.T) {
    pool := NewPool(WithWorkers(4))
    pool.Start()
    defer func() {
        ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
        defer cancel()
        pool.Shutdown(ctx)
    }()

    group := NewFutureGroup()

    for i := 0; i < 5; i++ {
        val := i
        future := pool.SubmitFunc(func(ctx context.Context) (interface{}, error) {
            time.Sleep(10 * time.Millisecond)
            return val * 2, nil
        })
        group.Add(future)
    }

    fmt.Println("TRACE: queue len after submit:", pool.queue.Len())
    fmt.Println("TRACE: futures map len:", pool.PendingFutures())
    
    // Wait with timeout
    ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    results, err := group.Wait(ctx)
    if err != nil {
        fmt.Println("TRACE: queue len at failure:", pool.queue.Len())
        fmt.Println("TRACE: futures map len at failure:", pool.PendingFutures())
        fmt.Println("TRACE: active workers:", pool.ActiveWorkerCount())
        // Check which futures are done
        for i, f := range []*Future{} {
            fmt.Printf("TRACE: future %d done: %v\n", i, f.IsDone())
        }
        t.Fatalf("Wait failed: %v", err)
    }
    fmt.Printf("TRACE: Got %d results\n", len(results))
}
