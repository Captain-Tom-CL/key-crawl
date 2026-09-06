package analysis

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/captain-tom-cl/key-crawl/src/internal/settings"
	"github.com/captain-tom-cl/key-crawl/src/internal/storage"
)

const (
	Idle      = "idle"
	Running   = "running"
	Completed = "completed"
)

var ErrAlreadyRunning = errors.New("分析任务正在运行")

type TaskStatus struct {
	State       string   `json:"state"`
	Total       int      `json:"total"`
	Succeeded   int      `json:"succeeded"`
	Failed      int      `json:"failed"`
	Success     []string `json:"success"`
	CurrentFile string   `json:"currentFile"`
	Error       string   `json:"error"`
	LastError   string   `json:"lastError"`
}

type analysisTaskStore struct {
	mu     sync.RWMutex
	status TaskStatus
}

var analysisTask = analysisTaskStore{
	status: TaskStatus{State: Idle, Success: []string{}},
}

var analyzerLogger = log.New(os.Stdout, "[分析] ", log.LstdFlags)

func Status() TaskStatus {
	return analysisTask.snapshot()
}

func Start() (TaskStatus, error) {
	return analysisTask.start()
}

func (task *analysisTaskStore) snapshot() TaskStatus {
	task.mu.RLock()
	defer task.mu.RUnlock()
	return task.snapshotLocked()
}

// The caller holds task.mu. Copy the slice so encoding HTTP responses never
// reads an array that the background worker is modifying.
func (task *analysisTaskStore) snapshotLocked() TaskStatus {
	status := task.status
	status.Success = append([]string{}, status.Success...)
	return status
}

func (task *analysisTaskStore) start() (TaskStatus, error) {
	task.mu.Lock()
	defer task.mu.Unlock()
	if task.status.State == Running {
		return task.snapshotLocked(), ErrAlreadyRunning
	}

	// Preparation only scans filenames and loads configuration; no model calls
	// or HTML reads happen while holding the lock.
	task.status = TaskStatus{State: Completed, Success: []string{}}
	files, err := storage.ListHTML()
	if err != nil {
		err = fmt.Errorf("读取待分析目录失败：%w", err)
		task.status.Error = err.Error()
		analyzerLogger.Print(task.status.Error)
		return task.snapshotLocked(), err
	}
	task.status.Total = len(files)
	if len(files) == 0 {
		return task.snapshotLocked(), nil
	}
	config, err := settings.Load()
	if err != nil {
		err = fmt.Errorf("读取设置失败：%w", err)
		task.status.Error = err.Error()
		analyzerLogger.Print(task.status.Error)
		return task.snapshotLocked(), err
	}

	// Reserve the single task before launching it. Its context and configuration
	// are independent of the HTTP request that starts the batch.
	task.status.State = Running
	initial := task.snapshotLocked()
	go task.run(context.Background(), *config, files)
	return initial, nil
}

func (task *analysisTaskStore) run(ctx context.Context, config settings.Config, files []string) {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	var taskErr error
	defer func() {
		// On scheduler failure, stop workers before publishing the final state.
		if recovered := recover(); recovered != nil {
			taskErr = fmt.Errorf("后台分析异常中断：%v", recovered)
			cancel()
		}
		workers.Wait()
		cancel()
		task.mu.Lock()
		if taskErr != nil {
			task.status.Error = taskErr.Error()
		}
		task.status.CurrentFile = ""
		task.status.State = Completed
		final := task.snapshotLocked()
		task.mu.Unlock()
		if taskErr != nil {
			analyzerLogger.Print(taskErr)
		}
		analyzerLogger.Printf("分析任务结束：成功 %d 个，失败 %d 个，共 %d 个", final.Succeeded, final.Failed, final.Total)
	}()

	requester := newModelRequester(&config)
	workerCount := min(config.Concurrency, len(files))
	analyzerLogger.Printf("后台分析开始：共 %d 个 HTML 文件，并发 %d，RPM %d（0 表示不主动限速）", len(files), workerCount, config.RequestsPerMinute)
	jobs := make(chan string)
	// Close before the deferred workers.Wait, on both normal exit and panic.
	defer close(jobs)
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case fileName, ok := <-jobs:
					if !ok {
						return
					}
					task.processFile(ctx, &config, requester, fileName)
				}
			}
		}()
	}
	for _, fileName := range files {
		select {
		case <-ctx.Done():
			taskErr = ctx.Err()
			return
		case jobs <- fileName:
		}
	}
}

func (task *analysisTaskStore) processFile(ctx context.Context, config *settings.Config, requester *modelRequester, fileName string) {
	var err error
	defer func() {
		// Recover per file: the scheduler cannot recover a worker's panic.
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("文件处理异常：%v", recovered)
		}
		task.recordResult(fileName, err)
	}()
	analyzerLogger.Printf("开始处理文件：%s", fileName)
	err = processAnalysisFile(ctx, config, requester, fileName)
}

func (task *analysisTaskStore) recordResult(fileName string, err error) {
	task.mu.Lock()
	if err != nil {
		task.status.Failed++
		task.status.LastError = fileName + "：" + err.Error()
	} else {
		task.status.Succeeded++
		task.status.Success = append(task.status.Success, fileName)
	}
	task.mu.Unlock()
	if err != nil {
		analyzerLogger.Printf("文件处理失败：%s；%v", fileName, err)
	} else {
		analyzerLogger.Printf("文件处理完成：%s", fileName)
	}
}
