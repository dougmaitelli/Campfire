package model

// CopyArchiveName identifies a copy job's temporary portable full backup.
func CopyArchiveName(id string) string { return "full-" + id + ".tar.gz" }

// CopyPhase describes panel-coordinated creation from a portable full backup.
type CopyPhase string

// Copy jobs preserve these checkpoints across panel and Spark restarts.
const (
	CopyPreparing  CopyPhase = "preparing"
	CopyBackup     CopyPhase = "backup"
	CopyUploading  CopyPhase = "uploading"
	CopyRestoring  CopyPhase = "restoring"
	CopyCleanup    CopyPhase = "cleanup"
	CopyComplete   CopyPhase = "complete"
	CopyFailed     CopyPhase = "failed"
	CopyCancelling CopyPhase = "cancelling"
	CopyCancelled  CopyPhase = "cancelled"
)
