// Upload workflows publish this state independently of its visual presentation.
export interface UploadProgress {
  name: string;
  index: number;
  count: number;
  completedFiles: number;
  uploadedBytes: number;
  totalBytes: number;
  phase: "uploading" | "saving" | "cancelling";
}
