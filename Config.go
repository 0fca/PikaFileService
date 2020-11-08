package PikaFileService

type Config struct {
	Folders []string `json:"folders"`
	WorkingDirectory string `json:"workDir"`
	Dst string `json:"dstPath"`
}
