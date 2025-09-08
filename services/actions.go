package services

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"workspace/models"
)

// ActionExecutor manages parallel execution of CI/CD actions
type ActionExecutor struct {
	mu           sync.RWMutex
	runningJobs  map[string]*ActionJob
	maxParallel  int
	jobQueue     chan *ActionJob
	workerWG     sync.WaitGroup
}

// ActionJob represents a single action execution job
type ActionJob struct {
	Action   *models.Action
	Run      *models.ActionRun
	RepoPath string
	Done     chan error
}

var (
	// Actions is the global action executor instance
	Actions = NewActionExecutor()
)

// NewActionExecutor creates a new action executor with default settings
func NewActionExecutor() *ActionExecutor {
	maxParallel := 5 // Default to 5 parallel jobs
	if envVal := os.Getenv("MAX_PARALLEL_ACTIONS"); envVal != "" {
		fmt.Sscanf(envVal, "%d", &maxParallel)
	}
	
	executor := &ActionExecutor{
		runningJobs: make(map[string]*ActionJob),
		maxParallel: maxParallel,
		jobQueue:    make(chan *ActionJob, 100), // Buffer up to 100 jobs
	}
	
	// Start worker goroutines
	executor.startWorkers()
	
	return executor
}

// startWorkers launches the worker goroutines for parallel execution
func (e *ActionExecutor) startWorkers() {
	for i := 0; i < e.maxParallel; i++ {
		e.workerWG.Add(1)
		go e.worker(i)
	}
	log.Printf("ActionExecutor: Started %d worker goroutines", e.maxParallel)
}

// worker is the main worker goroutine that processes jobs
func (e *ActionExecutor) worker(id int) {
	defer e.workerWG.Done()
	
	for job := range e.jobQueue {
		log.Printf("Worker %d: Starting execution of action %s", id, job.Action.Title)
		err := e.executeJob(job)
		job.Done <- err
		close(job.Done)
	}
}

// ExecuteAction queues an action for execution and returns immediately
func (e *ActionExecutor) ExecuteAction(action *models.Action, triggerEvent string) error {
	// Create action run record
	run := &models.ActionRun{
		Model:       models.DB.NewModel(""),
		ActionID:    action.ID,
		Status:      "pending",
		TriggerType: triggerEvent,
		Branch:      action.Branch,
	}
	
	// Save the run
	run, err := models.ActionRuns.Insert(run)
	if err != nil {
		return fmt.Errorf("failed to create action run: %v", err)
	}
	
	// Get repository path
	repo, err := models.Repositories.Get(action.RepoID)
	if err != nil {
		return fmt.Errorf("repository not found: %v", err)
	}
	
	// Create job
	job := &ActionJob{
		Action:   action,
		Run:      run,
		RepoPath: repo.Path(),
		Done:     make(chan error, 1),
	}
	
	// Track the job
	e.mu.Lock()
	e.runningJobs[run.ID] = job
	e.mu.Unlock()
	
	// Queue the job for execution
	select {
	case e.jobQueue <- job:
		log.Printf("ActionExecutor: Queued action %s for execution", action.Title)
		
		// Update run status to queued
		run.Status = "queued"
		models.ActionRuns.Update(run)
		
		// Don't wait for completion - return immediately
		go func() {
			// Wait for completion in background
			err := <-job.Done
			
			// Clean up tracking
			e.mu.Lock()
			delete(e.runningJobs, run.ID)
			e.mu.Unlock()
			
			if err != nil {
				log.Printf("ActionExecutor: Action %s failed: %v", action.Title, err)
			} else {
				log.Printf("ActionExecutor: Action %s completed successfully", action.Title)
			}
		}()
		
		return nil
		
	default:
		// Queue is full
		run.Status = "failed"
		run.Output = "Action queue is full. Too many actions running."
		run.Duration = 0
		models.ActionRuns.Update(run)
		
		return fmt.Errorf("action queue is full")
	}
}

