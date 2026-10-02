-- A paused download still counts as in the queue: one active copy of a video at a time.
DROP INDEX jobs_active_video;
CREATE UNIQUE INDEX jobs_active_video ON jobs(video_id)
    WHERE state IN ('queued', 'inspecting', 'downloading', 'processing', 'paused');
