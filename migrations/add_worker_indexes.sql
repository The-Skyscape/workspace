-- Add indexes for AI Worker performance
-- These indexes optimize common queries in the worker feature

-- Index for finding workers by user and status
CREATE INDEX IF NOT EXISTS idx_ai_workers_user_status 
ON ai_workers(UserID, Status);

-- Index for finding active workers sorted by activity
CREATE INDEX IF NOT EXISTS idx_ai_workers_active 
ON ai_workers(Status, LastActiveAt DESC) 
WHERE Status IN ('ready', 'creating');

-- Index for chat messages by worker
CREATE INDEX IF NOT EXISTS idx_chat_messages_worker 
ON chat_messages(WorkerID, CreatedAt DESC);

-- Index for worker-repo associations
CREATE INDEX IF NOT EXISTS idx_ai_worker_repos_worker 
ON ai_worker_repos(WorkerID);

CREATE INDEX IF NOT EXISTS idx_ai_worker_repos_repo 
ON ai_worker_repos(RepoID);

-- Composite index for finding repos by worker
CREATE INDEX IF NOT EXISTS idx_ai_worker_repos_composite 
ON ai_worker_repos(WorkerID, RepoID);

-- Index for worker usage stats (single row table but helps with updates)
CREATE INDEX IF NOT EXISTS idx_worker_usage_reset 
ON worker_usage(MonthReset);