// executeJob performs the actual execution of an action job
func (e *ActionExecutor) executeJob(job *ActionJob) error {
	action := job.Action
	run := job.Run
	
	// Update status to running
	run.Status = "running"
	startTime := time.Now()
	if err := models.ActionRuns.Update(run); err != nil {
		return fmt.Errorf("failed to update run status: %v", err)
	}
	
	// Update action status
	action.Status = "running"
	models.Actions.Update(action)
	
	// Execute based on type
	var output string
	var execErr error
	
	if action.Script != "" {
		output, execErr = e.executeScript(action, run, job.RepoPath)
	} else if action.Command != "" {
		output, execErr = e.executeCommand(action, run, job.RepoPath)
	} else {
		execErr = fmt.Errorf("no script or command defined")
	}
	
	// Update run with results
	run.Output = output
	run.Duration = int(time.Since(startTime).Seconds())
	
	if execErr != nil {
		run.Status = "failed"
		run.Output += fmt.Sprintf("\n\nError: %v", execErr)
		run.ExitCode = 1
		action.Status = "failed"
	} else {
		run.Status = "success"
		run.ExitCode = 0
		action.Status = "success"
	}
	
	// Save run
	if err := models.ActionRuns.Update(run); err != nil {
		log.Printf("Failed to update action run: %v", err)
	}
	
	// Update action
	if err := models.Actions.Update(action); err != nil {
		log.Printf("Failed to update action: %v", err)
	}
	
	// Log activity
	status := "completed"
	if execErr != nil {
		status = "failed"
	}
	models.LogActivity("action_executed", fmt.Sprintf("Action %s %s", action.Title, status),
		fmt.Sprintf("Action %s %s after %.1f seconds", action.Title, status, float64(run.Duration)),
		"system", action.RepoID, "action", action.ID)
	
	return execErr
}

// executeScript executes an action script in a Docker container
func (e *ActionExecutor) executeScript(action *models.Action, run *models.ActionRun, repoPath string) (string, error) {
	// Create sandbox container
	sandboxName := fmt.Sprintf("action-%s-%s", action.ID, run.ID)
	
	// Prepare the script
	scriptPath := filepath.Join("/tmp", fmt.Sprintf("action-%s.sh", run.ID))
	if err := os.WriteFile(scriptPath, []byte(action.Script), 0755); err != nil {
		return "", fmt.Errorf("failed to write script: %v", err)
	}
	defer os.Remove(scriptPath)
	
	// Create Docker run command
	dockerCmd := fmt.Sprintf(`docker run --rm --name %s \
		-v %s:/workspace:ro \
		-v %s:/script.sh:ro \
		-w /workspace \
		--network none \
		--memory="512m" \
		--cpus="1" \
		alpine:latest \
		sh -c "apk add --no-cache bash git && bash /script.sh"`,
		sandboxName, repoPath, scriptPath)
	
	// Execute with timeout
	timeout := 5 * time.Minute
	if action.TimeoutSeconds > 0 {
		timeout = time.Duration(action.TimeoutSeconds) * time.Second
	}
	
	cmd := exec.Command("bash", "-c", dockerCmd)
	cmd.Dir = repoPath
	
	// Set timeout
	timer := time.AfterFunc(timeout, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			// Also kill the container
			exec.Command("docker", "kill", sandboxName).Run()
		}
	})
	defer timer.Stop()
	
	// Execute and capture output
	output, err := cmd.CombinedOutput()
	
	// Clean up container if still running
	exec.Command("docker", "rm", "-f", sandboxName).Run()
	
	return string(output), err
}

// executeCommand executes a simple command in a Docker container
func (e *ActionExecutor) executeCommand(action *models.Action, run *models.ActionRun, repoPath string) (string, error) {
	// Create sandbox container
	sandboxName := fmt.Sprintf("action-%s-%s", action.ID, run.ID)
	
	// Wrap command for proper execution
	wrappedCmd := fmt.Sprintf("bash -c %q", action.Command)
	
	// Create Docker run command
	dockerCmd := fmt.Sprintf(`docker run --rm --name %s \
		-v %s:/workspace:ro \
		-w /workspace \
		--network none \
		--memory="512m" \
		--cpus="1" \
		alpine:latest \
		sh -c "apk add --no-cache bash git && %s"`,
		sandboxName, repoPath, wrappedCmd)
	
	// Execute with timeout
	timeout := 5 * time.Minute
	if action.TimeoutSeconds > 0 {
		timeout = time.Duration(action.TimeoutSeconds) * time.Second
	}
	
	cmd := exec.Command("bash", "-c", dockerCmd)
	cmd.Dir = repoPath
	
	// Set timeout
	timer := time.AfterFunc(timeout, func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
			// Also kill the container
			exec.Command("docker", "kill", sandboxName).Run()
		}
	})
	defer timer.Stop()
	
	// Execute and capture output
	output, err := cmd.CombinedOutput()
	
	// Clean up container if still running
	exec.Command("docker", "rm", "-f", sandboxName).Run()
	
	// Check for artifacts to collect
	if action.ArtifactPaths != "" {
		e.collectArtifacts(action, run, repoPath)
	}
	
	return string(output), err
}

// collectArtifacts collects build artifacts from the specified path
func (e *ActionExecutor) collectArtifacts(action *models.Action, run *models.ActionRun, repoPath string) {
	// For now, just use the first artifact path if multiple are specified
	paths := strings.Split(action.ArtifactPaths, ",")
	if len(paths) == 0 {
		return
	}
	
	artifactPath := filepath.Join(repoPath, strings.TrimSpace(paths[0]))
	
	// Check if artifact exists
	info, err := os.Stat(artifactPath)
	if err != nil {
		log.Printf("Artifact not found: %s", artifactPath)
		return
	}
	
	// Read artifact content (limit to 10MB)
	maxSize := int64(10 * 1024 * 1024)
	if info.Size() > maxSize {
		log.Printf("Artifact too large: %d bytes (max %d)", info.Size(), maxSize)
		return
	}
	
	content, err := os.ReadFile(artifactPath)
	if err != nil {
		log.Printf("Failed to read artifact: %v", err)
		return
	}
	
	// Create artifact record
	artifact := &models.ActionArtifact{
		Model:       models.DB.NewModel(""),
		RunID:       run.ID,
		ActionID:    action.ID,
		FileName:    filepath.Base(artifactPath),
		FilePath:    strings.TrimSpace(paths[0]),
		Content:     content,
		Size:        info.Size(),
		ContentType: "application/octet-stream",
	}
	
	if _, err := models.ActionArtifacts.Insert(artifact); err != nil {
		log.Printf("Failed to save artifact: %v", err)
	} else {
		log.Printf("Collected artifact: %s (%d bytes)", artifact.FileName, artifact.Size)
	}
}

// GetRunningJobs returns the list of currently running jobs
func (e *ActionExecutor) GetRunningJobs() map[string]*ActionJob {
	e.mu.RLock()
	defer e.mu.RUnlock()
	
	// Create a copy to avoid race conditions
	jobs := make(map[string]*ActionJob)
	for k, v := range e.runningJobs {
		jobs[k] = v
	}
	return jobs
}

// GetQueueSize returns the current size of the job queue
func (e *ActionExecutor) GetQueueSize() int {
	return len(e.jobQueue)
}

// GetWorkerCount returns the number of worker goroutines
func (e *ActionExecutor) GetWorkerCount() int {
	return e.maxParallel
}

// TriggerActionsByEvent triggers all actions matching the given event
func (e *ActionExecutor) TriggerActionsByEvent(eventType, repoID string, eventData map[string]string) error {
	// Find all actions for this event
	actions, err := models.Actions.Search("WHERE RepoID = ? AND TriggerEvent = ? AND Status != ?", 
		repoID, eventType, "disabled")
	if err != nil {
		return err
	}
	
	if len(actions) == 0 {
		return nil
	}
	
	log.Printf("ActionExecutor: Triggering %d actions for event %s in repo %s", 
		len(actions), eventType, repoID)
	
	// Execute each action in parallel
	for _, action := range actions {
		if err := e.ExecuteAction(action, eventType); err != nil {
			log.Printf("Failed to execute action %s: %v", action.Title, err)
		}
	}
	
	return nil
}

// Shutdown gracefully shuts down the action executor
func (e *ActionExecutor) Shutdown() {
	log.Println("ActionExecutor: Shutting down...")
	
	// Close job queue to signal workers to stop
	close(e.jobQueue)
	
	// Wait for all workers to finish
	e.workerWG.Wait()
	
	log.Println("ActionExecutor: Shutdown complete")
}